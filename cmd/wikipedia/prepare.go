package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	progressPrintRate   = 30 * time.Second
	scrollingMultiplier = 10
)

type PrepareCommand struct {
	SkippedCommonsFiles     string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikimedia Commons files."`
	SkippedWikipediaFiles   string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikipedia files."`
	SkippedWikidataEntities string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikidata entities."`
}

func (c *PrepareCommand) Run(globals *Globals) errors.E {
	errE := populateSkippedMap(c.SkippedCommonsFiles, &skippedCommonsFiles, &skippedCommonsFilesCount)
	if errE != nil {
		return errE
	}

	errE = populateSkippedMap(c.SkippedWikipediaFiles, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}

	errE = populateSkippedMap(c.SkippedWikidataEntities, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	ctx, cancel, _, esClient, processor, cache, errE := initializeElasticSearch(globals)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = c.saveStandardProperties(ctx, globals, esClient, processor)
	if errE != nil {
		return errE
	}

	return c.updateEmbeddedDocuments(ctx, globals, esClient, processor, cache)
}

func (c *PrepareCommand) saveStandardProperties(ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor) errors.E {
	for _, property := range search.StandardProperties {
		property := property
		saveDocument(globals, processor, &property)
	}

	// Make sure all just added documents are available for search.
	err := processor.Flush()
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = esClient.Refresh("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *PrepareCommand) updateEmbeddedDocuments(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *wikipedia.Cache,
) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	var count x.Counter

	total, err := esClient.Count("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	ticker := x.NewTicker(ctx, &count, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			fmt.Fprintf(
				os.Stderr,
				"Progress: %0.2f%%, ETA: %s, cache miss: %d, docs: %d, indexed: %d, failed: %d\n",
				p.Percent(), p.Remaining().Truncate(time.Second), cache.MissCount(), count.Count(), stats.Succeeded, stats.Failed,
			)
		}
	}()

	hits := make(chan *elastic.SearchHit, documentProcessingThreads)
	g.Go(func() error {
		defer close(hits)

		scroll := esClient.Scroll("docs").Size(documentProcessingThreads * scrollingMultiplier).SearchSource(elastic.NewSearchSource().SeqNoAndPrimaryTerm(true))
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
					err := c.processDocument(ctx, esClient, processor, cache, hit)
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

func (c *PrepareCommand) processDocument(
	ctx context.Context, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *wikipedia.Cache, hit *elastic.SearchHit,
) errors.E {
	var document search.Document
	err := x.UnmarshalWithoutUnknownFields(hit.Source, &document)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON decoding document %s failed: %s\n", hit.Id, err.Error())
		return nil
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = search.Identifier(hit.Id)

	changed, errE := wikipedia.UpdateEmbeddedDocuments(ctx, esClient, cache, &document)
	if errE != nil {
		fmt.Fprintf(os.Stderr, "updating document %s failed: %s\n", hit.Id, err.Error())
		return nil //nolint:nilerr
	}

	if changed {
		req := elastic.NewBulkIndexRequest().Index("docs").Id(hit.Id).IfSeqNo(*hit.SeqNo).IfPrimaryTerm(*hit.PrimaryTerm).Doc(&document)
		processor.Add(req)
	}

	return nil
}
