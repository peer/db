package main

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/internal/wikipedia"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	// Same as go-mediawiki's progressPrintRate.
	progressPrintRate   = 30 * time.Second
	scrollingMultiplier = 10
)

type PrepareCommand struct {
	SkippedWikidataEntities      string `help:"Load IDs of skipped Wikidata entities."             placeholder:"PATH" type:"path"`
	SkippedWikimediaCommonsFiles string `help:"Load filenames of skipped Wikimedia Commons files." placeholder:"PATH" type:"path"`
}

func (c *PrepareCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedWikidataEntities, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	errE = populateSkippedMap(c.SkippedWikimediaCommonsFiles, &skippedWikimediaCommonsFiles, &skippedWikimediaCommonsFilesCount)
	if errE != nil {
		return errE
	}

	ctx, stop, _, store, esClient, esProcessor, cache, errE := initializeElasticSearch(globals)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

	errE = c.saveCoreProperties(ctx, globals, store, esClient, esProcessor)
	if errE != nil {
		return errE
	}

	return c.updateEmbeddedDocuments(ctx, globals, store, esClient, cache)
}

func (c *PrepareCommand) saveCoreProperties(
	ctx context.Context, globals *Globals,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, esProcessor *elastic.BulkProcessor,
) errors.E {
	return peerdb.SaveCoreProperties(ctx, globals.Logger, store, esClient, esProcessor, globals.Elastic.Index)
}

func (c *PrepareCommand) updateEmbeddedDocuments(
	ctx context.Context, globals *Globals,
	s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, cache *es.Cache,
) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	total, err := esClient.Count(globals.Elastic.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	count := x.Counter(0)
	progress := es.Progress(globals.Logger, nil, cache, nil, "")
	ticker := x.NewTicker(ctx, &count, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	documents := make(chan identifier.Identifier, documentProcessingThreads)
	g.Go(func() error {
		defer close(documents)

		var after *identifier.Identifier
		for {
			docs, errE := s.List(ctx, after)
			if errE != nil {
				return errE
			}

			for _, d := range docs {
				select {
				case documents <- d:
				case <-ctx.Done():
					return errors.WithStack(ctx.Err())
				}
				after = &d
			}
		}
	})

	for range documentProcessingThreads {
		g.Go(func() error {
			for {
				select {
				case d, ok := <-documents:
					if !ok {
						return nil
					}
					err := c.updateEmbeddedDocumentsOne(ctx, globals.Elastic.Index, globals.Logger, s, esClient, cache, d)
					if err != nil {
						return err
					}
					count.Increment()
				case <-ctx.Done():
					return errors.WithStack(ctx.Err())
				}
			}
		})
	}

	return errors.WithStack(g.Wait())
}

func (c *PrepareCommand) updateEmbeddedDocumentsOne(
	ctx context.Context, index string, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, cache *es.Cache, id identifier.Identifier,
) errors.E { //nolint:unparam
	data, _, version, errE := store.GetLatest(ctx, id)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = id.String()
		logger.Error().Err(errE).Send()
		return nil
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(data, &doc)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = id.String()
		logger.Error().Err(errE).Send()
		return nil
	}

	changed, errE := wikipedia.UpdateEmbeddedDocuments(
		ctx, logger, store, index, esClient, cache,
		&skippedWikidataEntities, &skippedWikimediaCommonsFiles,
		&doc,
	)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = id.String()
		logger.Error().Err(errE).Msg("updating embedded documents failed")
		return nil
	}

	if changed {
		logger.Debug().Str("doc", id.String()).Msg("updating document")
		errE = peerdb.UpdateDocument(ctx, store, &doc, version)
		if errE != nil {
			details := errors.Details(errE)
			details["doc"] = id.String()
			logger.Error().Err(errE).Msg("updating document failed")
			return nil
		}
	}

	return nil
}
