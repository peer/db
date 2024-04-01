package peerdb

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

func (c *PopulateCommand) runIndex(ctx context.Context, globals *Globals, dbpool *pgxpool.Pool, esClient *elastic.Client, schema, index string, sizeField bool) errors.E {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = context.WithValue(ctx, requestIDContextKey, "populate")
	ctx = context.WithValue(ctx, schemaContextKey, schema)

	store, esProcessor, errE := initForSite(ctx, globals, dbpool, esClient, schema, index, sizeField)
	if errE != nil {
		return errE
	}

	return SaveCoreProperties(ctx, globals.Logger, store, esClient, esProcessor, index)
}

func (c *PopulateCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbpool, errE := internal.InitPostgres(ctx, string(globals.Database), globals.Logger, getRequestWithFallback(globals.Logger))
	if errE != nil {
		return errE
	}

	esClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic)
	if errE != nil {
		return errE
	}

	if len(globals.Sites) > 0 {
		for _, site := range globals.Sites {
			err := c.runIndex(ctx, globals, dbpool, esClient, site.Schema, site.Index, site.SizeField)
			if err != nil {
				return err
			}
		}
	} else {
		err := c.runIndex(ctx, globals, dbpool, esClient, globals.Schema, globals.Index, globals.SizeField)
		if err != nil {
			return err
		}
	}

	globals.Logger.Info().Msg("Done.")

	return nil
}
