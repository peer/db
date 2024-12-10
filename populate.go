package peerdb

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

func SaveCoreProperties(
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, esProcessor *elastic.BulkProcessor, index string,
) errors.E {
	for _, property := range document.CoreProperties {
		if ctx.Err() != nil {
			break
		}

		logger.Debug().Str("doc", property.ID.String()).Str("mnemonic", string(property.Mnemonic)).Msg("saving document")
		errE := InsertOrReplaceDocument(ctx, store, &property)
		if errE != nil {
			return errE
		}
	}

	// We sleep to make sure all changesets are bridged.
	time.Sleep(time.Second)

	// Make sure all just added documents are available for search.
	err := esProcessor.Flush()
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = esClient.Refresh(index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *PopulateCommand) runIndex(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client,
	schema, index string, sizeField bool,
) errors.E {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = context.WithValue(ctx, requestIDContextKey, "populate")
	ctx = context.WithValue(ctx, schemaContextKey, schema)

	store, _, _, esProcessor, errE := es.InitForSite(ctx, logger, dbpool, esClient, schema, index, sizeField)
	if errE != nil {
		return errE
	}

	return SaveCoreProperties(ctx, logger, store, esClient, esProcessor, index)
}

func (c *PopulateCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbpool, errE := internal.InitPostgres(ctx, string(globals.Postgres.URL), globals.Logger, getRequestWithFallback(globals.Logger))
	if errE != nil {
		return errE
	}

	esClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
	if errE != nil {
		return errE
	}

	if len(globals.Sites) > 0 {
		for _, site := range globals.Sites {
			err := c.runIndex(ctx, globals.Logger, dbpool, esClient, site.Schema, site.Index, site.SizeField)
			if err != nil {
				return err
			}
		}
	} else {
		err := c.runIndex(ctx, globals.Logger, dbpool, esClient, globals.Postgres.Schema, globals.Elastic.Index, globals.Elastic.SizeField)
		if err != nil {
			return err
		}
	}

	globals.Logger.Info().Msg("Done.")

	return nil
}
