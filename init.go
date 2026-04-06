package peerdb

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	internalBase "gitlab.com/peerdb/peerdb/internal/base"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// init initializes the store, coordinator, storage, and bridge for a specific site.
//
// It can be called multiple times. In that case it will not initialize again if
// the site has already been initialized.
func (s *Site) init(ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elasticsearch.TypedClient, shards int) errors.E {
	if s.initialized {
		return nil //nolint:nilnil
	}
	s.initialized = true

	logger = logger.With().Str("schema", s.Schema).Str("index", s.Index).Logger()

	ctx = WithFallbackDBContext(ctx, s.Schema, "init")
	ctx = logger.WithContext(ctx)

	b, riverClient, errE := internalBase.InitComponents(ctx, logger, dbpool, esClient, s.Schema, s.Index, shards)
	if errE != nil {
		return errE
	}

	s.Base = b
	s.DBPool = dbpool
	s.ESClient = esClient
	s.RiverClient = riverClient

	b.LanguagePriority = s.LanguagePriority

	errE = s.initDebugRiverHandler(ctx, logger)
	if errE != nil {
		return errE
	}

	return nil
}

// Init initializes PeerDB for all sites defined in globals.
//
// It establishes connections to PostgreSQL database and ElasticSearch.
// It configures PostgreSQL schemas and ElasticSearch indices.
//
// It can be called multiple times. In that case it will initialize only
// sites which have not been initialized yet.
//
// You have to run site.Start for each site after this call to start the
// base for each site.
func Init(ctx context.Context, globals *Globals) (func(), errors.E) {
	var dbpool *pgxpool.Pool
	var esClient *elasticsearch.TypedClient

	// First we check if any site have them initialized already.
	for _, site := range globals.Sites {
		if dbpool == nil && site.DBPool != nil {
			dbpool = site.DBPool
		}

		if esClient == nil && site.ESClient != nil {
			esClient = site.ESClient
		}

		if dbpool != nil && esClient != nil {
			break
		}
	}

	onShutdown := []func(){}
	onShutdownF := func() {
		for _, f := range onShutdown {
			if f == nil {
				continue
			}
			f()
		}
	}

	// Initialize for the first time.
	if dbpool == nil {
		var errE errors.E
		var dbpoolCleanup func()
		// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
		// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
		dbpool, dbpoolCleanup, errE = internalStore.InitPostgres(
			context.WithoutCancel(ctx),
			string(globals.Postgres.URL),
			globals.Logger,
			getRequestWithFallback(),
		)
		if errE != nil {
			return nil, errE
		}
		// We want dbpoolCleanup to be last.
		onShutdown = append(onShutdown, dbpoolCleanup)
	}

	// Initialize for the first time.
	if esClient == nil {
		var errE errors.E
		esClient, errE = internalSearch.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
		if errE != nil {
			return onShutdownF, errE
		}
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]

		errE := site.init(ctx, globals.Logger, dbpool, esClient, globals.Elastic.Shards)
		if errE != nil {
			return onShutdownF, errE
		}
	}

	return onShutdownF, nil
}
