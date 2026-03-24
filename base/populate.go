package base

import (
	"context"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/transform"
)

// GenerateCoreDocuments generates and transforms all core documents
// (classes, properties, vocabularies) along with any additional documents.
func GenerateCoreDocuments(ctx context.Context, additional []any) ([]any, []*document.D, errors.E) {
	documents := []any{}

	docs, errE := core.Classes()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	docs, errE = core.Properties()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	if ctx.Err() != nil {
		return nil, nil, errors.WithStack(ctx.Err())
	}

	docs, errE = core.Vocabularies()
	if errE != nil {
		return nil, nil, errE
	}
	documents = append(documents, docs...)

	documents = append(documents, additional...)

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

	transformed, errE := transform.Documents(ctx, mnemonics, documents)
	if errE != nil {
		return nil, nil, errE
	}

	return documents, transformed, nil
}

// PopulateAndStart inserts the given documents into the store, starts the base,
// waits for Elasticsearch to catch up, and then refreshes ElasticSearch index.
//
// You have to call this or Start for each base after Init.
func (b *B) PopulateAndStart(ctx context.Context, documents []*document.D, progress func(doc *document.D)) errors.E {
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

	errE = b.WaitUntilCaughtUp(ctx)
	if errE != nil {
		return errE
	}

	_, err := b.bridge.ESClient.Indices.Refresh().Index(b.bridge.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
