package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

var (
	// Set of filenames.
	skippedWikimediaCommonsFiles      = sync.Map{}
	skippedWikimediaCommonsFilesCount int64
)

// CommonsCommand uses Wikimedia Commons entities dump as input and updates corresponding documents for each entity in the dump,
// mapping statements to PeerDB claims.
//
// It expects documents populated by CommonsFilesCommand.
//
// It expects documents populated by WikidataCommand because it might need properties to resolve the data type used
// with the claim's value. It uses ElasticSearch to obtain documents of those properties.
//
// It accesses existing documents in ElasticSearch to load corresponding file's document which is then updated with claims based on
// statements and also claims with the following properties: WIKIMEDIA_COMMONS_ENTITY_ID (M prefixed ID),
// NAME (for any English labels), DESCRIPTION (for English entity descriptions).
//
// When creating claims referencing other documents it creates an invalid reference storing original Wikidata ID into the _id field prefixed with "-".
// This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true IDs (having Wikidata ID is useful for debugging when reference is invalid).
// References to Wikimedia Commons files are done in a similar fashion, but with a meta claim.
type CommonsCommand struct {
	SkippedFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikimedia Commons files."`
	URL          string `placeholder:"URL" help:"URL of Wikimedia Commons entities JSON dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *CommonsCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedFiles, &skippedWikimediaCommonsFiles, &skippedWikimediaCommonsFilesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestCommonsEntitiesRun
	}

	ctx, cancel, _, esClient, processor, cache, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessCommonsEntitiesDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, esClient, cache, processor, entity)
	})
	if errE != nil {
		return errE
	}

	return nil
}

func (c *CommonsCommand) processEntity(
	ctx context.Context, globals *Globals, esClient *elastic.Client, cache *es.Cache, processor *elastic.BulkProcessor, entity mediawiki.Entity,
) errors.E {
	filename := strings.TrimPrefix(entity.Title, "File:")
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = wikipedia.FirstUpperCase(filename)

	if _, ok := skippedWikimediaCommonsFiles.Load(filename); ok {
		globals.Log.Debug().Str("file", filename).Str("entity", entity.ID).Msg("skipped file")
		return nil
	}

	document, hit, err := wikipedia.GetWikimediaCommonsFile(ctx, globals.Index, esClient, filename)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = filename
		details["entity"] = entity.ID
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	additionalDocument, err := wikipedia.ConvertEntity(ctx, globals.Index, globals.Log, esClient, cache, wikipedia.NameSpaceWikimediaCommonsFile, entity)
	if err != nil {
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Str("doc", string(document.ID)).Str("file", filename).Err(err).Str("entity", entity.ID).Fields(errors.AllDetails(err)).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Str("doc", string(document.ID)).Str("file", filename).Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		} else {
			globals.Log.Error().Str("doc", string(document.ID)).Str("file", filename).Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		}
		return nil
	}

	// We remove media type property claims because we populate them ourselves from the image table SQL dump
	// (alongside more metadata). We also determine more detailed media types than what is available here.
	_ = additionalDocument.Remove(wikipedia.GetWikidataDocumentID("P1163"))

	if document.ID != additionalDocument.ID {
		globals.Log.Warn().Str("doc", string(document.ID)).Str("file", filename).Str("entity", entity.ID).
			Str("got", string(additionalDocument.ID)).Msg("document ID mismatch")
	}

	err = document.MergeFrom(additionalDocument)
	if err != nil {
		globals.Log.Error().Str("doc", string(document.ID)).Str("file", filename).Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", filename).Str("entity", entity.ID).Msg("updating document")
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

// CommonsFilesCommand uses Wikimedia Commons images (really files) table SQL dump as input and creates a document for each file in the table.
//
// It creates claims with the following properties (not necessary all of them): WIKIMEDIA_COMMONS_FILE_NAME (just filename, without "File:"
// prefix, but with underscores and file extension), WIKIMEDIA_COMMONS_FILE (URL to file page), FILE_URL (URL to full resolution or raw file),
// FILE (IS claim), MEDIA_TYPE, MEDIAWIKI_MEDIA_TYPE, SIZE (in bytes), PAGE_COUNT, DURATION (in seconds), multiple PREVIEW_URL
// (a list of URLs of previews), WIDTH, HEIGHT, NAME (a filename without file extension and without underscores).
// The idea is that these claims should be enough to populate a file claim (in other documents using these files).
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
	Token       string `placeholder:"TOKEN" env:"WIKIMEDIA_COMMONS_TOKEN" help:"Access token for Wikimedia Commons API. Not required. Environment variable: ${env}."`
	APILimit    int    `placeholder:"INT" default:"${defaultAPILimit}" help:"Maximum number of titles to work on in a single API request. Use 500 if you have an access token with higher limits. Default: ${defaultAPILimit}."` //nolint:lll
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save filenames of skipped Wikimedia Commons files."`
	URL         string `placeholder:"URL" help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest."`
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

	return filesCommandRun(
		globals, urlFunc,
		c.Token, c.APILimit, c.SaveSkipped, &skippedWikimediaCommonsFiles, &skippedWikimediaCommonsFilesCount,
		wikipedia.ConvertWikimediaCommonsImage,
	)
}

