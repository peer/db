package main

import (
	"context"
	"strings"
	"sync"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	articlesWikipediaNamespace = 0
	filesWikipediaNamespace    = 6
)

var (
	// Set of filenames.
	skippedWikipediaFiles      = sync.Map{}
	skippedWikipediaFilesCount int64
)

type WikipediaFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save filenames of skipped files."`
	URL         string `placeholder:"URL" help:"URL of Wikipedia image table SQL dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaFilesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaImageMetadataRun(client, "enwiki")
		}
	}

	ctx, cancel, httpClient, _, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i interface{}) errors.E {
			return processImage(
				ctx, globals, httpClient, processor, wikipedia.ConvertWikipediaImage,
				&skippedWikipediaFiles, &skippedWikipediaFilesCount, *i.(*wikipedia.Image),
			)
		},
		Progress:    config.Progress,
		Item:        &wikipedia.Image{},
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
	if errE != nil {
		return errE
	}

	errE = saveSkippedMap(c.SaveSkipped, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}

	return nil
}

type WikipediaFileDescriptionsCommand struct {
	SkippedWikipediaFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikipedia files."`
	URL                   string `placeholder:"URL" help:"URL of Wikipedia file descriptions HTML dump to use. It can be a local file path, too. Default: the latest."`
}

//nolint:dupl
func (c *WikipediaFileDescriptionsCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedWikipediaFiles, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaRun(client, "enwiki", filesWikipediaNamespace)
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

// Dump contains descriptions of Wikipedia files and of Wikimedia Commons files (used on Wikipedia).
// We want to use descriptions of just Wikipedia files, so when a file is not found among Wikipedia files,
// we check if it is a Wikimedia Commons file.
func (c *WikipediaFileDescriptionsCommand) isCommonsFile(
	ctx context.Context, esClient *elastic.Client, filename string,
) (bool, errors.E) {
	id := search.GetID(wikipedia.NameSpaceWikimediaCommonsFile, filename)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["doc"] = string(id)
		errors.Details(errE)["file"] = filename
		return false, errE
	} else if !esDoc.Found {
		return false, nil
	}

	return true, nil
}

func (c *WikipediaFileDescriptionsCommand) handleNotFoundError(
	ctx context.Context, globals *Globals, esClient *elastic.Client, article mediawiki.Article, id, filename string,
) errors.E {
	if _, ok := skippedWikipediaFiles.Load(filename); ok {
		globals.Log.Debug().Str("doc", id).Str("file", filename).Str("title", article.Name).Msg("not found skipped file")
		return nil
	}

	commons, err := c.isCommonsFile(ctx, esClient, filename)
	if err != nil {
		details := errors.AllDetails(err)
		details["title"] = article.Name
		globals.Log.Error().Err(err).Fields(details).Msg("error determining if commons file")
	} else if commons {
		globals.Log.Debug().Str("doc", id).Str("file", filename).Str("title", article.Name).Msg("commons file")
	} else {
		globals.Log.Warn().Str("doc", id).Str("file", filename).Str("title", article.Name).Msg("not found")
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

	id := search.GetID(wikipedia.NameSpaceWikipediaFile, filename)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		return c.handleNotFoundError(ctx, globals, esClient, article, string(id), filename)
	} else if err != nil {
		globals.Log.Error().Str("doc", string(id)).Str("file", filename).Str("title", article.Name).Err(err).Send()
		return nil
	} else if !esDoc.Found {
		return c.handleNotFoundError(ctx, globals, esClient, article, string(id), filename)
	}
	var document search.Document
	errE := x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = string(id)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = id

	errE = wikipedia.ConvertWikipediaArticle(&document, wikipedia.NameSpaceWikipediaFile, filename, article)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = string(document.ID)
		details["file"] = filename
		details["title"] = article.Name
		globals.Log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", filename).Str("title", article.Name).Msg("updating document")
	updateDocument(processor, *esDoc.SeqNo, *esDoc.PrimaryTerm, &document)

	return nil
}

type WikipediaArticlesCommand struct {
	SkippedWikidataEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
	URL                     string `placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

//nolint:dupl
func (c *WikipediaArticlesCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedWikidataEntities, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaRun(client, "enwiki", articlesWikipediaNamespace)
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

func (c *WikipediaArticlesCommand) handleNotFoundError(
	globals *Globals, article mediawiki.Article, id string,
) errors.E {
	if _, ok := skippedWikidataEntities.Load(id); ok {
		globals.Log.Debug().Str("doc", id).Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Msg("not found skipped file")
	} else {
		globals.Log.Warn().Str("doc", id).Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Msg("not found")
	}
	return nil
}

// TODO: Skip disambiguation pages (remove corresponding document if we already have it).
func (c *WikipediaArticlesCommand) processArticle(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article,
) errors.E {
	if article.MainEntity == nil {
		globals.Log.Warn().Str("title", article.Name).Msg("article does not have an associated entity")
		return nil
	}
	id := wikipedia.GetWikidataDocumentID(article.MainEntity.Identifier)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		return c.handleNotFoundError(globals, article, string(id))
	} else if err != nil {
		globals.Log.Error().Str("doc", string(id)).Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Err(err).Send()
		return nil
	} else if !esDoc.Found {
		return c.handleNotFoundError(globals, article, string(id))
	}
	var document search.Document
	errE := x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = string(id)
		details["entity"] = article.MainEntity.Identifier
		details["title"] = article.Name
		globals.Log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = id

	errE = wikipedia.ConvertWikipediaArticle(&document, wikipedia.NameSpaceWikidata, article.MainEntity.Identifier, article)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = string(document.ID)
		details["entity"] = article.MainEntity.Identifier
		details["title"] = article.Name
		globals.Log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", article.MainEntity.Identifier).Str("title", article.Name).Msg("updating document")
	updateDocument(processor, *esDoc.SeqNo, *esDoc.PrimaryTerm, &document)

	return nil
}
