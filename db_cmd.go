package peerdb

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// InitSites sets up default site configuration and build information if needed. It also applies consumer
// site defaults (through Customize.SiteDefaults) to every site: sites from the configuration already received
// them during configuration validation, but the default site synthesized here did not, and callers which populate
// Globals programmatically (without command-line parsing) get them here as well. SiteDefaults is idempotent,
// so the repeated application is safe.
func InitSites(globals *Globals) errors.E {
	if len(globals.Sites) == 0 {
		globals.Sites = []internalSite.Site{{
			Site: waf.Site{
				Domain:   "",
				CertFile: "",
				KeyFile:  "",
			},
			Build:                nil,
			IndexPrefix:          globals.Elastic.IndexPrefix,
			Schema:               globals.Postgres.Schema,
			Title:                "",
			Logo:                 "",
			LogoCompact:          "",
			LanguagePriority:     nil,
			DefaultLanguage:      "",
			LanguageCodes:        nil,
			Features:             internalSite.SiteFeatures{},
			Roles:                nil,
			Visibility:           nil,
			Auth:                 internalSite.SiteAuthConfig{},
			MetadataHeaderPrefix: "",
			Authenticator:        nil,
			Base:                 nil,
			DBPool:               nil,
			ESClient:             nil,
			RiverClient:          nil,
			DebugRiverHandler:    nil,
		}}
	}

	if globals.Customize.SiteDefaults != nil {
		for i := range globals.Sites {
			errE := globals.Customize.SiteDefaults(&globals.Sites[i])
			if errE != nil {
				return errE
			}
		}
	}

	// We set build information on sites.
	if cli.Version != "" || cli.BuildTimestamp != "" || cli.Revision != "" {
		for i := range globals.Sites {
			site := &globals.Sites[i]
			site.Build = &internalSite.Build{
				Version:        cli.Version,
				BuildTimestamp: cli.BuildTimestamp,
				Revision:       cli.Revision,
			}
		}
	}

	return nil
}

// startAndWaitSite starts the base for a site, runs optional beforeWait,
// then waits for indexing to catch up, and refreshes the ElasticSearch index.
func startAndWaitSite(ctx context.Context, logger zerolog.Logger, site internalSite.Site, beforeWait func(ctx context.Context) errors.E) (func(), errors.E) {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = internalStore.WithFallbackDBContext(ctx, site.Schema, "db")

	documents, errE := site.FetchDocuments(ctx, internalCore.PropertyClassID)
	if errE != nil {
		return nil, errE
	}
	languages, errE := site.FetchDocuments(ctx, internalCore.LanguageClassID)
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

	for _, index := range site.LevelIndexes() {
		_, err := site.ESClient.Indices.Refresh().Index(index).Do(ctx)
		if err != nil {
			errE := internalSearch.WithESError(err)
			errors.Details(errE)["index"] = index
			return onShutdown, errE
		}
	}

	logger.Info().
		Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).
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

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

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
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Msg("waiting for indexing")

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

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

	// When recreating the index, delete it before Init so that the base's EnsureIndex (run during
	// startup) recreates it from the current mapping. The documents are then replayed from PostgreSQL
	// into the fresh index below, so a mapping change is applied without losing source data.
	if c.RecreateIndex {
		esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
		if errE != nil {
			return errE
		}
		for _, site := range globals.Sites {
			for _, index := range site.LevelIndexes() {
				_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(ctx)
				if err != nil {
					errE := internalSearch.WithESError(err)
					errors.Details(errE)["index"] = index
					return errE
				}
				globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("index", index).Msg("index deleted for recreation")
			}
		}
	}

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

	// With --recreate-index we also clear the accumulated inverse-relation metadata from the store (for every
	// document, including deleted ones) before replaying, so the reindex rebuilds it from a clean slate rather
	// than diffing on top of stale or wrongly-leveled entries. This runs after Init (the store is available)
	// but before any base is started, so it does not race the bridge.
	if c.RecreateIndex {
		for _, site := range globals.Sites {
			cleared, errE := site.Base.ClearInverseRelations(ctx)
			if errE != nil {
				return errE
			}
			globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Int("cleared", cleared).Msg("cleared inverse relations")
		}
	}

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Msg("reindexing")

		onS, errE := startAndWaitSite(ctx, globals.Logger, site, func(ctx context.Context) errors.E {
			return site.Base.ResetBridgeProgress(ctx)
		})
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}

		// Replaying the whole commit log re-indexes each document once per commit that changes it.
		// In particular a reference target is rewritten every time a new referrer appears, so hub
		// documents accumulate many superseded (deleted) Lucene versions. Those deletes linger per
		// shard until merged and bloat the index (they also skew per-shard term statistics).
		// Now that the site is caught up, expunge them. We use only_expunge_deletes rather than
		// max_num_segments because the index keeps receiving live writes after a reindex, and
		// full-merging an index that is still written to is discouraged.
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Msg("expunging deletes")

		for _, index := range site.LevelIndexes() {
			_, err := site.ESClient.Indices.Forcemerge().Index(index).OnlyExpungeDeletes(true).Do(ctx)
			if err != nil {
				errE := internalSearch.WithESError(err)
				errors.Details(errE)["index"] = index
				return errE
			}
			globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("index", index).Msg("deletes expunged")
		}
	}

	globals.Logger.Info().Msg("db reindex done")

	return nil
}

