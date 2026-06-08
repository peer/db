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
	"slices"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
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

// completeSessionTimeout bounds how long completing a document-editing session may take.
const completeSessionTimeout = 5 * time.Minute

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

	// IndexAncestorProperties enables claim propagation to transitive super-properties
	// when indexing: a claim for property X is also indexed for every ancestor of X
	// via SUBPROPERTY_OF. Disabled by default.
	IndexAncestorProperties bool

	// IndexingHooks transform a document for indexing. The bridge runs them, adapted to document
	// post-hooks (skipping on an incoming error), after DocumentPostHooks when fetching documents for
	// indexing, so the indexed document is the post-hook document with any indexing-specific
	// normalization applied. They are not run on the read/API path.
	IndexingHooks []func(ctx context.Context, doc *document.D) (*document.D, errors.E)

	// DocumentPreHooks are called before fetching the document from the store.
	DocumentPreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// DocumentPostHooks are called after fetching the document from the store.
	DocumentPostHooks []func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E)

	// FilePreHooks are called before fetching the file from the store.
	FilePreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// FilePostHooks are called after fetching the file from the store.
	FilePostHooks []func(
		ctx context.Context, data []byte, metadata *storage.FileMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) ([]byte, *storage.FileMetadata, store.Version, []store.Version, errors.E)

	// SearchQueryHook, when set, is called per request and returns an optional
	// filter query that is added (as a bool filter clause) to every search
	// query - results, facets and active-filter counts - so a site can limit
	// which documents searches can see based on the caller. A nil query means no
	// restriction. It is not applied to the corpus-wide ScoreFactor statistic or
	// the internal reference-score count, which run without a caller.
	SearchQueryHook func(ctx context.Context) (types.QueryVariant, errors.E)

	// RegisterWorkers are called in order to register workers for processing
	// background jobs before the river client is started. Each callback is
	// invoked once with the same *river.Workers. Downstream packages append
	// rather than assign so PeerDB's built-in workers are not silently overwritten.
	RegisterWorkers []func(context.Context, *river.Workers) errors.E

	// Data type for Store is on purpose not document.D so that we can serve it directly without doing first JSON unmarshal just to marshal it again immediately.
	documents   *store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes]
	coordinator *coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *DocumentCompleteMetadata]
	files       *storage.Storage
	bridge      *internalSearch.Bridge

	// workers is used to register workers before calling Start.
	workers *river.Workers

	listener    *internalStore.Listener
	riverClient *river.Client[pgx.Tx]

	// languageCodes maps a language document ID to its primary language subtag (e.g., "en").
	// It is captured from the converter in Start and surfaced via LanguageCodes.
	languageCodes map[identifier.Identifier]string
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
		json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes,
	]{
		Schema:        b.Schema,
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
		Prefix:                 "docs",
		DataType:               "jsonb",
		MetadataType:           "jsonb",
		CompleteSession:        b.completeDocumentSession,
		CompleteSessionTx:      b.completeDocumentSessionTx,
		CompleteSessionTimeout: completeSessionTimeout,
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
		// The document hooks are set from the base's hooks in Start, once the site has populated them.
		DocumentPreHooks:  nil,
		DocumentPostHooks: nil,
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

// indexingPostHook adapts an indexing hook, which only transforms the document, to a document
// post-hook. On an incoming error it skips the indexing hook and returns the error unchanged;
// otherwise it runs the hook and passes the metadata, version, and parent changesets through.
func indexingPostHook(hook func(ctx context.Context, doc *document.D) (*document.D, errors.E)) func(
	ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		if errE != nil {
			return doc, metadata, version, parentChangesets, errE
		}
		doc, errE = hook(ctx, doc)
		return doc, metadata, version, parentChangesets, errE
	}
}

// Start starts the base.
//
// Documents are documents with properties and vocabularies which are used
// to index documents for search.
//
// You have to call this or PopulateAndStart for each base after Init.
func (b *B) Start(ctx context.Context, documents []*document.D) (func(), errors.E) {
	// The bridge fetches documents for indexing through the same pre/post hooks as the read path, plus
	// the indexing hooks (adapted to document post-hooks) appended after them, so the indexed document
	// is the filtered and normalized one.
	b.bridge.DocumentPreHooks = b.DocumentPreHooks
	postHooks := slices.Clone(b.DocumentPostHooks)
	for _, hook := range b.IndexingHooks {
		postHooks = append(postHooks, indexingPostHook(hook))
	}
	b.bridge.DocumentPostHooks = postHooks

	// We build the converter first so that invalid input (e.g., unsupported
	// language priority) fails fast without leaving any resources running.
	converter, errE := internalSearch.NewConverter(
		documents, documents, documents, b.LanguagePriority,
		b.bridge.GetDocument,
	)
	if errE != nil {
		return nil, errE
	}

	converter.IndexAncestorProperties = b.IndexAncestorProperties
	converter.DetectLanguages = true
	converter.CountReferences = b.bridge.CountReferences

	// The converter derived language codes from the language documents while being built.
	// Capture them so the site can surface them via LanguageCodes.
	b.languageCodes = converter.LanguageCodes

	for _, register := range b.RegisterWorkers {
		errE := register(ctx, b.workers)
		if errE != nil {
			return nil, errE
		}
	}

	// We prepare the bridge startup before starting the river client.
	errE = b.bridge.Prepare(internalStore.WithFallbackDBContext(ctx, b.Schema, "bridge"), converter)
	if errE != nil {
		return nil, errE
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
	errE = b.listener.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "listener"))
	if errE != nil {
		return onShutdown, errE
	}

	return onShutdown, b.bridge.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "bridge"))
}
