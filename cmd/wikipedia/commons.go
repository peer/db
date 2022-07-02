package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

// CommonsCommand uses Wikimedia Commons entities dump as input and creates a document for each entity in the file, mapping statements to PeerDB claims.
//
// Besides claims based on statements, it creates also claims with the following properties:
// WIKIMEDIA_COMMONS_ENTITY_ID (Q prefixed ID), WIKIMEDIA_COMMONS_PAGE_ID (internal page ID of the file),
// WIKIMEDIA_COMMONS_FILE_NAME (just filename, without "File:" prefix, but with underscores and file extension),
// WIKIMEDIA_COMMONS_FILE (URL to file page), FILE_URL (URL to full resolution or raw file), FILE (IS claim), ALSO_KNOWN_AS (for any English labels),
// DESCRIPTION (for English entity descriptions). Name of the document is filename without file extension and without underscores.
//
// When creating claims referencing other documents it just assumes a reference is valid and creates one, storing original Wikidata ID into a name
// for language xx-*. This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true document names for English language (ID for language xx-* is useful for debugging when reference is invalid).
// References to Wikimedia Commons files are done in a similar fashion, but with a meta claim.
type CommonsCommand struct {
	URL string `placeholder:"URL" help:"URL of Wikimedia Commons entities JSON dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *CommonsCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestCommonsEntitiesRun
	}

	ctx, cancel, _, _, processor, _, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessCommonsEntitiesDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, processor, entity)
	})
	if errE != nil {
		return errE
	}

	return nil
}

func (c *CommonsCommand) processEntity(
	ctx context.Context, globals *Globals, processor *elastic.BulkProcessor, entity mediawiki.Entity,
) errors.E {
	document, err := wikipedia.ConvertEntity(ctx, globals.Log, wikipedia.NameSpaceWikimediaCommonsFile, entity)
	if err != nil {
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		} else {
			globals.Log.Error().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		}
		return nil
	}

	err = document.Add(&search.IdentifierClaim{
		CoreClaim: search.CoreClaim{
			ID:         search.GetID(wikipedia.NameSpaceWikimediaCommonsFile, entity.ID, "WIKIMEDIA_COMMONS_PAGE_ID", 0),
			Confidence: wikipedia.HighConfidence,
		},
		Prop:       search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_PAGE_ID"),
		Identifier: strconv.FormatInt(entity.PageID, 10),
	})
	if err != nil {
		globals.Log.Error().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		return nil
	}

	// We remove media type property claims because we populate them ourselves from the image table SQL dump
	// (alongside more metadata). We also determine more detailed media types than what is available here.
	_ = document.Remove(wikipedia.GetWikidataDocumentID("P1163"))

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", entity.ID).Msg("saving document")
	insertOrReplaceDocument(processor, globals.Index, document)

	return nil
}

// CommonsFilesCommand uses Wikimedia Commons images (really files) table SQL dump as input and adds metadata for
// each file in the table to the corresponding document.
//
// It accesses existing documents in ElasticSearch to load corresponding file's document which is then updated with claims with the
// following properties (not necessary all of them): MEDIA_TYPE, MEDIAWIKI_MEDIA_TYPE, SIZE (in bytes), PAGE_COUNT, LENGTH (in seconds),
// multiple PREVIEW_URL (a list of URLs of previews), WIDTH, HEIGHT. The idea is that these claims should be enough to populate
// a file claim (in other documents using these files).
//
// For some files (primarily PDFs and DJVU files) metadata is not stored in the SQL table but SQL table only contains a reference to additional
// blob storage (see: https://phabricator.wikimedia.org/T301039). Because of that this command uses Wikimedia Commons API to obtain metadata
// for those files. This introduces some issues. API is rate limited, so processing can be slower than pure offline processing would be (configuring
// high ItemsProcessingThreads can mitigate this somewhat, so that while some threads are blocked on API, other threads can continue to process other
// files which do not require API). There can be discrepancies between the table state and what is available through the API: files from the table might
// be deleted since the table dump has been made. On the other hand, metadata rarely changes (only if metadata is re-extracted/re-computed, or if a new version
// of a file has been uploaded) so the fact that metadata might be from a different file revision does not seem to be too problematic here. We anyway
// want the latest information about files because we directly use files hosted on Wikimedia Commons by displaying them, so if they are changed or deleted,
// we want to know that (otherwise we could try to display an image which does not exist anymore, which would fail to load).
type CommonsFilesCommand struct {
	URL string `placeholder:"URL" help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *CommonsFilesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestCommonsImageMetadataRun
	}

	ctx, cancel, httpClient, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig[wikipedia.Image]{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i wikipedia.Image) errors.E {
			return c.processImage(
				ctx, globals, httpClient, esClient, processor, i,
			)
		},
		Progress:    config.Progress,
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
	if errE != nil {
		return errE
	}

	return nil
}

