package base

import (
	"context"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/transform"
)

// GenerateCoreDocuments generates and transforms all core documents
// (classes, properties, vocabularies) along with any additional documents.
func GenerateCoreDocuments(ctx context.Context, additional func(context.Context, []any) ([]any, errors.E)) ([]any, []*document.D, errors.E) {
	documents := []any{}

	// Properties are collected first so that mnemonics can be built for
	// Classes, which needs them for field descriptions.
	docs, errE := core.Properties()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

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

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	// Now add the rest of documents.
	docs, errE = core.Vocabularies()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	if additional != nil {
		docs, errE = additional(ctx, documents)
		if errE != nil {
			return nil, nil, errE
		}
		documents = append(documents, docs...)
	}

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

	return documents, transformed, nil
}

// PopulateAndStart inserts the given documents into the store, starts the base,
// waits for Elasticsearch to catch up, and then refreshes ElasticSearch index.
//
// Optional count and size counters can be provided to track ES indexing progress.
//
// You have to call this or Start for each base after Init.
func (b *B) PopulateAndStart(ctx context.Context, documents []*document.D, progress func(doc *document.D), count, size *x.Counter) errors.E {
	for _, doc := range documents {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		if progress != nil {
			progress(doc)
		}

		errE := b.InsertOrReplaceDocument(ctx, doc)
		if errE != nil {
			return errE
		}
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	errE := b.Start(ctx, documents)
	if errE != nil {
		return errE
	}

	errE = b.WaitUntilCaughtUp(ctx, count, size)
	if errE != nil {
		return errE
	}

	_, err := b.bridge.ESClient.Indices.Refresh().Index(b.bridge.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
