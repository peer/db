package site

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	z "gitlab.com/tozd/go/zerolog"

	internalBase "gitlab.com/peerdb/peerdb/internal/base"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// Prepare initializes the store, coordinator, storage, and bridge for the site.
//
// It can be called multiple times. In that case it will not initialize again if
// the site has already been initialized.
func (s *Site) Prepare(
	ctx context.Context, logger zerolog.Logger, withContext z.WithContextFunc,
	dbpool *pgxpool.Pool, esClient *elasticsearch.TypedClient, shards int,
) errors.E {
	if s.initialized {
		return nil
	}
	s.initialized = true

	logger = logger.With().Str("schema", s.Schema).Str("index", s.Index).Logger()

	ctx = internalStore.WithFallbackDBContext(ctx, s.Schema, "init")
	ctx = logger.WithContext(ctx)

	b, riverClient, errE := internalBase.InitComponents(ctx, logger, withContext, dbpool, esClient, s.Schema, s.Index, shards, s.LanguagePriority)
	if errE != nil {
		return errE
	}

	s.Base = b
	s.DBPool = dbpool
	s.ESClient = esClient
	s.RiverClient = riverClient

	errE = s.initDebugRiverHandler(ctx, logger)
	if errE != nil {
		return errE
	}

	return nil
}
