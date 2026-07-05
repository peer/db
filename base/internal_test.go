package base

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

func (b *B) TestingWithDocumentHooks(
	ctx context.Context, id identifier.Identifier, version *store.Version,
	fetch func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E),
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return b.withDocumentHooks(ctx, id, version, fetch)
}

func (b *B) TestingDocumentsForLevel(ctx context.Context, level string, documents []StartDocument) ([]*document.D, errors.E) {
	return b.documentsForLevel(ctx, level, documents)
}

// TestingAppendDocumentChangeUnvalidated appends a change directly through the coordinator,
// bypassing the full validation AppendDocumentChange does. It lets tests construct sessions
// with inapplicable operations, like sessions which predate apply-on-append validation.
func (b *B) TestingAppendDocumentChangeUnvalidated(ctx context.Context, session identifier.Identifier, data json.RawMessage, seqNo int64) (int64, errors.E) {
	return b.coordinator.Append(ctx, session, data, &documentChangeMetadata{
		At:   store.Time(time.Now().UTC()),
		User: store.UserFromContext(ctx),
	}, &seqNo)
}

// TestingSessionDocsLen returns the number of entries in the session document cache.
func (b *B) TestingSessionDocsLen() int {
	b.sessionDocs.mu.Lock()
	defer b.sessionDocs.mu.Unlock()
	return len(b.sessionDocs.entries)
}

// TestingSessionDocsExpire backdates every cached session document entry, as if
// sessionDocTTL had elapsed since it was stored.
func (b *B) TestingSessionDocsExpire() {
	b.sessionDocs.mu.Lock()
	defer b.sessionDocs.mu.Unlock()
	for _, entry := range b.sessionDocs.entries {
		entry.storedAt = time.Now().Add(-sessionDocTTL - time.Second)
	}
}

// TestingSessionDocsSweep runs the session document cache sweep, removing expired entries.
func (b *B) TestingSessionDocsSweep() {
	b.sessionDocs.sweep()
}

// TestingSessionDocsOperation returns the operation number the session's cached document
// state is at, or -1 when the cache has no entry for the session.
func (b *B) TestingSessionDocsOperation(session identifier.Identifier) int64 {
	doc, lastOperation := b.sessionDocs.Get(session)
	if doc == nil {
		return -1
	}
	return lastOperation
}

// TestingSessionDocsDelete removes the session's entry from the session document cache, so the
// next append rebuilds the state from the database.
func (b *B) TestingSessionDocsDelete(session identifier.Identifier) {
	b.sessionDocs.Delete(session)
}
