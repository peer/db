package peerdb

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/indexer"
	"gitlab.com/peerdb/peerdb/transform"
)

func (c *PopulateCommand) populateSite(ctx context.Context, logger zerolog.Logger, site Site) errors.E { //nolint:maintidx
	logger.Info().Str("index", site.Index).Str("schema", site.Schema).Msg("populating")

	// We set fallback context values which are used to set application name on PostgreSQL connections.
	ctx = WithFallbackDBContext(ctx, "populate", site.Schema)

	documents := []any{}

	docs, errE := core.Classes(logger)
	if errE != nil {
		return errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core classes generated successfully")

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	docs, errE = core.Properties(logger)
	if errE != nil {
		return errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core properties generated successfully")

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	docs, errE = core.Vocabularies(logger)
	if errE != nil {
		return errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core vocabularies generated successfully")

	logger.Info().Int("count", len(documents)).Msg("generated all documents")

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	if c.SaveDir != "" {
		logger.Info().Str("path", c.SaveDir).Msg("saving structs as files into a directory")

		err := os.MkdirAll(c.SaveDir, 0o755) //nolint:gosec,mnd
		if err != nil {
			return errors.WithDetails(err, "path", c.SaveDir)
		}

		for _, doc := range documents {
			if ctx.Err() != nil {
				return errors.WithStack(ctx.Err())
			}

			id, errE := transform.ExtractDocumentID(doc)
			if errE != nil {
				errors.Details(errE)["id"] = id
				return errE
			}

			output, errE := x.MarshalWithoutEscapeHTML(doc)
			if errE != nil {
				return errE
			}

			var res bytes.Buffer
			err = json.Indent(&res, output, "", "  ")
			if err != nil {
				return errors.WithStack(err)
			}
			res.WriteString("\n")

			p := []string{c.SaveDir}
			for i := range len(id) - 1 {
				p = append(p, x.SafeFilename(id[i]))
			}
			path := filepath.Join(p...)

			filename := x.SafeFilename(id[len(id)-1] + ".json")

			err := os.MkdirAll(path, 0o755) //nolint:gosec,mnd
			if err != nil {
				return errors.WithDetails(err, "path", path)
			}

			path = filepath.Join(path, filename)

			err = os.WriteFile(path, res.Bytes(), 0o644) //nolint:gosec,mnd
			if err != nil {
				return errors.WithDetails(err, "path", path)
			}
		}

		logger.Info().Int("count", len(documents)).Msg("saved all structs")
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	mnemonics, errE := transform.Mnemonics(ctx, documents)
	if errE != nil {
		return errE
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	transformed, errE := transform.Documents(ctx, mnemonics, documents)
	if errE != nil {
		return errE
	}

	logger.Info().Int("count", len(transformed)).Msg("transformed all documents")

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	if c.OutputDir != "" {
		logger.Info().Str("path", c.OutputDir).Msg("saving documents as files into a directory")

		err := os.MkdirAll(c.OutputDir, 0o755) //nolint:gosec,mnd
		if err != nil {
			return errors.WithDetails(err, "path", c.OutputDir)
		}

		for _, doc := range transformed {
			if ctx.Err() != nil {
				return errors.WithStack(ctx.Err())
			}

			output, errE := x.MarshalWithoutEscapeHTML(doc)
			if errE != nil {
				errors.Details(errE)["id"] = doc.ID.String()
				return errE
			}

			var res bytes.Buffer
			err = json.Indent(&res, output, "", "  ")
			if err != nil {
				return errors.WithStack(err)
			}
			res.WriteString("\n")

			path := filepath.Join(c.OutputDir, doc.ID.String()+".json")

			err := os.WriteFile(path, res.Bytes(), 0o644) //nolint:gosec,mnd
			if err != nil {
				return errors.WithDetails(err, "path", path)
			}
		}

		logger.Info().Int("count", len(transformed)).Msg("saved all documents")
	}

	if c.DryRun {
		logger.Info().Msg("dry run, not inserting documents into the database")
		return nil
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	count := x.NewCounter(0)
	size := x.NewCounter(int64(len(transformed)))
	progress := indexer.Progress(logger, "indexing", func(e *zerolog.Event) {
		stats := site.ESProcessor.Stats()
		e.Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded)
	})
	ticker := x.NewTicker(ctx, count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	for _, doc := range transformed {
		if ctx.Err() != nil {
			break
		}

		count.Increment()

		logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE := InsertOrReplaceDocument(ctx, site.Store, &doc)
		if errE != nil {
			return errE
		}
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	// We wait for everything to be indexed into ElasticSearch.
	// TODO: Improve this to not have a busy wait.
	for {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		err := site.ESProcessor.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		stats := site.ESProcessor.Stats()
		c := count.Count()
		if c <= stats.Indexed {
			break
		}
		time.Sleep(time.Second)
	}

	_, err := site.ESClient.Refresh(site.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	stats := site.ESProcessor.Stats()
	logger.Info().
		Str("index", site.Index).Str("schema", site.Schema).
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).
		Msg("indexing done")

	return nil
}

// Run executes the populate command to populate database with documents.
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
