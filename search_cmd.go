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

func (c *SearchWaitCommand) waitSite(ctx context.Context, logger zerolog.Logger, site Site) errors.E {
	logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("waiting for indexing")

	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, site.Schema, "search-wait")

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

// Run executes the search wait command which initializes the base,
// waits until all pending indexing is complete, and then exits.
func (c *SearchWaitCommand) Run(globals *Globals) errors.E {
	// We stop gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

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
		errE := c.waitSite(ctx, globals.Logger, site)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("search wait done")

	return nil
}
