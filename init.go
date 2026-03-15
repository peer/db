package peerdb

import (
	"context"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/base"
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

// init initializes the store, coordinator, storage, and bridge for a specific site.
//
// It can be called multiple times. In that case it will not initialize again if
// the site has already been initialized.
func (s *Site) init(ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client) ([]func(), errors.E) {
	if s.initialized {
		return nil, nil
	}
	s.initialized = true

	ctx = WithFallbackDBContext(ctx, "init", s.Schema)
	ctx = logger.With().Str("schema", s.Schema).Str("index", s.Index).Logger().WithContext(ctx)

	errE := es.EnsureIndex(ctx, esClient, s.Index)
	if errE != nil {
		return nil, errE
	}

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, s.Schema)
	})
	if errE != nil {
		return nil, errE
	}

	listener := internal.NewListener(dbpool)

	riverClient, workers, errE := internal.NewRiver(ctx, logger, dbpool, s.Schema)
	if errE != nil {
		return nil, errE
	}

	b := &base.B{
		Schema: s.Schema,
		Index:  s.Index,
	}
	errE = b.Init(ctx, dbpool, listener, esClient, riverClient, workers)
	if errE != nil {
		return nil, errE
	}

	// Now that everything is initialized, we can start the river client.
	// It will be stopped when ctx is cancelled.
	err := riverClient.Start(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	onShutdown := []func(){
		func() {
			// Wait for the client to stop.
			<-riverClient.Stopped()
		},
	}

	// After that, we can start the listener.
	errE = listener.Start(ctx)
	if errE != nil {
		return onShutdown, errE
	}

	// And after the listener we can start the base.
	errE = b.Start(ctx)
	if errE != nil {
		return onShutdown, errE
	}

	s.Base = b
	s.DBPool = dbpool
	s.ESClient = esClient
	s.RiverClient = riverClient

	return onShutdown, nil
}

// Init initializes PeerDB for all sites defined in globals.
//
// It establishes connections to PostgreSQL database and ElasticSearch.
// It configures PostgreSQL schemas and ElasticSearch indices.
//
// It can be called multiple times. In that case it will initialize only
// sites which have not been initialized yet.
func Init(ctx context.Context, globals *Globals) (func(), errors.E) {
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
			return nil, errE
		}
	}

	// Initialize for the first time.
	if esClient == nil {
		var errE errors.E
		esClient, errE = es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
		if errE != nil {
			return nil, errE
		}
	}

	onShutdown := []func(){}
	onShutdownF := func() {
		for _, f := range onShutdown {
			f()
		}
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]

		onS, errE := site.init(ctx, globals.Logger, dbpool, esClient)
		onShutdown = append(onShutdown, onS...)
		if errE != nil {
			return onShutdownF, errE
		}
	}

	return onShutdownF, nil
}