// vacuumSchema runs VACUUM on every table in the given PostgreSQL schema to reclaim space held by
// dead tuples and to refresh planner statistics. VACUUM cannot run inside a transaction, so we
// acquire a single connection and run each statement directly in autocommit mode. We enumerate the
// schema's tables instead of running a bare VACUUM because the latter vacuums every schema in the
// database, which would redundantly revisit other sites sharing the same database.
func vacuumSchema(ctx context.Context, dbpool *pgxpool.Pool, schema string) errors.E {
	conn, err := dbpool.Acquire(ctx)
	if err != nil {
		return internalStore.WithPgxError(err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY tablename`, schema)
	if err != nil {
		return internalStore.WithPgxError(err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return internalStore.WithPgxError(err)
	}

	for _, table := range tables {
		_, err := conn.Exec(ctx, fmt.Sprintf(`VACUUM (ANALYZE) "%s"."%s"`, schema, table))
		if err != nil {
			return internalStore.WithPgxError(err)
		}
	}

	return nil
}

// Run executes the db vacuum command which reclaims dead tuples in PostgreSQL
// and expunges deleted documents from ElasticSearch for all configured sites.
func (c *DBVacuumCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

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
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Msg("vacuuming")

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := internalStore.WithFallbackDBContext(ctx, site.Schema, "vacuum")

		errE = vacuumSchema(siteCtx, dbpool, site.Schema)
		if errE != nil {
			return errE
		}

		globals.Logger.Info().Str("schema", site.Schema).Msg("schema vacuumed")

		// We expunge deleted Lucene documents per shard so the superseded versions that versioned writes
		// and reindexing accumulate do not bloat the index or skew per-shard term statistics. We use
		// only_expunge_deletes rather than max_num_segments because the index keeps receiving live writes,
		// and full-merging an index that is still written to is discouraged.
		for _, index := range site.LevelIndexes() {
			_, err := esClient.Indices.Forcemerge().Index(index).OnlyExpungeDeletes(true).Do(siteCtx)
			if err != nil {
				errE := internalSearch.WithESError(err)
				errors.Details(errE)["index"] = index
				return errE
			}
			globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("index", index).Msg("deletes expunged")
		}
	}

	globals.Logger.Info().Msg("db vacuum done")

	return nil
}

// Run executes the db export command which exports documents to CSV or JSON.
func (c *DBExportCommand) Run(globals *Globals) (returnErr errors.E) { //nolint:nonamedreturns
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

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
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Msg("exporting")

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := internalStore.WithFallbackDBContext(ctx, site.Schema, "export")

		onS, errE := startAndWaitSite(siteCtx, globals.Logger, site, nil)
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}

		getDoc := func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
			doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id)
			return doc, errE
		}

		errE = internalExport.Export(siteCtx, w, site.ESClient, site.TopIndex(), getDoc, internalExport.Config{
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

// Run executes the db diagram command which writes a Mermaid ER diagram describing classes and fields.
func (c *DBDiagramCommand) Run(globals *Globals) (returnErr errors.E) { //nolint:nonamedreturns
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

	return internalExport.Diagram(globals.Logger, w, c.SkipCore)
}

// Run executes the db wipe command which drops PostgreSQL schemas
// and deletes ElasticSearch indices for all configured sites.
func (c *DBWipeCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

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
		globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Msg("wiping")

		// We set fallback context values which are used to set application name on PostgreSQL connections.
		siteCtx := internalStore.WithFallbackDBContext(ctx, site.Schema, "wipe")

		errE = internalStore.RetryTransaction(siteCtx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, site.Schema))
			return internalStore.WithPgxError(err)
		})
		if errE != nil {
			return errE
		}

		globals.Logger.Info().Str("schema", site.Schema).Msg("schema dropped")

		for _, index := range site.LevelIndexes() {
			_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(siteCtx)
			if err != nil {
				errE := internalSearch.WithESError(err)
				errors.Details(errE)["index"] = index
				return errE
			}
			globals.Logger.Info().Str("indexPrefix", site.IndexPrefix).Str("index", index).Msg("index deleted")
		}
	}

	globals.Logger.Info().Msg("db wipe done")

	return nil
}
