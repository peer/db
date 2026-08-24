package base

import (
	"context"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/store"
)

// LanguageCodes returns a map that maps language document ID to primary language subtag (e.g., "en").
func (b *B) LanguageCodes() map[identifier.Identifier]string {
	return b.languageCodes
}

// NamingProperties returns the properties a display label is picked from, in the order they are
// considered: the first one a document has a claim for is the one its label is taken from.
func (b *B) NamingProperties() []identifier.Identifier {
	return b.namingProperties
}

// IndexedDocument returns the search document for the given document and metadata.
//
// dataJSON is expected to be the raw stored document: indexing does not run the read-path document hooks.
// The indexing normalize hooks are applied here; the indexing finalize hooks run inside the conversion
// itself, after the document is augmented with embedded and incoming inverse claims.
func (b *B) IndexedDocument(ctx context.Context, dataJSON json.RawMessage, metadata *store.DocumentMetadata) (*internalSearch.Document, errors.E) {
	doc := new(document.D)
	errE := x.UnmarshalWithoutUnknownFields(dataJSON, doc)
	if errE != nil {
		return nil, errE
	}
	for _, hook := range b.IndexingNormalizeHooks {
		doc, errE = hook(ctx, doc, metadata)
		if errE != nil {
			return nil, errE
		}
	}
	return b.bridge.ConvertDocument(ctx, doc, metadata)
}

// DocumentHierarchyPaths returns the document's full hierarchy paths in the indexed toPath form
// ("<hierarchyProp>:<root>/.../<id>", ending with the document itself). A value reached through several
// parents or several value hierarchies has more than one path; a value in no value hierarchy gets a single
// self path ("__SELF__:<id>").
func (b *B) DocumentHierarchyPaths(ctx context.Context, id identifier.Identifier) ([]string, errors.E) {
	return b.bridge.DocumentHierarchyPaths(ctx, id)
}

// EnqueueAllForReindex enqueues every document for re-indexing and submits a job to drain the queue, so the
// bridge re-renders each document's current state into ElasticSearch, from the store and the materialized
// state, without writing either. It returns the number of documents enqueued. When count and size are non-nil
// they track progress.
func (b *B) EnqueueAllForReindex(ctx context.Context, count, size *x.Counter) (int, errors.E) {
	return b.bridge.EnqueueAllForReindex(ctx, count, size)
}
