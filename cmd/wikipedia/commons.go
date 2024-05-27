package main

import (
	"context"
	"encoding/json"
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

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/internal/wikipedia"
	"gitlab.com/peerdb/peerdb/store"
)

//nolint:gochecknoglobals
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
// When creating claims referencing other documents it creates an invalid reference storing original Wikidata ID into the _temp field.
// This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true IDs (having Wikidata ID is useful for debugging when reference is invalid).
// References to Wikimedia Commons files are done in a similar fashion, but with a meta claim.
type CommonsCommand struct {
	SkippedFiles string `help:"Load filenames of skipped Wikimedia Commons files."                                                         placeholder:"PATH" type:"path"`
	URL          string `help:"URL of Wikimedia Commons entities JSON dump to use. It can be a local file path, too. Default: the latest." placeholder:"URL"`
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

	ctx, stop, _, store, esClient, esProcessor, cache, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

	errE = mediawiki.ProcessCommonsEntitiesDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, store, esClient, cache, entity)
	})
	if errE != nil {
		return errE
	}

	return nil
}

func (c *CommonsCommand) processEntity(
	ctx context.Context, globals *Globals,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, cache *es.Cache, entity mediawiki.Entity,
) errors.E {
	filename := strings.TrimPrefix(entity.Title, "File:")
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = wikipedia.FirstUpperCase(filename)

	if _, ok := skippedWikimediaCommonsFiles.Load(filename); ok {
		globals.Logger.Debug().Str("file", filename).Str("entity", entity.ID).Msg("skipped file")
		return nil
	}

	document, version, errE := wikipedia.GetWikimediaCommonsFile(ctx, store, globals.Elastic.Index, esClient, filename)
	if errE != nil {
		details := errors.Details(errE)
		details["file"] = filename
		details["entity"] = entity.ID
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	additionalDocument, errE := wikipedia.ConvertEntity(ctx, globals.Logger, store, cache, wikipedia.NameSpaceWikimediaCommonsFile, entity)
	if errE != nil {
		if errors.Is(errE, wikipedia.ErrSilentSkipped) {
			globals.Logger.Debug().Str("doc", document.ID.String()).Str("file", filename).Err(errE).Str("entity", entity.ID).Send()
		} else if errors.Is(errE, wikipedia.ErrSkipped) {
			globals.Logger.Warn().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).Err(errE).Send()
		} else {
			globals.Logger.Error().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).Err(errE).Send()
		}
		return nil
	}

	// We remove media type property claims because we populate them ourselves from the image table SQL dump
	// (alongside more metadata). We also determine more detailed media types than what is available here.
	_ = additionalDocument.Remove(wikipedia.GetWikidataDocumentID("P1163"))

	if document.ID != additionalDocument.ID {
		globals.Logger.Warn().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).
			Str("got", additionalDocument.ID.String()).Msg("document ID mismatch")
	}

	errE = document.MergeFrom(additionalDocument)
	if errE != nil {
		globals.Logger.Error().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).Err(errE).Send()
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).Msg("updating document")
	errE = peerdb.UpdateDocument(ctx, store, document, version)
	if errE != nil {
		globals.Logger.Error().Str("doc", document.ID.String()).Str("file", filename).Str("entity", entity.ID).Err(errE).Send()
		return nil
	}

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
//
//nolint:lll
type CommonsFilesCommand struct {
	Token       string `                             env:"WIKIMEDIA_COMMONS_TOKEN" help:"Access token for Wikimedia Commons API. Not required. Environment variable: ${env}."                                                               placeholder:"TOKEN"`
	APILimit    int    `default:"${defaultAPILimit}"                               help:"Maximum number of titles to work on in a single API request. Use 500 if you have an access token with higher limits. Default: ${defaultAPILimit}." placeholder:"INT"` //nolint:lll
	SaveSkipped string `                                                           help:"Save filenames of skipped Wikimedia Commons files."                                                                                                placeholder:"PATH"  type:"path"`
	URL         string `                                                           help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest."                                      placeholder:"URL"`
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
	SkippedFiles string `help:"Load filenames of skipped Wikimedia Commons files." placeholder:"PATH" type:"path"`
}

func (c *CommonsFileDescriptionsCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedFiles, &skippedWikimediaCommonsFiles, &skippedWikimediaCommonsFilesCount)
	if errE != nil {
		return errE
	}

	ctx, stop, httpClient, store, esClient, esProcessor, _, _, errE := initializeRun(globals, nil, nil)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

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
			stats := esProcessor.Stats()
			globals.Logger.Info().
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
					globals.Logger.Error().Err(errE).Send()
					continue
				}

				count.Increment()

				errE = c.processPage(ctx, globals, store, esClient, page, html)
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
	ctx context.Context, globals *Globals,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, page wikipedia.AllPagesPage, html string,
) errors.E { //nolint:unparam
	filename := strings.TrimPrefix(page.Title, "File:")
	// First we make sure we do not have spaces.
	filename = strings.ReplaceAll(filename, " ", "_")
	// The first letter has to be upper case.
	filename = wikipedia.FirstUpperCase(filename)

	if _, ok := skippedWikimediaCommonsFiles.Load(filename); ok {
		globals.Logger.Debug().Str("file", filename).Str("title", page.Title).Msg("skipped file")
		return nil
	}

	document, version, errE := wikipedia.GetWikimediaCommonsFile(ctx, store, globals.Elastic.Index, esClient, filename)
	if errE != nil {
		details := errors.Details(errE)
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.SetPageID(wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page.Identifier, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertFileDescription(wikipedia.NameSpaceWikimediaCommonsFile, "FROM_WIKIMEDIA_COMMONS", filename, html, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageInCategories(globals.Logger, wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageUsedTemplates(globals.Logger, wikipedia.NameSpaceWikimediaCommonsFile, "WIKIMEDIA_COMMONS", filename, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageRedirects(globals.Logger, wikipedia.NameSpaceWikimediaCommonsFile, filename, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["file"] = filename
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("file", filename).Str("title", page.Title).Msg("updating document")
	errE = peerdb.UpdateDocument(ctx, store, document, version)
	if errE != nil {
		globals.Logger.Error().Str("doc", document.ID.String()).Str("file", filename).Str("title", page.Title).Err(errE).Send()
		return nil
	}

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
	SkippedEntities string `help:"Load IDs of skipped Wikidata entities." placeholder:"PATH" type:"path"`
}

func (c *CommonsCategoriesCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedEntities, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	ctx, stop, httpClient, store, esClient, esProcessor, _, _, errE := initializeRun(globals, nil, nil)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

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
			stats := esProcessor.Stats()
			globals.Logger.Info().
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
					globals.Logger.Debug().Str("title", page.Title).Msg("category without Wikidata item")
					continue
				}

				err := limiter.Wait(ctx)
				if err != nil {
					// Context has been canceled.
					return errors.WithStack(err)
				}

				html, errE := wikipedia.GetPageHTML(ctx, httpClient, "commons.wikimedia.org", page.Title)
				if errE != nil {
					globals.Logger.Error().Err(errE).Send()
					continue
				}

				count.Increment()

				errE = c.processPage(ctx, globals, store, esClient, page, html)
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
	ctx context.Context, globals *Globals,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, page wikipedia.AllPagesPage, html string,
) errors.E { //nolint:unparam
	// We know this is available because we check before calling this method.
	id := page.Properties["wikibase_item"]

	if _, ok := skippedWikidataEntities.Load(wikipedia.GetWikidataDocumentID(id).String()); ok {
		globals.Logger.Debug().Str("entity", id).Str("title", page.Title).Msg("skipped entity")
		return nil
	}

	document, version, errE := wikipedia.GetWikidataItem(ctx, store, globals.Elastic.Index, esClient, id)
	if errE != nil {
		details := errors.Details(errE)
		details["entity"] = id
		details["title"] = page.Title
		if errors.Is(errE, wikipedia.ErrNotFound) {
			globals.Logger.Warn().Err(errE).Send()
		} else {
			globals.Logger.Error().Err(errE).Send()
		}
		return nil
	}

	// Page title might not be in English so we do NOT add it as NAME claim on the document when processing
	// Wikidata entities (we have it there through site links on Wikidata entities), nor we add it here,
	// but we rely on Wikidata entities' labels only.

	errE = wikipedia.SetPageID(wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page.Identifier, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertCategoryDescription(id, "FROM_WIKIMEDIA_COMMONS", html, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageInCategories(globals.Logger, wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageUsedTemplates(globals.Logger, wikipedia.NameSpaceWikidata, "WIKIMEDIA_COMMONS", id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageRedirects(globals.Logger, wikipedia.NameSpaceWikidata, id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("entity", id).Str("title", page.Title).Msg("updating document")
	errE = peerdb.UpdateDocument(ctx, store, document, version)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

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
	SkippedEntities string `help:"Load IDs of skipped Wikidata entities." placeholder:"PATH" type:"path"`
}

func (c *CommonsTemplatesCommand) Run(globals *Globals) errors.E {
	return templatesCommandRun(globals, "commons.wikimedia.org", c.SkippedEntities, "WIKIMEDIA_COMMONS", "FROM_WIKIMEDIA_COMMONS")
}
