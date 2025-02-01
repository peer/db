package main

import (
	"time"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
)

const (
	progressPrintRate = 30 * time.Second
)

//nolint:gochecknoglobals
var NameSpaceProducts = uuid.MustParse("55945768-34e9-4584-9310-cf78602a4aa7")

func index(config *Config) errors.E {
	ctx, stop, httpClient, store, esClient, esProcessor, errE := es.Standalone(
		config.Logger, string(config.Postgres.URL), config.Elastic.URL, config.Postgres.Schema, config.Elastic.Index, config.Elastic.SizeField,
	)
	if errE != nil {
		return errE
	}
	defer stop()

	progress := es.Progress(config.Logger, esProcessor, nil, nil, "indexing")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return peerdb.SaveCoreProperties(ctx, config.Logger, store, esClient, esProcessor, config.Elastic.Index)
	})

	g.Go(func() error {
		return config.FoodDataCentral.Run(ctx, config, httpClient, store, progress)
	})

	return errors.WithStack(g.Wait())
}
