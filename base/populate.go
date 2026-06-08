package base

import (
	"context"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/transform"
)

// GenerateCoreDocuments generates and transforms all core documents
// (classes, properties, vocabularies). Optionally, beforeTransform can be
// used to modify, add, or remove documents before transforming.
func GenerateCoreDocuments(ctx context.Context, beforeTransform func(context.Context, []any) ([]any, errors.E)) ([]any, []*document.D, errors.E) {
	logger := zerolog.Ctx(ctx)

	documents := []any{}

	// Properties are collected first so that mnemonics can be built for
	// Classes, which needs them for field descriptions.
	docs, errE := core.Properties()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core properties generated successfully")

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	mnemonics, errE := transform.Mnemonics(ctx, documents)
	if errE != nil {
		return nil, nil, errE
	}

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	// Now we can add classes.
	docs, errE = core.Classes(mnemonics)
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core classes generated successfully")

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	// Now add the rest of documents.
	docs, errE = core.Vocabularies()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	logger.Info().Msg("core vocabularies generated successfully")

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	if beforeTransform != nil {
		documents, errE = beforeTransform(ctx, documents)
		if errE != nil {
			return nil, nil, errE
		}

		if ctx.Err() != nil {
			return nil, nil, errors.WithStack(ctx.Err())
		}
	}

	logger.Info().Int("count", len(documents)).Msg("generated documents")

	// Rebuild mnemonics with all documents.
	mnemonics, errE = transform.Mnemonics(ctx, documents)
	if errE != nil {
		return nil, nil, errE
	}

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	transformed, errE := transform.Documents(ctx, mnemonics, documents)
	if errE != nil {
		return nil, nil, errE
	}

	logger.Info().Int("count", len(transformed)).Msg("transformed documents")

	return documents, transformed, nil
}

// PopulateAndStart inserts the given documents into the store, starts the base,
// waits for Elasticsearch to catch up, and then refreshes ElasticSearch index.
//
// Optional count and size counters can be provided to track ES indexing progress.
//
// You have to call this or Start for each base after Init.
func (b *B) PopulateAndStart(
	ctx context.Context, documents []*document.D, progress func(doc *document.D), beforeWait func(ctx context.Context) errors.E, count, size *x.Counter,
) (func(), errors.E) {
	for _, doc := range documents {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		if progress != nil {
			progress(doc)
		}

		errE := b.InsertOrReplaceDocument(ctx, doc)
		if errE != nil {
			return nil, errE
		}
	}

	if ctx.Err() != nil {
		return nil, errors.WithStack(ctx.Err())
	}

	onShutdown, errE := b.Start(ctx, documents)
	if errE != nil {
		return onShutdown, errE
	}

	if beforeWait != nil {
		errE = beforeWait(ctx)
		if errE != nil {
			return onShutdown, errE
		}
	}

	errE = b.WaitUntilCaughtUp(ctx, count, size)
	if errE != nil {
		return onShutdown, errE
	}

	errE = b.bridge.Refresh(ctx)
	if errE != nil {
		return onShutdown, errE
	}

	return onShutdown, nil
}
