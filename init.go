package peerdb

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	// TODO: Determine reasonable size for the buffer.
	bridgeBufferSize = 100
)

// WithFallbackDBContext returns context with fallback context values which are used
// to set application name and schema on PostgreSQL connections when it is not part
// of the request.
func WithFallbackDBContext(ctx context.Context, name, schema string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey, name)
	ctx = context.WithValue(ctx, schemaContextKey, schema)
	return ctx
}

// initForSite initializes the store, coordinator, storage, and bridge for a specific site.
func initForSite(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client, schema, index string,
) (
	*store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	*coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata],
	*storage.Storage,
	*es.Bridge[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	errors.E,
) {
	ctx = WithFallbackDBContext(ctx, "init", schema)
	ctx = logger.With().Str("schema", schema).Str("index", index).Logger().WithContext(ctx)

	// TODO: Add some monitoring of the channel contention.
	channel := make(
		chan store.CommittedChangesets[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
		bridgeBufferSize,
	)

	errE := es.EnsureIndex(ctx, esClient, index)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	listener := internal.NewListener(dbpool)

	s := &store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]{
		Prefix:       "docs",
		Committed:    channel,
		DataType:     "jsonb",
		MetadataType: "jsonb",
		PatchType:    "jsonb",
	}
	errE = s.Init(ctx, dbpool, listener)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	var c *coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata]
	c = &coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata]{
		Prefix:       "docs",
		DataType:     "jsonb",
		MetadataType: "jsonb",
		EndCallback: func(ctx context.Context, session identifier.Identifier, metadata *types.DocumentEndMetadata) (*types.DocumentEndMetadata, errors.E) {
			return es.EndDocumentSession(ctx, s, c, session, metadata)
		},
		Appended: nil,
		Ended:    nil,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = c.Init(ctx, dbpool, nil)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	storage := &storage.Storage{
		Prefix:    "storage",
		Committed: nil,
	}
	// We do not use Committed channel here so we pass nil for listener.
	errE = storage.Init(ctx, dbpool, nil)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	internal.StartListener(ctx, listener)

	b := &es.Bridge[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]{
		Store:     s,
		ESClient:  esClient,
		Index:     index,
		Committed: channel,
		Listener:  listener,
	}
	errE = b.Init(ctx, dbpool)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	b.Start(ctx)

	return s, c, storage, b, nil
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

		if site.Store == nil || site.Coordinator == nil || site.Storage == nil || site.Bridge == nil {
			store, coordinator, storage, bridge, errE := initForSite(ctx, globals.Logger, dbpool, esClient, site.Schema, site.Index)
			if errE != nil {
				return errE
			}

			site.Store = store
			site.Coordinator = coordinator
			site.Storage = storage
			site.Bridge = bridge
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
