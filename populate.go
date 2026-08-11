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
	"gitlab.com/peerdb/peerdb/internal/xeno"
	"gitlab.com/peerdb/peerdb/transform"
)

// generateTestDataDocuments generates the core documents together with the test data schema (its
// properties and classes), adds the loaded test data documents to them, and transforms all of them
// into documents to index. Properties are generated before classes because the class field schemas
// are built from property mnemonics.
func generateTestDataDocuments(ctx context.Context, logger zerolog.Logger, testData []any) ([]any, []*document.D, errors.E) {
	return base.GenerateCoreDocuments(ctx, func(ctx context.Context, coreDocuments []any) ([]any, errors.E) {
		documents := coreDocuments

		docs, errE := xeno.Properties()
		if errE != nil {
			return nil, errE
		}
		documents = append(documents, docs...)

		logger.Info().Msg("test data properties generated successfully")

		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		mnemonics, errE := transform.Mnemonics(ctx, documents)
		if errE != nil {
			return nil, errE
		}

		docs, errE = xeno.Classes(mnemonics)
		if errE != nil {
			return nil, errE
		}
		documents = append(documents, docs...)

		logger.Info().Msg("test data classes generated successfully")

		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		return append(documents, testData...), nil
	})
}

// insertTestDataFiles inserts the test data attachments into the storage, under IDs derived from the
// test data namespace, its storage segment, and each file's path inside the test data files
// directory, which is what the documents linking to them expect.
func insertTestDataFiles(ctx context.Context, site internalSite.Site, files []fileToInsert, count *x.Counter) errors.E {
	for _, f := range files {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		count.Increment()

		p := filepath.Join(f.Dir, f.Path)
		file, err := os.Open(p) //nolint:gosec
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["path"] = f.Path
			return errE
		}

		_, errE := site.Base.InsertOrReplaceFile(ctx, []string{xeno.Namespace, xeno.FilesStorage, f.Path}, file, f.Filename)
		if errE != nil {
			_ = file.Close()
			errors.Details(errE)["path"] = f.Path
			return errE
		}

		errE = errors.WithStack(file.Close())
		if errE != nil {
			errors.Details(errE)["path"] = f.Path
			return errE
		}
	}

	return nil
}

func (c *PopulateCommand) populateSite(ctx context.Context, site internalSite.Site) (func(), errors.E) {
	logger := *zerolog.Ctx(ctx)
	logger.Info().Msg("populating")

	var documents []any
	var transformed []*document.D
	files := []fileToInsert{}

	// The test data set and the schema it needs are opt-in: without a test data directory only the
	// core documents are populated.
	if c.TestDataDir == "" {
		var errE errors.E
		documents, transformed, errE = base.GenerateCoreDocuments(ctx, nil)
		if errE != nil {
			return nil, errE
		}
	} else {
		testData, errE := loadTestData(ctx, logger, c.TestDataDir)
		if errE != nil {
			return nil, errE
		}

		files, errE = testDataFiles(c.TestDataDir)
		if errE != nil {
			return nil, errE
		}

		logger.Info().Int("count", len(testData)).Int("files", len(files)).Msg("loaded all test data")

		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		documents, transformed, errE = generateTestDataDocuments(ctx, logger, testData)
		if errE != nil {
			return nil, errE
		}
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
	size := x.NewCounter(int64(len(transformed) + len(files)))
	progress := indexer.Progress(logger, "indexing", nil)
	ticker := x.NewTicker(ctx, count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	// The files are inserted through the callback, so that all documents are inserted first and the
	// files go in while those are still being indexed.
	populateShutdown, errE := site.PopulateAndStart(ctx, transformed, func(doc *document.D) {
		count.Increment()
		logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
	}, func(ctx context.Context) errors.E {
		return insertTestDataFiles(ctx, site, files, count)
	}, count, size)
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
