package peerdb

import (
	"context"

	"github.com/hashicorp/go-cleanhttp"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// Init initializes PeerDB for all sites defined in globals.
//
// It establishes connections to PostgreSQL database and ElasticSearch.
// It configures PostgreSQL schemas and ElasticSearch indices.
func Init(ctx context.Context, globals *Globals) errors.E {
	dbpool, errE := internal.InitPostgres(ctx, string(globals.Postgres.URL), globals.Logger, getRequestWithFallback(globals.Logger))
	if errE != nil {
		return errE
	}

	esClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
	if errE != nil {
		return errE
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := context.WithValue(ctx, requestIDContextKey, "init")
		siteCtx = context.WithValue(siteCtx, schemaContextKey, site.Schema)

		store, coordinator, storage, esProcessor, errE := es.InitForSite(siteCtx, globals.Logger, dbpool, esClient, site.Schema, site.Index)
		if errE != nil {
			return errE
		}

		site.Store = store
		site.Coordinator = coordinator
		site.Storage = storage
		site.ESProcessor = esProcessor
		site.ESClient = esClient
	}

	return nil
}
