package main

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	articlesWikipediaNamespace   = 0
	filesWikipediaNamespace      = 6
	templatesWikipediaNamespace  = 10
	categoriesWikipediaNamespace = 14
	modulesWikipediaNamespace    = 828

	// See: https://phabricator.wikimedia.org/T307610
	// TODO: Why we have to use 500 here instead of 1000 to not hit the rate limit?
	wikipediaRESTRateLimit  = 500
	wikipediaRESTRatePeriod = 10 * time.Second
)

var (
	redirectRegex         = regexp.MustCompile(`(?i)#REDIRECT\s+\[\[`)
	wiktionaryRegex       = regexp.MustCompile(`(?i)\{\{(wiktionary redirect|WiktionaryRedirect|Wiktionary-redirect|wi(\||\}\})|wtr(\||\}\}))`)
	wikispeciesRegex      = regexp.MustCompile(`(?i)\{\{(wikispecies redirect)`)
	wikimediaCommonsRegex = regexp.MustCompile(`(?i)\{\{(Wikimedia Commons redirect|commons redirect)`)
)

var (
	// Set of filenames.
	skippedWikipediaFiles      = sync.Map{}
	skippedWikipediaFilesCount int64
)

// TODO: Files uploaded to Wikipedia are moved to Wikimedia Commons. We should make sure we do not have duplicate files.
//       For example, if file exists in Wikipedia dump but was then moved to Wikimedia Commons and exists in its dump as well.

