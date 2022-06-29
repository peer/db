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

// WikidataCommand uses Wikidata entities dump as input and creates a document for each entity in the file, mapping statements to PeerDB claims.
//
// It skips some entities: those without English label and those items which have a sitelink to Wikipedia, but it is not to an article, template, or category.
//
// Besides claims based on statements, it creates also claims with the following properties: WIKIDATA_PROPERTY_ID (P prefixed ID),
// WIKIDATA_PROPERTY_PAGE (URL to property page on Wikidata), PROPERTY (IS claim), WIKIDATA_ITEM_ID (Q prefixed ID), WIKIDATA_ITEM_PAGE
// (URL to item page on Wikidata), ITEM (IS claim), ENGLISH_WIKIPEDIA_ARTICLE_TITLE (article title, without underscores), ENGLISH_WIKIPEDIA_ARTICLE
// (URL to the article), ALSO_KNOWN_AS (for any non-first English labels), DESCRIPTION (for English entity descriptions).
// Name of the document is the first English label.
//
// When creating claims referencing other documents it just assumes a reference is valid and creates one, storing original Wikidata ID into a name
// for language XX. This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true document names for English language (ID for language XX is useful for debugging when reference is invalid).
// References to Wikimedia Commons files are done in a similar fashion, but with a meta claim.
type WikidataCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped entities."`
	URL         string `placeholder:"URL" help:"URL of Wikidata entities JSON dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikidataCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ context.Context, _ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ context.Context, _ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestWikidataEntitiesRun
	}

	ctx, cancel, _, _, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.ProcessWikidataDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, processor, entity)
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
	ctx context.Context, globals *Globals, processor *elastic.BulkProcessor, entity mediawiki.Entity,
) errors.E {
	document, err := wikipedia.ConvertEntity(ctx, globals.Log, wikipedia.NameSpaceWikidata, entity)
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
	saveDocument(processor, globals.Index, document)

	return nil
}
