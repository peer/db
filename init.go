package peerdb

import (
	"context"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// WithFallbackDBContext returns context with fallback context values which are used
// to set application name and schema on PostgreSQL connections when it is not part
// of the request.
func WithFallbackDBContext(ctx context.Context, name, schema string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey, name)
	ctx = context.WithValue(ctx, schemaContextKey, schema)
	return ctx
}

// Init initializes PeerDB for all sites defined in globals.
//
// It establishes connections to PostgreSQL database and ElasticSearch.
// It configures PostgreSQL schemas and ElasticSearch indices.
//
// It can be called multiple times. In that case it will initialize only
// sites which have not been initialized yet.
func Init(ctx context.Context, globals *Globals) errors.E {
	var dbpool *pgxpool.Pool
	var esClient *elastic.Client

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

	// Initialize for the first time.
	if dbpool == nil {
		var errE errors.E
		dbpool, errE = internal.InitPostgres(ctx, string(globals.Postgres.URL), globals.Logger, getRequestWithFallback(globals.Logger))
		if errE != nil {
			return errE
		}
	}

	// Initialize for the first time.
	if esClient == nil {
		var errE errors.E
		esClient, errE = es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
		if errE != nil {
			return errE
		}
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]

		siteCtx := WithFallbackDBContext(ctx, "init", site.Schema)

		if site.Store == nil || site.Coordinator == nil || site.Storage == nil || site.ESProcessor == nil {
			store, coordinator, storage, esProcessor, errE := es.InitForSite(siteCtx, globals.Logger, dbpool, esClient, site.Schema, site.Index)
			if errE != nil {
				return errE
			}

			site.Store = store
			site.Coordinator = coordinator
			site.Storage = storage
			site.ESProcessor = esProcessor
		}

		if site.ESClient == nil {
			site.ESClient = esClient
		}

		if site.DBPool == nil {
			site.DBPool = dbpool
		}
	}

	return nil
}