// WikipediaFilesCommand uses English Wikipedia images (really files) table SQL dump as input and creates a document for each file in the table.
//
// It creates claims with the following properties (not necessary all of them): ENGLISH_WIKIPEDIA_FILE_NAME (just filename, without "File:"
// prefix, but with underscores and file extension), ENGLISH_WIKIPEDIA_FILE (URL to file page), FILE_URL (URL to full resolution or raw file),
// FILE (is claim), MEDIA_TYPE, MEDIAWIKI_MEDIA_TYPE, SIZE (in bytes), PAGE_COUNT, DURATION (in seconds), multiple PREVIEW_URL
// (a list of URLs of previews), WIDTH, HEIGHT, NAME (a filename without file extension and without underscores).
// The idea is that these claims should be enough to populate a file claim (in other documents using these files).
//
// Most files used on English Wikipedia are from Wikimedia Commons, but some are not for copyright reasons (e.g., you can use a copyrighted
// image on Wikipedia as fair use, but that is not acceptable on Wikimedia Commons). This command processes files only on English Wikipedia.
//
// For some files (primarily PDFs and DJVU files) metadata is not stored in the SQL table but SQL table only contains a reference to additional
// blob storage (see: https://phabricator.wikimedia.org/T301039). Because of that this command uses English Wikipedia API to obtain metadata
// for those files. This introduces some issues. API is rate limited, so processing can be slower than pure offline processing would be (configuring
// high ItemsProcessingThreads can mitigate this somewhat, so that while some threads are blocked on API, other threads can continue to process other
// files which do not require API). There can be discrepancies between the table state and what is available through the API: files from the table might
// be deleted since the table dump has been made. On the other hand, metadata rarely changes (only if metadata is re-extracted/re-computed, or if a new version
// of a file has been uploaded) so the fact that metadata might be from a different file revision does not seem to be too problematic here. We anyway
// want the latest information about files because we directly use files hosted on English Wikipedia by displaying them, so if they are changed or deleted,
// we want to know that (otherwise we could try to display an image which does not exist anymore, which would fail to load).
type WikipediaFilesCommand struct {
	Token       string `placeholder:"TOKEN" env:"WIKIPEDIA_TOKEN" help:"Access token for Wikipedia API. Not required. Environment variable: ${env}."`
	APILimit    int    `placeholder:"INT" default:"${defaultAPILimit}" help:"Maximum number of titles to work on in a single API request. Use 500 if you have an access token with higher limits. Default: ${defaultAPILimit}."` //nolint:lll
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save filenames of skipped Wikipedia files."`
	URL         string `placeholder:"URL" help:"URL of Wikipedia image table SQL dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaFilesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(ctx context.Context, client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaImageMetadataRun(ctx, client, "enwiki")
		}
	}

	return filesCommandRun(
		globals, urlFunc,
		c.Token, c.APILimit, c.SaveSkipped, &skippedWikipediaFiles, &skippedWikipediaFilesCount,
		wikipedia.ConvertWikipediaImage)
}

// WikipediaFileDescriptionsCommand uses Wikipedia file descriptions HTML dump (namespace 6) as input and adds file's description
// to a corresponding file document.
//
// It expects documents populated by WikipediaFilesCommand.
//
// File articles contain a lot of metadata which we do not yet extract, but extract only a HTML description. It is expected that the
// rest of metadata will be available through Wikimedia Commons entities or similar structured data. Extracted HTML descriptions are processed
// so that HTML can be directly displayed alongside other content. Use of Wikipedia's CSS nor Javascript is not needed after processing.
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding file's document which is then updated with claims with the
// following properties: ENGLISH_WIKIPEDIA_PAGE_ID (internal page ID of the file), DESCRIPTION (potentially multiple),
// NAME (from redirects pointing to the file), IN_ENGLISH_WIKIPEDIA_CATEGORY (for categories the file is in),
// USES_ENGLISH_WIKIPEDIA_TEMPLATE (for templates used).
type WikipediaFileDescriptionsCommand struct {
	SkippedFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikipedia files."`
	URL          string `placeholder:"URL" help:"URL of Wikipedia file descriptions HTML dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaFileDescriptionsCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedFiles, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(ctx context.Context, client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaRun(ctx, client, "enwiki", filesWikipediaNamespace)
		}
	}

	ctx, cancel, _, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessWikipediaDump(ctx, config, func(ctx context.Context, article mediawiki.Article) errors.E {
		return c.processArticle(ctx, globals, esClient, processor, article)
	})
	if errE != nil {
		return errE
	}

	return nil
}

func (c *WikipediaFileDescriptionsCommand) processArticle(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article,
) errors.E {
	filename := strings.TrimPrefix(article.Name, "File:")
	// First we make sure we do not have spaces.
	filename = strings.ReplaceAll(filename, " ", "_")
	// The first letter has to be upper case.
	filename = wikipedia.FirstUpperCase(filename)

	if _, ok := skippedWikipediaFiles.Load(filename); ok {
		globals.Log.Debug().Str("file", filename).Str("title", article.Name).Msg("skipped file")
		return nil
	}

	// Dump contains descriptions of Wikipedia files and of Wikimedia Commons files (used on Wikipedia).
	// We want to use descriptions of just Wikipedia files, so when a file is not found among Wikipedia files,
	// we check if it is a Wikimedia Commons file.
	document, hit, err := wikipedia.GetWikipediaFile(ctx, globals.Index, esClient, filename)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = filename
		details["title"] = article.Name
		if errors.Is(err, wikipedia.WikimediaCommonsFileError) {
			globals.Log.Debug().Err(err).Fields(details).Send()
		} else if errors.Is(err, wikipedia.NotFoundError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		return nil
	}

	err = wikipedia.SetPageID(wikipedia.NameSpaceWikipediaFile, "ENGLISH_WIKIPEDIA", filename, article.Identifier, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertFileDescription(wikipedia.NameSpaceWikipediaFile, "FROM_ENGLISH_WIKIPEDIA", filename, article.ArticleBody.HTML, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleInCategories(globals.Log, wikipedia.NameSpaceWikipediaFile, "ENGLISH_WIKIPEDIA", filename, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleUsedTemplates(globals.Log, wikipedia.NameSpaceWikipediaFile, "ENGLISH_WIKIPEDIA", filename, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleRedirects(globals.Log, wikipedia.NameSpaceWikipediaFile, filename, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", filename).Str("title", article.Name).Msg("updating document")
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

func wikipediaArticlesRun(
	globals *Globals, skippedWikidataEntitiesPath, url string, namespace int,
	convertArticle func(string, string, *search.Document) errors.E,
) errors.E {
	errE := populateSkippedMap(skippedWikidataEntitiesPath, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if url != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return url, nil
		}
	} else {
		urlFunc = func(ctx context.Context, client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaRun(ctx, client, "enwiki", namespace)
		}
	}

	ctx, cancel, _, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessWikipediaDump(ctx, config, func(ctx context.Context, article mediawiki.Article) errors.E {
		return wikipediaArticlesProcessArticle(ctx, globals, esClient, processor, article, convertArticle)
	})
	if errE != nil {
		return errE
	}

	return nil
}

func wikipediaArticlesProcessArticle(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article,
	convertArticle func(string, string, *search.Document) errors.E,
) errors.E {
	if article.MainEntity == nil {
		if redirectRegex.MatchString(article.ArticleBody.WikiText) {
			globals.Log.Debug().Str("title", article.Name).Msg("article does not have an associated entity: redirect")
		} else if wiktionaryRegex.MatchString(article.ArticleBody.WikiText) {
			globals.Log.Debug().Str("title", article.Name).Msg("article does not have an associated entity: wiktionary")
		} else if wikispeciesRegex.MatchString(article.ArticleBody.WikiText) {
			globals.Log.Debug().Str("title", article.Name).Msg("article does not have an associated entity: wikispecies")
		} else if wikimediaCommonsRegex.MatchString(article.ArticleBody.WikiText) {
			globals.Log.Debug().Str("title", article.Name).Msg("article does not have an associated entity: wikimedia commons")
		} else {
			globals.Log.Warn().Str("title", article.Name).Msg("article does not have an associated entity")
		}
		return nil
	}

	if _, ok := skippedWikidataEntities.Load(string(wikipedia.GetWikidataDocumentID(article.MainEntity.Identifier))); ok {
		globals.Log.Debug().Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Msg("skipped entity")
		return nil
	}

	document, hit, err := wikipedia.GetWikidataItem(ctx, globals.Index, esClient, article.MainEntity.Identifier)
	if err != nil {
		details := errors.AllDetails(err)
		details["entity"] = article.MainEntity.Identifier
		details["title"] = article.Name
		if errors.Is(err, wikipedia.NotFoundError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		return nil
	}

	id := article.MainEntity.Identifier

	err = wikipedia.SetPageID(wikipedia.NameSpaceWikidata, "ENGLISH_WIKIPEDIA", id, article.Identifier, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = convertArticle(id, article.ArticleBody.HTML, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleInCategories(globals.Log, wikipedia.NameSpaceWikidata, "ENGLISH_WIKIPEDIA", id, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleUsedTemplates(globals.Log, wikipedia.NameSpaceWikidata, "ENGLISH_WIKIPEDIA", id, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertArticleRedirects(globals.Log, wikipedia.NameSpaceWikidata, id, article, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Msg("updating document")
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

// WikipediaArticlesCommand uses Wikipedia articles HTML dump (namespace 0) as input and adds Wikipedia article's body to a
// corresponding Wikidata entity.
//
// It expects documents populated by WikidataCommand.
//
// Most Wikidata entities do not have Wikipedia articles, but many do and this command adds a HTML body of the article to each of them,
// serving as the main field to do full-text search on. It does some heavy processing of the HTML itself so that HTML can be directly displayed
// alongside other content. Use of Wikipedia's CSS nor Javascript is not needed after processing. It removes infoboxes and banners as the
// intend is that the same information is available through structured data (although this is not yet true). It removes references, citations,
// and inline comments (e.g., "citation needed") as the intend is that they are exposed through annotations (pending as well). From the body of
// the article it extracts also a summary (generally few paragraphs at the beginning of the article).
//
// Internal links inside HTML are not yet converted to links to PeerDB documents. This is done in PrepareCommand.
//
// It accesses existing documents in ElasticSearch to load corresponding Wikidata entity's document which is then updated with claims with the
// following properties: ARTICLE (body of the article), HAS_ARTICLE (a label), ENGLISH_WIKIPEDIA_PAGE_ID (internal page ID of the article),
// DESCRIPTION (a summary, with higher confidence than Wikidata's description), NAME (from redirects pointing to the article),
// IN_ENGLISH_WIKIPEDIA_CATEGORY (for categories the article is in), USES_ENGLISH_WIKIPEDIA_TEMPLATE (for templates used).
type WikipediaArticlesCommand struct {
	SkippedEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
	URL             string `placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaArticlesCommand) Run(globals *Globals) errors.E {
	// TODO: Skip disambiguation pages (remove corresponding document if we already have it).
	return wikipediaArticlesRun(globals, c.SkippedEntities, c.URL, articlesWikipediaNamespace, wikipedia.ConvertWikipediaArticle)
}

