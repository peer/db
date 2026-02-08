package peerdb

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

// SaveCoreProperties saves the core property documents to the store and indices them in ElasticSearch.
func SaveCoreProperties(
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, esProcessor *elastic.BulkProcessor, index string, indexingCount, indexingSize *x.Counter,
) errors.E {
	if indexingSize != nil {
		indexingSize.Add(int64(len(document.CoreProperties)))
	}

	for _, property := range document.CoreProperties {
		if ctx.Err() != nil {
			break
		}

		logger.Debug().Str("doc", property.ID.String()).Str("mnemonic", string(property.Mnemonic)).Msg("saving document")
		errE := InsertOrReplaceDocument(ctx, store, &property)
		if errE != nil {
			return errE
		}

		if indexingCount != nil {
			indexingCount.Increment()
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

func (c *PopulateCommand) populateSite(ctx context.Context, logger zerolog.Logger, site Site) errors.E {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, "populate", site.Schema)

	return SaveCoreProperties(ctx, logger, site.Store, site.ESClient, site.ESProcessor, site.Index, nil, nil)
}

// Run executes the populate command to populate database with core documents.
func (c *PopulateCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(globals.Sites) == 0 {
		globals.Sites = []Site{{
			Site: waf.Site{
				Domain:   "",
				CertFile: "",
				KeyFile:  "",
			},
			Build:           nil,
			Index:           globals.Elastic.Index,
			Schema:          globals.Postgres.Schema,
			Title:           "",
			Store:           nil,
			Coordinator:     nil,
			Storage:         nil,
			ESProcessor:     nil,
			ESClient:        nil,
			DBPool:          nil,
			propertiesTotal: 0,
		}}
	}

	// We set build information on sites.
	if cli.Version != "" || cli.BuildTimestamp != "" || cli.Revision != "" {
		for i := range globals.Sites {
			site := &globals.Sites[i]
			site.Build = &Build{
				Version:        cli.Version,
				BuildTimestamp: cli.BuildTimestamp,
				Revision:       cli.Revision,
			}
		}
	}

	errE := Init(ctx, globals)
	if errE != nil {
		return errE
	}

	for _, site := range globals.Sites {
		errE := c.populateSite(ctx, globals.Logger, site)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("populate done")

	return nil
}
