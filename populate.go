package peerdb

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/indexer"
	"gitlab.com/peerdb/peerdb/transform"
)

func (c *PopulateCommand) populateSite(ctx context.Context, logger zerolog.Logger, site Site) (func(), errors.E) {
	logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("populating")

	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, site.Schema, "populate")

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
		return nil, nil
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

	onShutdown, errE := site.PopulateAndStart(ctx, transformed, func(doc *document.D) {
		count.Increment()
		logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
	}, nil, count, size)
	if errE != nil {
		return onShutdown, errE
	}

	logger.Info().
		Str("index", site.Index).Str("schema", site.Schema).
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Msg("indexing done")

	return onShutdown, nil
}

// Run executes the populate command to populate database with documents.
func (c *PopulateCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	InitSites(globals)

	ctx, cancel := context.WithCancel(ctx)

	if !c.DryRun {
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
	} else {
		// It is safe to call cancel multiple times. We want it to be
		// called before any onShutdown waits.
		defer cancel()
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
		onS, errE := c.populateSite(ctx, globals.Logger, site)
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return errE
		}
	}

	globals.Logger.Info().Msg("populate done")

	return nil
}
