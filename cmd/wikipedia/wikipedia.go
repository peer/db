package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

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
	skippedWikipediaFiles                 = sync.Map{}
	skippedWikipediaFilesCount            int64
	skippedWikipediaFileDescriptions      = sync.Map{}
	skippedWikipediaFileDescriptionsCount int64
	skippedWikipediaArticles              = sync.Map{}
	skippedWikipediaArticlesCount         int64
)

type WikipediaFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped files."`
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
			return c.processImage(ctx, globals, httpClient, processor, *i.(*wikipedia.Image))
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

func (c *WikipediaFilesCommand) processImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, processor *elastic.BulkProcessor, image wikipedia.Image,
) errors.E {
	document, err := wikipedia.ConvertWikipediaImage(ctx, httpClient, image)
	if errors.Is(err, wikipedia.SkippedError) {
		_, loaded := skippedWikipediaFiles.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(&skippedWikipediaFilesCount, 1)
		}
		if !errors.Is(err, wikipedia.SilentSkippedError) {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		}
		return nil
	} else if err != nil {
		return err
	}

	saveDocument(globals, processor, document)

	return nil
}

type WikipediaFileDescriptionsCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped file descriptions."`
	URL         string `placeholder:"URL" help:"URL of Wikipedia file descriptions HTML dump to use. It can be a local file path, too. Default: the latest."`
}

//nolint:dupl
func (c *WikipediaFileDescriptionsCommand) Run(globals *Globals) errors.E {
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

	ctx, cancel, _, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikipediaFileDescriptionsCount)
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

	errE = saveSkippedMap(c.SaveSkipped, &skippedWikipediaFileDescriptions, &skippedWikipediaFileDescriptionsCount)
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

	id := search.GetID(wikipedia.NameSpaceWikipediaFile, filename)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		fmt.Fprintf(os.Stderr, "document %s for file \"%s\" not found\n", id, article.Name)
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting document %s for file \"%s\": %s\n", id, article.Name, err.Error())
		return nil
	} else if !esDoc.Found {
		fmt.Fprintf(os.Stderr, "document %s for file \"%s\" not found\n", id, article.Name)
		return nil
	}
	var document search.Document
	err = x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error JSON decoding document %s for file \"%s\": %s", id, article.Name, err.Error())
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = id

	errE := wikipedia.ConvertWikipediaArticle(&document, wikipedia.NameSpaceWikipediaFile, filename, article)
	if errors.Is(errE, wikipedia.SkippedError) {
		_, loaded := skippedWikipediaFileDescriptions.LoadOrStore(article.Name, true)
		if !loaded {
			atomic.AddInt64(&skippedWikipediaFileDescriptionsCount, 1)
		}
		if !errors.Is(errE, wikipedia.SilentSkippedError) {
			fmt.Fprintf(os.Stderr, "%s\n", errE.Error())
		}
		return nil
	} else if errE != nil {
		return errE
	}

	updateDocument(globals, processor, *esDoc.SeqNo, *esDoc.PrimaryTerm, &document)

	return nil
}

type WikipediaArticlesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped articles."`
	URL         string `placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

//nolint:dupl
func (c *WikipediaArticlesCommand) Run(globals *Globals) errors.E {
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

	ctx, cancel, _, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikipediaArticlesCount)
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

	errE = saveSkippedMap(c.SaveSkipped, &skippedWikipediaArticles, &skippedWikipediaArticlesCount)
	if errE != nil {
		return errE
	}

	return nil
}

// TODO: Skip disambiguation pages (remove corresponding document if we already have it).
func (c *WikipediaArticlesCommand) processArticle(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article,
) errors.E {
	if article.MainEntity == nil {
		_, loaded := skippedWikipediaArticles.LoadOrStore(article.Name, true)
		if !loaded {
			atomic.AddInt64(&skippedWikipediaArticlesCount, 1)
		}
		fmt.Fprintf(os.Stderr, "article \"%s\" does not have an associated entity\n", article.Name)
		return nil
	}
	id := wikipedia.GetWikidataDocumentID(article.MainEntity.Identifier)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	} else if !esDoc.Found {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	}
	var document search.Document
	err = x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error JSON decoding document %s for entity %s for article \"%s\": %s", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = id

	errE := wikipedia.ConvertWikipediaArticle(&document, wikipedia.NameSpaceWikidata, article.MainEntity.Identifier, article)
	if errors.Is(errE, wikipedia.SkippedError) {
		_, loaded := skippedWikipediaArticles.LoadOrStore(article.Name, true)
		if !loaded {
			atomic.AddInt64(&skippedWikipediaArticlesCount, 1)
		}
		if !errors.Is(errE, wikipedia.SilentSkippedError) {
			fmt.Fprintf(os.Stderr, "%s\n", errE.Error())
		}
		return nil
	} else if errE != nil {
		return errE
	}

	updateDocument(globals, processor, *esDoc.SeqNo, *esDoc.PrimaryTerm, &document)

	return nil
}
