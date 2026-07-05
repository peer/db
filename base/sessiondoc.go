package base

import (
	"context"
	"sync"
	"time"

	"github.com/mohae/deepcopy"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// sessionDocTTL is how long a cached session document state is kept after it was last stored.
// The TTL bounds memory use of the cache: a cached state never becomes invalid (session
// operations are immutable), it just stops being worth keeping for an abandoned session.
const sessionDocTTL = 24 * time.Hour

// sessionDocSweepInterval is how often the background sweep removes expired entries.
const sessionDocSweepInterval = time.Hour

// sessionDocEntry is a cached document state of an edit session with all committed operations
// up to (and including) lastOperation applied. Because session operations are immutable and
// contiguous, the state after a given operation is deterministic. The doc is shared between
// readers and MUST NOT be mutated; apply changes to a clone.
type sessionDocEntry struct {
	doc           *document.D
	lastOperation int64
	storedAt      time.Time
}

// sessionDocCache is an in-process in-memory cache of the latest committed document state per
// edit session. AppendDocumentChange uses it to validate an incoming operation against the
// state produced by the previous one without replaying the whole session on every append.
type sessionDocCache struct {
	mu      sync.Mutex
	entries map[identifier.Identifier]*sessionDocEntry
}

func newSessionDocCache() *sessionDocCache {
	return &sessionDocCache{
		mu:      sync.Mutex{},
		entries: map[identifier.Identifier]*sessionDocEntry{},
	}
}

// Start runs the background sweep which removes expired entries, until ctx is canceled.
func (c *sessionDocCache) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(sessionDocSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.sweep()
			}
		}
	}()
}

func (c *sessionDocCache) sweep() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for session, entry := range c.entries {
		if now.Sub(entry.storedAt) >= sessionDocTTL {
			delete(c.entries, session)
		}
	}
}

// Get returns the session's cached document state and the operation number it is at, or nil
// when the cache has no entry for the session. The caller decides what the state is good for
// relative to the operation it is validating. The returned document is shared and MUST NOT be
// mutated; apply changes to a clone.
func (c *sessionDocCache) Get(session identifier.Identifier) (*document.D, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[session]
	if !ok {
		return nil, 0
	}
	return entry.doc, entry.lastOperation
}

// Store caches doc as the session's state after lastOperation. It never regresses: an entry
// for a later operation is kept instead.
func (c *sessionDocCache) Store(session identifier.Identifier, doc *document.D, lastOperation int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing, ok := c.entries[session]
	if ok && existing.lastOperation >= lastOperation {
		return
	}
	c.entries[session] = &sessionDocEntry{
		doc:           doc,
		lastOperation: lastOperation,
		storedAt:      time.Now(),
	}
}

// Delete removes the session's entry. Called when the session ends and no further operations
// can be appended to it.
func (c *sessionDocCache) Delete(session identifier.Identifier) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, session)
}

// cloneDocument returns a deep copy of doc.
//
// TODO: Move cloneDocument to D.Clone() and use it around the codebase.
func cloneDocument(doc *document.D) (*document.D, errors.E) {
	docCopy, ok := deepcopy.Copy(doc).(*document.D)
	if !ok {
		return nil, errors.New("deep copy returned unexpected type")
	}
	return docCopy, nil
}