// CommonsFileDescriptionsCommand uses Wikimedia Commons API as input to obtain and extract descriptions for files (namespace 6)
// and adds file's description to a corresponding file document.
//
// It expects documents populated by CommonsFilesCommand.
//
// File articles contain a lot of metadata which we do not yet extract, but extract only a HTML description. It is expected that the
// rest of metadata is available through Wikimedia Commons entities or similar structured data. Extracted HTML descriptions are processed
// so that HTML can be directly displayed alongside other content. Use of Wikipedia's CSS nor Javascript is not needed after processing.
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding Wikimedia Commons entity's document which is then updated with
// claims with the following properties: WIKIMEDIA_COMMONS_PAGE_ID (internal page ID of the file), DESCRIPTION (potentially multiple),
// NAME (from redirects pointing to the file), IN_WIKIMEDIA_COMMONS_CATEGORY (for categories the file is in),
// USES_WIKIMEDIA_COMMONS_TEMPLATE (for templates used).
type CommonsFileDescriptionsCommand struct {
	SkippedFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikimedia Commons files."`
}

func (c *CommonsFileDescriptionsCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedFiles, &skippedWikimediaCommonsFiles, &skippedWikimediaCommonsFilesCount)
	if errE != nil {
		return errE
	}

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
		return wikipedia.ListAllPages(ctx, httpClient, []int{filesWikipediaNamespace}, "commons.wikimedia.org", limiter, pages)
	})

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, 0, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			globals.Log.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("count", count.Count()).
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

				errE = c.processPage(ctx, globals, esClient, processor, page, html)
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
	ctx context.Context, globals *Globals, esClient *elastic.Client,
	processor *elastic.BulkProcessor, page wikipedia.AllPagesPage, html string,
) errors.E {
	filename := strings.TrimPrefix(page.Title, "File:")
	// First we make sure we do not have spaces.
	filename = strings.ReplaceAll(filename, " ", "_")
	// The first letter has to be upper case.
	filename = wikipedia.FirstUpperCase(filename)

	if _, ok := skippedWikimediaCommonsFiles.Load(filename); ok {
		globals.Log.Debug().Str("file", filename).Str("title", page.Title).Msg("skipped file")
		return nil
	}

	document, hit, err := wikipedia.GetWikimediaCommonsFile(ctx, globals.Index, esClient, filename)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.SetPageID(wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page.Identifier, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertFileDescription(wikipedia.NameSpaceWikimediaCommonsFile, "FROM_WIKIMEDIA_COMMONS", filename, html, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageInCategories(globals.Log, wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageUsedTemplates(globals.Log, wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

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
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

// CommonsCategoriesCommand uses Wikimedia Commons API as input to obtain and extract descriptions for categories (namespace 14) from their
// Wikimedia Commons articles and adds category's description to a corresponding Wikidata entity.
//
// It expects documents populated by WikidataCommand.
//
// Category articles generally have a very short description of a category, if at all. This command extracts the HTML description
// which is processed so that HTML can be directly displayed alongside other content. Use of Wikipedia's CSS nor Javascript is not
// needed after processing.
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding Wikidata entity's document which is then updated with claims with the
// following properties: WIKIMEDIA_COMMONS_PAGE_ID (internal page ID of the category), DESCRIPTION (extracted from Wikimedia Commons' category article),
// NAME (from redirects pointing to the category), IN_WIKIMEDIA_COMMONS_CATEGORY (for categories the category is in),
// USES_WIKIMEDIA_COMMONS_TEMPLATE (for templates used).
type CommonsCategoriesCommand struct {
	SkippedEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
}

func (c *CommonsCategoriesCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedEntities, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

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
		return wikipedia.ListAllPages(ctx, httpClient, []int{categoriesWikipediaNamespace}, "commons.wikimedia.org", limiter, pages)
	})

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, 0, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			globals.Log.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("count", count.Count()).
				Str("elapsed", p.Elapsed.Truncate(time.Second).String()).
				Send()
		}
	}()

	for i := 0; i < int(rateLimit); i++ {
		g.Go(func() error {
			// Loop ends with pages is closed, which happens when context is cancelled, too.
			for page := range pages {
				if page.Properties["wikibase_item"] == "" {
					globals.Log.Debug().Str("title", page.Title).Msg("category without Wikidata item")
					continue
				}

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

				errE = c.processPage(ctx, globals, esClient, processor, page, html)
				if errE != nil {
					return errE
				}
			}
			return nil
		})
	}

	return errors.WithStack(g.Wait())
}

func (c *CommonsCategoriesCommand) processPage(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, page wikipedia.AllPagesPage, html string,
) errors.E {
	// We know this is available because we check before calling this method.
	id := page.Properties["wikibase_item"]

	if _, ok := skippedWikidataEntities.Load(string(wikipedia.GetWikidataDocumentID(id))); ok {
		globals.Log.Debug().Str("entity", id).Str("title", page.Title).Msg("skipped entity")
		return nil
	}

	document, hit, err := wikipedia.GetWikidataItem(ctx, globals.Index, esClient, id)
	if err != nil {
		details := errors.AllDetails(err)
		details["entity"] = id
		details["title"] = page.Title
		if errors.Is(err, wikipedia.NotFoundError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		return nil
	}

	err = wikipedia.SetPageID(wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page.Identifier, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertCategoryDescription(id, "FROM_WIKIMEDIA_COMMONS", html, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageInCategories(globals.Log, wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageUsedTemplates(globals.Log, wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageRedirects(globals.Log, wikipedia.NameSpaceWikidata, id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", id).Str("title", page.Title).Msg("updating document")
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

// CommonsTemplatesCommand uses Wikimedia Commons API as input to obtain and extract descriptions for templates (namespace 10) and modules
// (namespace 828) from their documentation and adds template's or module's description to a corresponding Wikidata entity.
//
// It expects documents populated by WikidataCommand.
//
// Documentation is obtained from template's or module's documentation subpage, if it exists, otherwise from the template's or modules page itself.
// Documentation generally has a very short description of a template, if at all. This command extracts the HTML description which is processed so
// that HTML can be directly displayed alongside other content. Use of Wikipedia's CSS nor Javascript is not needed after processing.
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding Wikidata entity's document which is then updated with claims with the
// following properties: WIKIMEDIA_COMMONS_PAGE_ID (internal page ID of the template or module), DESCRIPTION (extracted from documentation),
// NAME (from redirects pointing to the template or module), IN_WIKIMEDIA_COMMONS_CATEGORY (for categories the template or module is in),
// USES_WIKIMEDIA_COMMONS_TEMPLATE (for templates used).
type CommonsTemplatesCommand struct {
	SkippedEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
}

func (c *CommonsTemplatesCommand) Run(globals *Globals) errors.E {
	return templatesCommandRun(globals, "commons.wikimedia.org", c.SkippedEntities, "WIKIMEDIA_COMMONS", "FROM_WIKIMEDIA_COMMONS")
}
