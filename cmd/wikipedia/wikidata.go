package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search/internal/wikipedia"
)

var (
	skippedWikidataEntities      = sync.Map{}
	skippedWikidataEntitiesCount int64
)

type WikidataCommand struct {
	SkippedCommonsFiles   string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikimedia Commons files."`
	SkippedWikipediaFiles string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikipedia files."`
	SaveSkipped           string `placeholder:"PATH" type:"path" help:"Save IDs of skipped entities."`
	URL                   string `placeholder:"URL" help:"URL of Wikidata Entities JSON dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikidataCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedCommonsFiles, &skippedCommonsFiles, &skippedCommonsFilesCount)
	if errE != nil {
		return errE
	}

	errE = populateSkippedMap(c.SkippedWikipediaFiles, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}

	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestWikidataEntitiesRun
	}

	ctx, cancel, httpClient, esClient, processor, cache, config, errE := initializeRun(globals, urlFunc, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessWikidataDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, httpClient, esClient, processor, cache, entity)
	})
	if errE != nil {
		return errE
	}

	errE = saveSkippedMap(c.SaveSkipped, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	return nil
}

func (c *WikidataCommand) processEntity(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, esClient *elastic.Client,
	processor *elastic.BulkProcessor, cache *wikipedia.Cache, entity mediawiki.Entity,
) errors.E {
	document, err := wikipedia.ConvertEntity(ctx, httpClient, esClient, cache, &skippedCommonsFiles, entity)
	if errors.Is(err, wikipedia.SkippedError) {
		_, loaded := skippedWikidataEntities.LoadOrStore(entity.ID, true)
		if !loaded {
			atomic.AddInt64(&skippedWikidataEntitiesCount, 1)
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
