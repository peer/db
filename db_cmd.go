package peerdb

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/indexer"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
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
			Build:            nil,
			Index:            globals.Elastic.Index,
			Schema:           globals.Postgres.Schema,
			Title:            "",
			Logo:             "",
			LanguagePriority: nil,
			DefaultLanguage:  "",
			LanguageCodes:    nil,
			Features:         SiteFeatures{},
			Base:             nil,
			DBPool:           nil,
			ESClient:         nil,
			RiverClient:      nil,
			initialized:      false,
			propertiesTotal:  0,
			unitsTotal:       0,
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
func startAndWaitSite(ctx context.Context, logger zerolog.Logger, site Site, beforeWait func(ctx context.Context) errors.E) errors.E {
	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, site.Schema, "db")

	documents, errE := site.fetchDocuments(ctx, internalCore.PropertyClassID)
	if errE != nil {
		return errE
	}
	languages, errE := site.fetchDocuments(ctx, internalCore.LanguageClassID)
	if errE != nil {
		return errE
	}

	documents = append(documents, languages...)

	errE = site.Start(ctx, documents)
	if errE != nil {
		return errE
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
			return errE
		}
	}

	errE = site.Base.WaitUntilCaughtUp(ctx, count, size)
	if errE != nil {
		return errE
	}

	_, err := site.ESClient.Indices.Refresh().Index(site.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	logger.Info().
		Str("index", site.Index).Str("schema", site.Schema).
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Msg("indexing done")

	return nil
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

	onShutdown, errE := Init(ctx, globals)
	if onShutdown != nil {
		defer onShutdown()
	}
	defer cancel()
	if errE != nil {
		return errE
	}

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("waiting for indexing")

		errE := startAndWaitSite(ctx, globals.Logger, site, nil)
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

	onShutdown, errE := Init(ctx, globals)
	if onShutdown != nil {
		defer onShutdown()
	}
	defer cancel()
	if errE != nil {
		return errE
	}

	for _, site := range globals.Sites {
		globals.Logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("reindexing")

		errE = startAndWaitSite(ctx, globals.Logger, site, func(ctx context.Context) errors.E {
			// Reset bridge progress so all commits are re-processed.
			return site.Base.Bridge().ResetSeq(ctx)
		})
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("db reindex done")

	return nil
}
