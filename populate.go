package peerdb

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/indexer"
	"gitlab.com/peerdb/peerdb/transform"
)

func (c *PopulateCommand) populateSite(ctx context.Context, site internalSite.Site) (func(), errors.E) {
	logger := *zerolog.Ctx(ctx)
	logger.Info().Msg("populating")

	documents, transformed, errE := base.GenerateCoreDocuments(ctx, nil)
	if errE != nil {
		return nil, errE
	}

	logger.Info().Int("count", len(documents)).Msg("generated all documents")

	if ctx.Err() != nil {
		return nil, errors.WithStack(ctx.Err())
	}

	if c.SaveDir != "" {
		logger.Info().Str("path", c.SaveDir).Msg("saving structs as files into a directory")

		errE := x.SaveJSONToDir(ctx, c.SaveDir, documents, func(doc any) (string, errors.E) {
			id, errE := transform.ExtractDocumentID(doc)
			if errE != nil {
				return "", errE
			}

			p := slices.Clone(id)
			for i := range len(id) - 1 {
				p = append(p, x.SafeFilename(id[i]))
			}
			p = append(p, x.SafeFilename(id[len(id)-1])+".json")

			return filepath.Join(p...), nil
		})
		if errE != nil {
			return nil, errE
		}

		logger.Info().Int("count", len(documents)).Msg("saved all structs")

		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}
	}

	if c.OutputDir != "" {
		logger.Info().Str("path", c.OutputDir).Msg("saving documents as files into a directory")

		errE := x.SaveJSONToDir(ctx, c.OutputDir, transformed, func(doc *document.D) (string, errors.E) {
			return doc.ID.String(), nil
		})
		if errE != nil {
			return nil, errE
		}

		logger.Info().Int("count", len(transformed)).Msg("saved all documents")

		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}
	}

	if c.DryRun {
		logger.Info().Msg("dry run, not inserting documents into the database")
		// A nil shutdown function is a valid value: the base was not started, so there is nothing to shut down.
		return nil, nil //nolint:nilnil
	}

	count := x.NewCounter(0)
	size := x.NewCounter(int64(len(transformed)))
	progress := indexer.Progress(logger, "indexing", nil)
	ticker := x.NewTicker(ctx, count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	populateShutdown, errE := site.PopulateAndStart(ctx, transformed, func(doc *document.D) {
		count.Increment()
		logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
	}, nil, count, size)
	if errE != nil {
		return populateShutdown, errE
	}

	logger.Info().
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Msg("indexing done")

	return populateShutdown, nil
}

// Run executes the populate command to populate database with documents. Each site is populated through
// PopulateSite when set, otherwise with the generated core documents.
func (c *PopulateCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	errE := InitSites(globals)
	if errE != nil {
		return errE
	}

	ctx, cancel := context.WithCancel(ctx)

	if !c.DryRun {
		onShutdownInit, errE := Init(ctx, globals)
		if onShutdownInit != nil {
			defer onShutdownInit()
		}
		defer cancel()
		if errE != nil {
			return errE
		}
	} else {
		defer cancel()
	}

	populateSite := c.PopulateSite
	if populateSite == nil {
		populateSite = c.populateSite
	}

	for _, site := range globals.Sites {
		errE := populateOneSite(ctx, populateSite, site)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("populate done")

	return nil
}

// populateOneSite prepares the per-site context (the context logger carries the site fields and the fallback
// database context) and calls populateSite with it, cancellable per site. The shutdown function populateSite
// returns is run after the context is cancelled. This order is required: the shutdown waits for the base
// (started inside PopulateAndStart) to stop, and the base stops only when the context is cancelled, so
// running the shutdown with the context still alive would block forever.
func populateOneSite(
	ctx context.Context, populateSite func(ctx context.Context, site Site) (func(), errors.E), site internalSite.Site,
) errors.E {
	ctx = zerolog.Ctx(ctx).With().Str("indexPrefix", site.IndexPrefix).Str("schema", site.Schema).Logger().WithContext(ctx)
	ctx = internalStore.WithFallbackDBContext(ctx, site.Schema, "populate")

	ctx, cancel := context.WithCancel(ctx)
	var onShutdown func()
	defer func() {
		cancel()
		if onShutdown != nil {
			onShutdown()
		}
	}()

	onShutdown, errE := populateSite(ctx, site)
	return errE
}
