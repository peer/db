package base

import (
	"context"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// withDocumentHooks runs the document pre-hooks, reads the raw document via fetch, unmarshals it, and
// runs the document post-hooks, returning the post-hook document together with its metadata, version,
// and parent changesets.
//
// version is passed to the pre-hooks and is nil for a latest read. fetch is the store read (GetLatest,
// or Get at a specific version) and returns the raw document, which is nil when the document is deleted
// at that version. The post-hooks run even when doc is nil so they can observe and transform the error.
func (b *B) withDocumentHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.DocumentPreHooks {
		errE := hook(ctx, id, version)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	data, metadata, resolved, parentChangesets, errE := fetch()
	var doc *document.D
	if data != nil {
		doc = new(document.D)
		errE2 := x.UnmarshalWithoutUnknownFields(data, doc)
		if errE2 != nil {
			return nil, metadata, resolved, parentChangesets, errors.Join(errE, errE2)
		}
	}
	for _, hook := range b.DocumentPostHooks {
		doc, metadata, resolved, parentChangesets, errE = hook(ctx, doc, metadata, resolved, parentChangesets, errE)
	}
	return doc, metadata, resolved, parentChangesets, errE
}

func (b *B) withHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.DocumentPreHooks {
		errE := hook(ctx, id, version)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	data, metadata, resolved, parentChangesets, errE := fetch()
	if len(b.DocumentPostHooks) > 0 {
		var doc *document.D
		if data != nil {
			doc = new(document.D)
			errE2 := x.UnmarshalWithoutUnknownFields(data, doc)
			if errE2 != nil {
				return nil, metadata, resolved, parentChangesets, errors.Join(errE, errE2)
			}
		}
		for _, hook := range b.DocumentPostHooks {
			doc, metadata, resolved, parentChangesets, errE = hook(ctx, doc, metadata, resolved, parentChangesets, errE)
		}
		if doc != nil {
			var errE2 errors.E
			data, errE2 = x.MarshalWithoutEscapeHTML(doc)
			if errE != nil {
				return nil, metadata, resolved, parentChangesets, errors.Join(errE, errE2)
			}
		} else {
			data = nil
		}
	}
	return data, metadata, resolved, parentChangesets, errE
}