// WikipediaCategoriesCommand uses Wikipedia categories HTML dump (namespace 14) as input and extracts descriptions from their Wikipedia articles and
// adds category's description to a corresponding Wikidata entity.
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
// following properties: ENGLISH_WIKIPEDIA_PAGE_ID (internal page ID of the article), DESCRIPTION (extracted from Wikipedia's category article),
// NAME (from redirects pointing to the category), IN_ENGLISH_WIKIPEDIA_CATEGORY (for categories the category is in),
// USES_ENGLISH_WIKIPEDIA_TEMPLATE (for templates used).
type WikipediaCategoriesCommand struct {
	SkippedEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
	URL             string `placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaCategoriesCommand) Run(globals *Globals) errors.E {
	return wikipediaArticlesRun(globals, c.SkippedEntities, c.URL, categoriesWikipediaNamespace, func(id, html string, document *search.Document) errors.E {
		return wikipedia.ConvertCategoryDescription(id, "FROM_ENGLISH_WIKIPEDIA", html, document)
	})
}

// WikipediaTemplatesCommand uses Wikipedia API as input to obtain and extract descriptions for templates (namespace 10) and modules (namespace 828)
// from their documentation and adds template's or module's description to a corresponding Wikidata entity.
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
// following properties: ENGLISH_WIKIPEDIA_PAGE_ID (internal page ID of the template or module), DESCRIPTION (extracted from documentation),
// NAME (from redirects pointing to the template or module), IN_ENGLISH_WIKIPEDIA_CATEGORY (for categories the template or module is in),
// USES_ENGLISH_WIKIPEDIA_TEMPLATE (for templates used).
type WikipediaTemplatesCommand struct {
	SkippedEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
}

func (c *WikipediaTemplatesCommand) Run(globals *Globals) errors.E {
	return templatesCommandRun(globals, "en.wikipedia.org", c.SkippedEntities, "ENGLISH_WIKIPEDIA", "FROM_ENGLISH_WIKIPEDIA")
}
