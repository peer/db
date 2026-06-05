package base

import (
	"context"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/store"
)

// LanguageCodes returns a map that maps language document ID to primary language subtag (e.g., "en").
func (b *B) LanguageCodes() map[identifier.Identifier]string {
	return b.languageCodes
}

// IndexedDocument returns the search document for the given document and metadata.
func (b *B) IndexedDocument(ctx context.Context, dataJSON json.RawMessage, metadata *store.DocumentMetadata) (*internalSearch.Document, errors.E) {
	return b.bridge.ConvertDocument(ctx, dataJSON, metadata)
}

// ResetBridgeProgress resets bridge progress so all commits are re-processed.
func (b *B) ResetBridgeProgress(ctx context.Context) errors.E {
	return b.bridge.ResetSeq(ctx)
}
