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
	"gitlab.com/tozd/identifier"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/wikipedia"
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
	return peerdb.SaveCoreProperties(ctx, globals.Logger, esClient, processor, globals.Index)
}

func (c *PrepareCommand) updateEmbeddedDocuments(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *es.Cache,
) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	total, err := esClient.Count(globals.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	count := x.Counter(0)
	progress := es.Progress(globals.Logger, processor, cache, nil, "")
	ticker := x.NewTicker(ctx, &count, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
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
					err := c.updateEmbeddedDocumentsOne(ctx, globals.Index, globals.Logger, esClient, processor, cache, hit)
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
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *es.Cache, hit *elastic.SearchHit,
) errors.E {
	var doc peerdb.Document
	errE := x.UnmarshalWithoutUnknownFields(hit.Source, &doc)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = hit.Id
		log.Error().Err(errE).Fields(details).Send()
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	doc.ID, errE = identifier.FromString(hit.Id)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = doc.ID.String()
		details["id"] = hit.Id
		log.Error().Err(errE).Fields(details).Msg("invalid hit ID")
		return nil
	}

	changed, errE := wikipedia.UpdateEmbeddedDocuments(
		ctx, index, log, esClient, cache,
		&skippedWikidataEntities, &skippedWikimediaCommonsFiles,
		&doc,
	)
	if errE != nil {
		details := errors.AllDetails(errE)
		details["doc"] = doc.ID.String()
		log.Error().Err(errE).Fields(details).Msg("updating embedded documents failed")
		return nil
	}

	if changed {
		log.Debug().Str("doc", doc.ID.String()).Msg("updating document")
		peerdb.UpdateDocument(processor, index, *hit.SeqNo, *hit.PrimaryTerm, &doc)
	}

	return nil
}
