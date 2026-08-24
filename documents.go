package peerdb

import (
	"context"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/store"
)

// fetchDocuments returns all documents which are instances of the given class by loading their latest
// stored versions. The documents of a class are listed through the metadata of their stored versions
// (see store.DocumentMetadata.InstanceOf), so no search index is needed.
//
// It reads the raw stored documents directly and unfiltered, without the read-path document hooks
// (and thus any permission checks).
func fetchDocuments(ctx context.Context, b *base.B, classID identifier.Identifier) ([]base.StartDocument, errors.E) {
	filter, errE := x.MarshalWithoutEscapeHTML(struct {
		InstanceOf []identifier.Identifier `json:"instanceOf"`
	}{[]identifier.Identifier{classID}})
	if errE != nil {
		return nil, errE
	}

	documents := []base.StartDocument{}
	var after *identifier.Identifier
	for {
		ids, errE := b.Documents().List(ctx, filter, after)
		if errE != nil {
			return nil, errE
		}
		for _, id := range ids {
			data, metadata, _, _, errE := b.Documents().GetLatest(ctx, id)
			if errors.Is(errE, store.ErrValueNotFound) {
				// This includes ErrValueDeleted, too.
				continue
			} else if errE != nil {
				return nil, errE
			}
			doc := new(document.D)
			errE = x.UnmarshalWithoutUnknownFields(data, doc)
			if errE != nil {
				return nil, errE
			}
			documents = append(documents, base.StartDocument{Document: doc, Metadata: metadata})
		}
		if len(ids) < store.MaxPageLength {
			break
		}
		after = &ids[len(ids)-1]
	}

	return documents, nil
}

// converterDocuments loads the documents the base's converters are built from: the property, class, and
// language documents (instances of the respective core meta-classes). Every path that starts the base
// from stored documents loads them through this, so they cannot drift apart. A site which stores none of
// them gets an empty list, which base.B.Start accepts.
func converterDocuments(ctx context.Context, b *base.B) ([]base.StartDocument, errors.E) {
	documents, errE := fetchDocuments(ctx, b, internalCore.PropertyClassID)
	if errE != nil {
		return nil, errE
	}
	languages, errE := fetchDocuments(ctx, b, internalCore.LanguageClassID)
	if errE != nil {
		return nil, errE
	}
	classes, errE := fetchDocuments(ctx, b, internalCore.ClassClassID)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, languages...)
	documents = append(documents, classes...)
	return documents, nil
}
