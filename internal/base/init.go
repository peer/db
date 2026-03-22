// Package base provides reusable helpers for the base package.
package base

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/base"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// InitAndStartComponents initializes and starts Base components.
func InitAndStartComponents(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client,
	schema, index string, languagePriority map[string][]string,
) (*base.B, *river.Client[pgx.Tx], func(), errors.E) {
	errE := internalSearch.EnsureIndex(ctx, esClient, index)
	if errE != nil {
		return nil, nil, nil, errE
	}

	errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	if errE != nil {
		return nil, nil, nil, errE
	}

	listener := internalStore.NewListener(dbpool)

	riverClient, workers, errE := internalStore.NewRiver(ctx, logger, dbpool, schema)
	if errE != nil {
		return nil, nil, nil, errE
	}

	b := &base.B{
		Schema:           schema,
		Index:            index,
		LanguagePriority: languagePriority,
	}
	errE = b.Init(ctx, dbpool, listener, esClient, riverClient, workers)
	if errE != nil {
		return nil, nil, nil, errE
	}

	// Now that everything is initialized, we can start the river client.
	// It will be stopped when ctx is cancelled.
	err := riverClient.Start(ctx)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	onShutdown := func() {
		// Wait for the client to stop.
		<-riverClient.Stopped()
	}

	// After that, we can start the listener.
	return b, riverClient, onShutdown, listener.Start(ctx)
}
