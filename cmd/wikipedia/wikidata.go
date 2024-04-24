package main

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/wikipedia"
	"gitlab.com/peerdb/peerdb/store"
)

//nolint:gochecknoglobals
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
// (URL to item page on Wikidata), ITEM (IS claim), ENGLISH_WIKIPEDIA_PAGE_TITLE (Wikipedia page title, without underscores), ENGLISH_WIKIPEDIA_PAGE
// (URL to the Wikipedia page), WIKIMEDIA_COMMONS_PAGE_TITLE (Wikimedia Commons page title, without underscores), WIKIMEDIA_COMMONS_PAGE
// (URL to the Wikimedia Commons page), NAME (for English labels and aliases), DESCRIPTION (for English entity descriptions).
//
// When creating claims referencing other documents it creates an invalid reference storing original Wikidata ID into the _temp field.
// This is because the order of entities in a dump is arbitrary so we first insert all documents and then in PrepareCommand do another
// pass, checking all references and setting true IDs (having Wikidata ID is useful for debugging when reference is invalid).
// References to Wikimedia Commons files are done in a similar fashion, but with a meta claim.
type WikidataCommand struct {
	SaveSkipped string `help:"Save IDs of skipped Wikidata entities."                                                            placeholder:"PATH" type:"path"`
	URL         string `help:"URL of Wikidata entities JSON dump to use. It can be a local file path, too. Default: the latest." placeholder:"URL"`
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

	ctx, stop, _, store, _, esProcessor, cache, config, errE := initializeRun(globals, urlFunc, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

	errE = mediawiki.ProcessWikidataDump(ctx, config, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, store, cache, entity)
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
	ctx context.Context, globals *Globals, store *store.Store[json.RawMessage, json.RawMessage, json.RawMessage], cache *es.Cache, entity mediawiki.Entity,
) errors.E {
	document, errE := wikipedia.ConvertEntity(ctx, globals.Logger, store, cache, wikipedia.NameSpaceWikimediaCommonsFile, entity)
	if errE != nil {
		if errors.Is(errE, wikipedia.ErrSilentSkipped) {
			globals.Logger.Debug().Str("entity", entity.ID).Err(errE).Send()
		} else if errors.Is(errE, wikipedia.ErrSkipped) {
			globals.Logger.Warn().Str("entity", entity.ID).Err(errE).Send()
		} else {
			globals.Logger.Error().Str("entity", entity.ID).Err(errE).Send()
		}
		id := wikipedia.GetWikidataDocumentID(entity.ID)
		_, loaded := skippedWikidataEntities.LoadOrStore(id.String(), true)
		if !loaded {
			atomic.AddInt64(&skippedWikidataEntitiesCount, 1)
		}
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("entity", entity.ID).Msg("saving document")
	errE = peerdb.InsertOrReplaceDocument(ctx, store, document)
	if errE != nil {
		globals.Logger.Error().Str("entity", entity.ID).Err(errE).Send()
		return nil
	}

	return nil
}
