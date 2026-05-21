package peerdb

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/indexer"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalExport "gitlab.com/peerdb/peerdb/internal/export"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// InitSites sets up default site configuration and build information if needed.
func InitSites(globals *Globals) {
	if len(globals.Sites) == 0 {
		globals.Sites = []Site{{
			Site: waf.Site{
				Domain:   "",
				CertFile: "",
				KeyFile:  "",
			},
			Build:                nil,
			Index:                globals.Elastic.Index,
			Schema:               globals.Postgres.Schema,
			Title:                "",
			Logo:                 "",
			LanguagePriority:     nil,
			DefaultLanguage:      "",
			LanguageCodes:        nil,
			Features:             SiteFeatures{},
			Roles:                nil,
			Auth:                 SiteAuthConfig{},
			AuthEnabled:          false,
			MetadataHeaderPrefix: "",
			verifier:             nil,
			Base:                 nil,
			DBPool:               nil,
			ESClient:             nil,
			RiverClient:          nil,
			flowStore:            nil,
			debugRiverHandler:    nil,
			initialized:          false,
			propertiesTotal:      0,
			unitsTotal:           0,
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
}

// startAndWaitSite starts the base for a site, runs optional beforeWait,
// then waits for indexing to catch up, and refreshes the ElasticSearch index.
func startAndWaitSite(ctx context.Context, logger zerolog.Logger, site Site, beforeWait func(ctx context.Context) errors.E) (func(), errors.E) {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, site.Schema, "db")

	documents, errE := site.fetchDocuments(ctx, internalCore.PropertyClassID)
	if errE != nil {
		return nil, errE
	}
	languages, errE := site.fetchDocuments(ctx, internalCore.LanguageClassID)
	if errE != nil {
		return nil, errE
	}

	documents = append(documents, languages...)

	onShutdown, errE := site.Start(ctx, documents)
	if errE != nil {
		return onShutdown, errE
	}

	count := x.NewCounter(0)
	size := x.NewCounter(0)
	progress := indexer.Progress(logger, "indexing", nil)
	ticker := x.NewTicker(ctx, count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	if beforeWait != nil {
		errE = beforeWait(ctx)
		if errE != nil {
			return onShutdown, errE
		}
	}

	errE = site.Base.WaitUntilCaughtUp(ctx, count, size)
	if errE != nil {
		return onShutdown, errE
	}

	_, err := site.ESClient.Indices.Refresh().Index(site.Index).Do(ctx)
	if err != nil {
		return onShutdown, errors.WithStack(err)
	}

	logger.Info().
		Str("index", site.Index).Str("schema", site.Schema).
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Msg("indexing done")

	return onShutdown, nil
}

// Run executes the db wait command which initializes the base,
// waits until all pending indexing is complete, and then exits.
func (c *DBWaitCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	InitSites(globals)

	ctx, cancel := context.WithCancel(ctx)

	onShutdownInit, errE := Init(ctx, globals)
	if onShutdownInit != nil {
		defer onShutdownInit()
	}
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()
	if errE != nil {
		return errE
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
	defer onShutdownF()
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("waiting for indexing")

		onS, errE := startAndWaitSite(ctx, globals.Logger, site, nil)
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("db wait done")

	return nil
}

// Run executes the db reindex command which resets the bridge progress,
// re-processes all commits from the beginning, and then exits.
func (c *DBReindexCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	InitSites(globals)

	ctx, cancel := context.WithCancel(ctx)

	onShutdownInit, errE := Init(ctx, globals)
	if onShutdownInit != nil {
		defer onShutdownInit()
	}
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()
	if errE != nil {
		return errE
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
	defer onShutdownF()
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("reindexing")

		onS, errE := startAndWaitSite(ctx, globals.Logger, site, func(ctx context.Context) errors.E {
			return site.Base.ResetBridgeProgress(ctx)
		})
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("db reindex done")

	return nil
}

// Run executes the db export command which exports documents to CSV or JSON.
func (c *DBExportCommand) Run(globals *Globals) (returnErr errors.E) { //nolint:nonamedreturns
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	InitSites(globals)

	ctx, cancel := context.WithCancel(ctx)

	onShutdownInit, errE := Init(ctx, globals)
	if onShutdownInit != nil {
		defer onShutdownInit()
	}
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()
	if errE != nil {
		return errE
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
	defer onShutdownF()
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()

	// Determine output writer.
	var w io.Writer
	if c.Output == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(c.Output)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			errE := f.Close()
			if errE != nil && returnErr == nil {
				returnErr = errors.WithStack(errE)
			}
		}()
		w = f
	}

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("exporting")

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := WithFallbackDBContext(ctx, site.Schema, "export")

		onS, errE := startAndWaitSite(siteCtx, globals.Logger, site, nil)
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}

		getDoc := func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
			doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id)
			return doc, errE
		}

		errE = internalExport.Export(siteCtx, w, site.ESClient, site.Index, getDoc, internalExport.Config{
			Format:     c.Format,
			InstanceOf: c.InstanceOf,
			Properties: c.Property,
		})
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("db export done")

	return nil
}

// Run executes the db wipe command which drops PostgreSQL schemas
// and deletes ElasticSearch indices for all configured sites.
func (c *DBWipeCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	InitSites(globals)

	// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
	// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
	dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(
		context.WithoutCancel(ctx),
		string(globals.Postgres.URL),
		globals.Logger,
		getRequestWithFallback(),
	)
	if errE != nil {
		return errE
	}
	defer dbpoolCleanup()

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
	if errE != nil {
		return errE
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("wiping")

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := WithFallbackDBContext(ctx, site.Schema, "wipe")

		errE = internalStore.RetryTransaction(siteCtx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, site.Schema))
			return internalStore.WithPgxError(err)
		})
		if errE != nil {
			return errE
		}

		globals.Logger.Info().Str("schema", site.Schema).Msg("schema dropped")

		_, err := esClient.Indices.Delete(site.Index).IgnoreUnavailable(true).Do(siteCtx)
		if err != nil {
			return errors.WithStack(err)
		}

		globals.Logger.Info().Str("index", site.Index).Msg("index deleted")
	}

	globals.Logger.Info().Msg("db wipe done")

	return nil
}
