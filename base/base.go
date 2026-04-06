// Package base provides the main entry point for storing and managing data and files in PeerDB.
//
// It is a high-level component which wraps multiple lower-level components and offers
// an unified API for storing and managing data and files in PeerDB.
//
// It supports two types of data:
//
//   - PeerDB documents.
//   - Files.
package base

import (
	"context"
	"encoding/json"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	// TODO: Determine reasonable size for the buffer.
	// TODO: Add some monitoring of the channel contention.
	bridgeBufferSize = 100
)

// B is a base for data and files.
//
//nolint:lll
type B struct {
	Schema string
	Index  string

	// languagePriority defines per-language fallback order for display label resolution.
	// It maps a language to its ordered fallback languages for display label resolution.
	// If a language is not a key, fallback is only the undetermined language.
	// If a language has an empty slice, no fallback is attempted at all.
	//
	// All languages with keys in LanguagePriority are seen as enabled.
	LanguagePriority map[string][]string

	// Hooks are called in order to allow for modification of documents before they are indexed.
	IndexingHooks []func(doc *document.D) (*document.D, errors.E)

	// RegisterWorkers is called to register workers for processing background jobs.
	RegisterWorkers func(context.Context, *river.Workers) errors.E

	// Data type for Store is on purpose not document.D so that we can serve it directly without doing first JSON unmarshal just to marshal it again immediately.
	documents   *store.Store[json.RawMessage, *internalStore.DocumentMetadata, *internalStore.NoMetadata, *internalStore.NoMetadata, *internalStore.CommitMetadata, document.Changes]
	coordinator *coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *DocumentCompleteMetadata]
	files       *storage.Storage
	bridge      *internalSearch.Bridge

	// workers is used to register workers before calling Start.
	workers *river.Workers

	listener    *internalStore.Listener
	riverClient *river.Client[pgx.Tx]
}

// Init initializes the base.
func (b *B) Init(
	ctx context.Context,
	dbpool *pgxpool.Pool, listener *internalStore.Listener,
	esClient *elasticsearch.TypedClient,
	riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if b.documents != nil {
		return errors.New("already initialized")
	}

	documents := &store.Store[
		json.RawMessage, *internalStore.DocumentMetadata, *internalStore.NoMetadata, *internalStore.NoMetadata, *internalStore.CommitMetadata, document.Changes,
	]{
		Prefix:        "docs",
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
		CommittedSize: bridgeBufferSize,
	}
	errE := documents.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	c := &coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *DocumentCompleteMetadata]{
		Prefix:            "docs",
		DataType:          "jsonb",
		MetadataType:      "jsonb",
		CompleteSession:   b.completeDocumentSession,
		CompleteSessionTx: b.completeDocumentSessionTx,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = c.Init(ctx, dbpool, nil, b.Schema, riverClient, workers)
	if errE != nil {
		return errE
	}

	files := &storage.Storage{
		Schema:             b.Schema,
		Prefix:             "files",
		PrimaryCoordinator: &primaryCoordinator{Coordinator: c},
	}
	// We do not use the underlying store's Committed channel here so we pass nil as listener.
	errE = files.Init(ctx, dbpool, nil, riverClient, workers)
	if errE != nil {
		return errE
	}

	bridge := &internalSearch.Bridge{
		Store:    documents,
		ESClient: esClient,
		Index:    b.Index,
	}
	errE = bridge.Init(ctx, dbpool, listener, b.Schema, riverClient, workers)
	if errE != nil {
		return errE
	}

	b.documents = documents
	b.coordinator = c
	b.files = files
	b.bridge = bridge
	b.workers = workers
	b.listener = listener
	b.riverClient = riverClient

	return nil
}

// Start starts the base.
//
// Documents are documents with properties and vocabularies which are used
// to index documents for search.
//
// You have to call this or PopulateAndStart for each base after Init.
func (b *B) Start(ctx context.Context, documents []*document.D) (func(), errors.E) {
	if b.RegisterWorkers != nil {
		errE := b.RegisterWorkers(ctx, b.workers)
		if errE != nil {
			return nil, errE
		}
	}

	// Now we can start the river client.
	// It will be stopped when ctx is cancelled.
	err := b.riverClient.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "river"))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	onShutdown := func() {
		// Wait for the client to stop.
		<-b.riverClient.Stopped()
	}

	// After that, we can start the listener.
	errE := b.listener.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "listener"))
	if errE != nil {
		return onShutdown, errE
	}

	converter, errE := internalSearch.NewConverter(
		documents, documents, documents, b.LanguagePriority,
		func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
			// TODO: Make sure once we have permissions, that the public has the permission to read the document.
			doc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, id)
			if errE != nil {
				return nil, errE
			}
			return doc, nil
		},
	)
	if errE != nil {
		return onShutdown, errE
	}

	converter.Hooks = b.IndexingHooks

	return onShutdown, b.bridge.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "bridge"), converter)
}
