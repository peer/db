package main

import (
	"context"
	"io"
	"runtime"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	// Same as go-mediawiki's progressPrintRate.
	progressPrintRate   = 30 * time.Second
	scrollingMultiplier = 10
)

type PrepareCommand struct {
	SkippedWikidataEntities      string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
	SkippedWikimediaCommonsFiles string `placeholder:"PATH" type:"path" help:"Load filenames of skipped Wikimedia Commons files."`
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

	ctx, cancel, _, esClient, processor, cache, errE := initializeElasticSearch(globals)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = c.saveCoreProperties(ctx, globals, esClient, processor)
	if errE != nil {
		return errE
	}

	return c.updateEmbeddedDocuments(ctx, globals, esClient, processor, cache)
}

func (c *PrepareCommand) saveCoreProperties(ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor) errors.E {
	return search.SaveCoreProperties(ctx, globals.Log, esClient, processor, globals.Index)
}

func (c *PrepareCommand) updateEmbeddedDocuments(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *wikipedia.Cache,
) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	var count x.Counter

	total, err := esClient.Count(globals.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	ticker := x.NewTicker(ctx, &count, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			globals.Log.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("docs", count.Count()).
				Uint64("cacheMiss", cache.MissCount()).Str("eta", p.Remaining().Truncate(time.Second).String()).
				Msgf("progress %0.2f%%", p.Percent())
		}
	}()

	hits := make(chan *elastic.SearchHit, documentProcessingThreads)
	g.Go(func() error {
		defer close(hits)

		scroll := esClient.Scroll(globals.Index).
			Size(documentProcessingThreads*scrollingMultiplier).
			Sort("_doc", true).
			SearchSource(elastic.NewSearchSource().SeqNoAndPrimaryTerm(true))
		for {
			results, err := scroll.Do(ctx)
			if errors.Is(err, io.EOF) {
				return nil
			} else if err != nil {
				return errors.WithStack(err)
			}

			for _, hit := range results.Hits.Hits {
				select {
				case hits <- hit:
				case <-ctx.Done():
					return errors.WithStack(ctx.Err())
				}
			}
		}
	})

	for i := 0; i < documentProcessingThreads; i++ {
		g.Go(func() error {
			for {
				select {
				case hit, ok := <-hits:
					if !ok {
						return nil
					}
					err := c.updateEmbeddedDocumentsOne(ctx, globals.Index, globals.Log, esClient, processor, cache, hit)
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
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *wikipedia.Cache, hit *elastic.SearchHit,
) errors.E {
	var document search.Document
	errE := x.UnmarshalWithoutUnknownFields(hit.Source, &document)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = hit.Id
		log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = search.Identifier(hit.Id)

	changed, errE := wikipedia.UpdateEmbeddedDocuments(
		ctx, index, log, esClient, cache,
		&skippedWikidataEntities, &skippedWikimediaCommonsFiles,
		&document,
	)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = string(document.ID)
		log.Error().Err(errE).Fields(details).Msg("updating embedded documents failed")
		return nil
	}

	if changed {
		log.Debug().Str("doc", string(document.ID)).Msg("updating document")
		search.UpdateDocument(processor, index, *hit.SeqNo, *hit.PrimaryTerm, &document)
	}

	return nil
}
