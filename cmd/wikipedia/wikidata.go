package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search/internal/wikipedia"
)

var (
	// Set of document IDs.
	skippedWikidataEntities      = sync.Map{}
	skippedWikidataEntitiesCount int64
)

// WikidataCommand uses Wikidata entities dump as input and creates a document for each entity in the file, mapping Wikidata statements to PeerDB claims.
//
// It skips some entities: those without English label and those items which have a sitelink to Wikipedia, but it is not to an article, template, or category.
//
// Besides claims based on Wikipedia statements, it creates also claims with the following properties: WIKIDATA_PROPERTY_ID (P prefixed ID),
// WIKIDATA_PROPERTY_PAGE (URL to property page on Wikidata), PROPERTY (is claim), WIKIDATA_ITEM_ID (Q prefixed ID), WIKIDATA_ITEM_PAGE
// (URL to item page on Wikidata), ITEM (is claim), ENGLISH_WIKIPEDIA_ARTICLE_TITLE (article title, without underscores), ENGLISH_WIKIPEDIA_ARTICLE
// (URL to the article), ALSO_KNOWN_AS (fot any non-primary English labels), DESCRIPTION (for English Wikidata entity descriptions).
//
// When resolving Wikimedia Commons references it uses existing documents in ElasticSearch to find a corresponding file document and constructs
// a file claim. If it cannot find one, it uses Wikimedia Commons API to determine if the file has been renamed and the reference should be updated.
// These cases are not common so using Wikimedia Commons API should not slow down the overall processing.
//
// When creating claims referencing other documents it just assumes a reference is valid and creates one, storing original Wikidata ID into a name
// for language XX. This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true document names for English language (ID for language XX is useful for debugging when reference is invalid).
type WikidataCommand struct {
	SkippedCommonsFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikimedia Commons files."`
	SaveSkipped         string `placeholder:"PATH" type:"path" help:"Save IDs of skipped entities."`
	URL                 string `placeholder:"URL" help:"URL of Wikidata Entities JSON dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikidataCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedCommonsFiles, &skippedCommonsFiles, &skippedCommonsFilesCount)
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
	document, err := wikipedia.ConvertEntity(ctx, globals.Log, httpClient, esClient, cache, &skippedCommonsFiles, globals.Token, globals.APILimit, entity)
	if err != nil {
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		} else {
			globals.Log.Error().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Send()
		}
		id := wikipedia.GetWikidataDocumentID(entity.ID)
		_, loaded := skippedWikidataEntities.LoadOrStore(string(id), true)
		if !loaded {
			atomic.AddInt64(&skippedWikidataEntitiesCount, 1)
		}
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", entity.ID).Msg("saving document")
	saveDocument(processor, document)

	return nil
}