func (c *CommonsFilesCommand) processImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, esClient *elastic.Client, processor *elastic.BulkProcessor, image wikipedia.Image,
) errors.E {
	document, hit, err := wikipedia.GetWikimediaCommonsFile(ctx, globals.Index, esClient, image.Name)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = image.Name
		globals.Log.Error().Err(err).Fields(details).Msg("file not found")
		return nil
	}

	err = wikipedia.ConvertWikimediaCommonsImage(ctx, globals.Log, httpClient, globals.Token, globals.APILimit, image, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = image.Name
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Err(err).Fields(details).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", image.Name).Msg("updating document")
	updateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

// CommonsFileDescriptionsCommand uses Wikimedia Commons API as input to obtain and extract descriptions for files (namespace 6)
// and adds file's description to a corresponding file document.
//
// It expects documents populated by CommonsCommand.
//
// File articles contain a lot of metadata which we do not yet extract, but extract only a HTML description. It is expected that the
// rest of metadata is available through Wikimedia Commons entities or similar structured data. Extracted HTML descriptions are processed
// so that HTML can be directly displayed alongside other content. Use of Wikipedia's CSS nor Javascript is not needed after processing.
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding Wikimedia Commons entity's document which is then updated with
// claims with the following properties: DESCRIPTION (potentially multiple), ALSO_KNOWN_AS (from redirects pointing to the file).
type CommonsFileDescriptionsCommand struct{}

func (c *CommonsFileDescriptionsCommand) Run(globals *Globals) errors.E {
	ctx, cancel, httpClient, esClient, processor, _, _, errE := initializeRun(globals, nil, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	pages := make(chan wikipedia.AllPagesPage, wikipedia.APILimit)
	rateLimit := wikipediaRESTRateLimit / wikipediaRESTRatePeriod.Seconds()
	limiter := rate.NewLimiter(rate.Limit(rateLimit), wikipediaRESTRateLimit)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(pages)
		return wikipedia.ListAllPages(ctx, httpClient, []int{filesWikipediaNamespace}, "commons.wikimedia.org", globals.Token, limiter, pages)
	})

	var count x.Counter
	ticker := x.NewTicker(ctx, &count, 0, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			globals.Log.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("docs", count.Count()).
				Str("elapsed", p.Elapsed.Truncate(time.Second).String()).
				Send()
		}
	}()

	for i := 0; i < int(rateLimit); i++ {
		g.Go(func() error {
			// Loop ends with pages is closed, which happens when context is cancelled, too.
			for page := range pages {
				err := limiter.Wait(ctx)
				if err != nil {
					// Context has been canceled.
					return errors.WithStack(err)
				}

				html, errE := wikipedia.GetPageHTML(ctx, httpClient, "commons.wikimedia.org", page.Title)
				if errE != nil {
					globals.Log.Error().Err(errE).Fields(errors.AllDetails(errE)).Send()
					continue
				}

				count.Increment()

				errE = c.processPage(ctx, globals, httpClient, esClient, processor, page, html)
				if errE != nil {
					return errE
				}
			}
			return nil
		})
	}

	return errors.WithStack(g.Wait())
}

func (c *CommonsFileDescriptionsCommand) processPage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, esClient *elastic.Client,
	processor *elastic.BulkProcessor, page wikipedia.AllPagesPage, html string,
) errors.E {
	filename := strings.TrimPrefix(page.Title, "File:")
	// First we make sure we do not have spaces.
	filename = strings.ReplaceAll(filename, " ", "_")
	// The first letter has to be upper case.
	filename = wikipedia.FirstUpperCase(filename)

	document, hit, err := wikipedia.GetWikimediaCommonsFile(ctx, globals.Index, esClient, filename)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Msg("file not found")
		return nil
	}

	err = wikipedia.ConvertWikimediaCommonsFileDescription(wikipedia.NameSpaceWikimediaCommonsFile, filename, page, html, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	// TODO: Convert categories found in the page.
	// TODO: Convert templates found in the page.

	err = wikipedia.ConvertPageRedirects(globals.Log, wikipedia.NameSpaceWikimediaCommonsFile, filename, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", filename).Str("title", page.Title).Msg("updating document")
	updateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}
