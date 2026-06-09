// Package base provides reusable helpers for the base package.
package base

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	z "gitlab.com/tozd/go/zerolog"

	"gitlab.com/peerdb/peerdb/base"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// InitComponents initializes Base components.
func InitComponents(
	ctx context.Context, logger zerolog.Logger, withContext z.WithContextFunc, dbpool *pgxpool.Pool,
	esClient *elasticsearch.TypedClient, schema, index string, shards int, languagePriority map[string][]string, levels []string,
) (*base.B, *river.Client[pgx.Tx], errors.E) {
	for _, level := range levels {
		errE := internalSearch.EnsureIndex(ctx, esClient, internalSearch.LevelIndex(index, level), shards, languagePriority)
		if errE != nil {
			return nil, nil, errE
		}
	}

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	if errE != nil {
		return nil, nil, errE
	}

	listener := internalStore.NewListener(dbpool)

	riverClient, workers, errE := internalStore.NewRiver(ctx, logger, withContext, dbpool, schema)
	if errE != nil {
		return nil, nil, errE
	}

	b := &base.B{
		Schema:                  schema,
		Index:                   index,
		Levels:                  levels,
		LanguagePriority:        nil,
		IndexAncestorProperties: false,
		IndexingHooks:           nil,
		DocumentPreHooks:        nil,
		DocumentPostHooks:       nil,
		FilePreHooks:            nil,
		FilePostHooks:           nil,
		SearchQueryHook:         nil,
		RegisterWorkers:         nil,
	}
	errE = b.Init(ctx, dbpool, listener, esClient, riverClient, workers)
	if errE != nil {
		return nil, nil, errE
	}

	return b, riverClient, errE
}
