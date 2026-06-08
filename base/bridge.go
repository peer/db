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

// IndexedDocument returns the search document for the given document and metadata.
//
// The caller is expected to have already applied the read-path DocumentPostHooks when fetching dataJSON.
func (b *B) IndexedDocument(ctx context.Context, dataJSON json.RawMessage, metadata *store.DocumentMetadata) (*internalSearch.Document, errors.E) {
	doc := new(document.D)
	errE := x.UnmarshalWithoutUnknownFields(dataJSON, doc)
	if errE != nil {
		return nil, errE
	}
	for _, hook := range b.IndexingHooks {
		doc, errE = hook(ctx, doc)
		if errE != nil {
			return nil, errE
		}
	}
	// It passes a nil generation so the converted document's own info is computed but not cached.
	return b.bridge.ConvertDocument(ctx, doc, metadata, nil)
}

// ResetBridgeProgress resets bridge progress so all commits are re-processed.
func (b *B) ResetBridgeProgress(ctx context.Context) errors.E {
	return b.bridge.ResetSeq(ctx)
}
