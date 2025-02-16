package main

import (
	"time"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
)

//nolint:gochecknoglobals
var NameSpaceProducts = uuid.MustParse("55945768-34e9-4584-9310-cf78602a4aa7")

func index(config *Config) errors.E {
	mainCtx, stop, httpClient, store, esClient, esProcessor, errE := es.Standalone(
		config.Logger, string(config.Postgres.URL), config.Elastic.URL, config.Postgres.Schema, config.Elastic.Index, config.Elastic.SizeField,
	)
	if errE != nil {
		return errE
	}
	defer stop()

	g, ctx := errgroup.WithContext(mainCtx)

	indexingCount := x.NewCounter(0)
	indexingSize := x.NewCounter(0)
	progress := es.Progress(config.Logger, esProcessor, nil, nil, "indexing")
	ticker := x.NewTicker(ctx, indexingCount, indexingSize, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	g.Go(func() error {
		return peerdb.SaveCoreProperties(ctx, config.Logger, store, esClient, esProcessor, config.Elastic.Index, indexingCount, indexingSize)
	})

	g.Go(func() error {
		return config.FoodDataCentral.Run(ctx, config, httpClient, store, indexingCount, indexingSize)
	})

	errE = errors.WithStack(g.Wait())
	if errE != nil {
		return errE
	}

	// We wait for everything to be indexed into ElasticSearch.
	// TODO: Improve this to not have a busy wait.
	for {
		err := esProcessor.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		stats := esProcessor.Stats()
		c := indexingCount.Count()
		if c <= stats.Indexed {
			break
		}
		time.Sleep(time.Second)
	}

	_, err := esClient.Refresh(config.Elastic.Index).Do(mainCtx)
	if err != nil {
		return errors.WithStack(err)
	}

	stats := esProcessor.Stats()
	config.Logger.Info().
		Int64("count", indexingCount.Count()).
		Int64("total", indexingSize.Count()).
		Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).
		Msg("indexing done")

	return nil
}
