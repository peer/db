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
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
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
	Schema      string
	IndexPrefix string

	// StorageDir is the directory under which file contents are stored. The file store holds only
	// each file's content hash while the contents live on disk under StorageDir. It is required.
	StorageDir string

	// Levels is the ordered list of visibility level names (lowest to highest). The bridge indexes each
	// document into one index per level and accumulates inverse relations per level. The highest (last)
	// level must be the unfiltered superset containing every document, so the indexing hooks must not
	// filter anything at it.
	Levels []string

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

	// IndexingNormalizeHooks transform a document for indexing before it is augmented with embedded claims
	// and synthetic incoming inverse claims. The bridge runs them per visibility level, on that level's
	// copy of the document read from the store, with the level's visibility in ctx. A hook may return
	// store.ErrAccessDenied to hide the document at that level (it is then deleted from that level's
	// index). Because they shape the per-level document itself, they also drive the inverse-relation,
	// reference-target, and embedding accumulation. They are the only per-document hooks run when fetching
	// documents for indexing: the read-path document pre-hooks and post-hooks are not run during indexing,
	// so a site which wants their filtering during indexing calls them from a hook here itself. They are
	// not run on the read/API path.
	//
	// The metadata is the document's store metadata (nil for a freshly generated, not-yet-read document
	// passed to Start). It is shared across levels and hooks, so hooks must not mutate it.
	IndexingNormalizeHooks []func(ctx context.Context, doc *document.D, metadata *store.DocumentMetadata) (*document.D, errors.E)

	// IndexingFinalizeHooks transform a document for indexing after it has been augmented with embedded
	// claims and then synthetic incoming inverse claims, right before the conversion to the search
	// document, so related claims fetched via embedding can be post-processed. They run per visibility
	// level, on a private copy of the document. Their changes do not feed the inverse-relation,
	// reference-target, and embedding accumulation, nor the document-intrinsic fields (display label,
	// claim count, earliest time), which are all computed before augmentation. They are not run on the
	// read/API path.
	IndexingFinalizeHooks []func(ctx context.Context, doc *document.D) (*document.D, errors.E)

	// DocumentPreHooks are called before fetching the document from the store on the read/API path.
	// They are not called during indexing.
	DocumentPreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// DocumentPostHooks are called after fetching the document from the store on the read/API path.
	// They are not called during indexing.
	DocumentPostHooks []func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E)

	// FilePreHooks are called before fetching the file from the store.
	FilePreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// FilePostHooks are called after fetching the file from the store. The file is an open handle on
	// the contents; a hook that drops or replaces it (returns a different handle or a non-nil error)
	// is responsible for closing the handle it received.
	FilePostHooks []func(
		ctx context.Context, file io.ReadSeekCloser, metadata *storage.FileMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E)

	// SearchQueryHook, when set, is called per request and returns an optional
	// filter query that is added (as a bool filter clause) to every search
	// query - results, facets and active-filter counts - so a site can limit
	// which documents searches can see based on the caller. A nil query means no
	// restriction. It is not applied to the corpus-wide ScoreFactor statistic or
	// the internal reference-score count, which run without a caller.
	//
	// TODO: Gate search ranking to constant scores before returning a per-document (per-user) ACL filter here, to avoid leaking document existence through _score.
	//       A filter returned here drops documents from the result set but not from the relevance-scoring collection
	//       statistics (IDF and friends), so on a shared per-level index the _score of accessible hits leaks the existence of
	//       inaccessible documents. See the term-statistics leak TODO in ResultsGet for the full mechanism, the avoid-list,
	//       and the constant_score mitigation.
	SearchQueryHook func(ctx context.Context) (types.QueryVariant, errors.E)

	// Data type for Store is on purpose not document.D so that we can serve it directly without doing first JSON unmarshal just to marshal it again immediately.
	documents   *store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes]
	coordinator *coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *DocumentCompleteMetadata]
	files       *storage.Storage
	bridge      *internalSearch.Bridge

	listener *internalStore.Listener
	river    *internalStore.River

	// sessionDocs caches per edit session the latest committed document state, used by
	// AppendDocumentChange to validate the next operation against the state produced by
	// the previous one.
	sessionDocs *sessionDocCache

	// languageCodes maps a language document ID to its primary language subtag (e.g., "en").
	// It is captured from the converter in Start and surfaced via LanguageCodes.
	languageCodes map[identifier.Identifier]string
}

// Init initializes the base.
func (b *B) Init(
	ctx context.Context,
	dbpool *pgxpool.Pool, listener *internalStore.Listener,
	esClient *elasticsearch.TypedClient,
	r *internalStore.River,
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
		Prefix:                   "docs",
		DataType:                 "jsonb",
		MetadataType:             "jsonb",
		CompleteSession:          b.completeDocumentSession,
		CompleteSessionTx:        b.completeDocumentSessionTx,
		CompleteSessionOnErrorTx: b.completeSessionOnErrorTx,
		CompleteSessionTimeout:   completeSessionTimeout,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = c.Init(ctx, dbpool, nil, r)
	if errE != nil {
		return errE
	}

	files := &storage.Storage{
		Schema:             b.Schema,
		Prefix:             "files",
		Dir:                b.StorageDir,
		PrimaryCoordinator: &primaryCoordinator{Coordinator: c},
	}
	// We do not use the underlying store's Committed channel here so we pass nil as listener.
	errE = files.Init(ctx, dbpool, nil, r)
	if errE != nil {
		return errE
	}

	bridge := &internalSearch.Bridge{
		Store:       documents,
		ESClient:    esClient,
		IndexPrefix: b.IndexPrefix,
		// The normalize hooks are set from the base's indexing hooks in Start, once the site has populated them.
		NormalizeHooks: nil,
	}
	errE = bridge.Init(ctx, dbpool, listener, r)
	if errE != nil {
		return errE
	}

	b.documents = documents
	b.coordinator = c
	b.files = files
	b.bridge = bridge
	b.listener = listener
	b.river = r
	b.sessionDocs = newSessionDocCache()

	return nil
}

// AddWorker registers a river worker (implementation of jobs) for additional job kinds you can later
// submit through river client. Every job kind runs in its own queue named after the kind, with the given
// queue configuration. The kind's JobArgs must set the same queue through InsertOpts. It must be called
// after Init and before Start. Registration after the river client was started is a hard failure because
// river does not support it.
func AddWorker[T river.JobArgs](b *B, worker river.Worker[T], queueConfig river.QueueConfig) errors.E {
	return internalStore.RiverAddWorker(b.river, worker, queueConfig)
}

// QueueName derives the river queue name for a job kind. Every job kind runs in its own queue. The kind's
// JobArgs should use this in InsertOpts so its jobs land in the queue added by AddWorker.
func QueueName(kind string) string {
	return internalStore.RiverQueueName(kind)
}

// StartDocument is a document passed to Start as converter vocabulary, together with the metadata it was
// read with. For a freshly generated, not-yet-read document the metadata is nil.
type StartDocument struct {
	Document *document.D
	Metadata *store.DocumentMetadata
}

// Start starts the base.
//
// Documents are property, class, and language documents used to index
// documents for search. All three kinds must be provided.
//
// You have to call this or PopulateAndStart for each base after Init.
func (b *B) Start(ctx context.Context, documents []StartDocument) (func(), errors.E) {
	// The bridge fetches documents for indexing through the indexing normalize hooks only (the read-path
	// document pre-hooks and post-hooks are not run during indexing). The indexing finalize hooks run
	// later, inside the conversion, after the document is augmented with embedded claims and synthetic
	// incoming inverse claims.
	b.bridge.NormalizeHooks = b.IndexingNormalizeHooks

	// Build one converter and one ElasticSearch index per visibility level. We build them first so that
	// invalid input (e.g., an unsupported language priority) fails fast without leaving any resources running.
	targets := make([]internalSearch.Target, 0, len(b.Levels))
	for i, level := range b.Levels {
		index := internalSearch.LevelIndex(b.IndexPrefix, level)
		// Each level's converter resolves vocabulary (properties, classes, languages) as that level sees it,
		// so a vocab document or claim hidden at the level does not contribute to resolution there (for
		// example an inverse-property declaration hidden at the level then yields no inverse relation at that
		// level). documents is the unfiltered superset. documentsForLevel filters it to this level's view.
		levelDocuments, errE := b.documentsForLevel(ctx, level, documents)
		if errE != nil {
			return nil, errE
		}
		converter, errE := internalSearch.NewConverter(
			levelDocuments, levelDocuments, levelDocuments, b.LanguagePriority,
			b.bridge.GetDocument,
		)
		if errE != nil {
			return nil, errE
		}
		converter.IndexAncestorProperties = b.IndexAncestorProperties
		converter.DetectLanguages = true
		converter.CountReferences = b.bridge.CountReferencesFunc(index)
		converter.FinalizeHooks = b.IndexingFinalizeHooks
		if i == len(b.Levels)-1 {
			// The converter derived language codes from the language documents while being built.
			// The highest (last) level is the unfiltered superset, so its converter has the complete set.
			// We capture them so the site can surface them via LanguageCodes.
			b.languageCodes = converter.LanguageCodes
		}
		targets = append(targets, internalSearch.Target{Level: level, Index: index, Converter: converter})
	}

	// We prepare the bridge startup before starting the river client.
	errE := b.bridge.Prepare(internalStore.WithFallbackDBContext(ctx, b.Schema, "bridge"), targets)
	if errE != nil {
		return nil, errE
	}

	// Now we can start the river client. It will be stopped when ctx is cancelled.
	// After this, registering further workers (AddWorker) is a hard failure.
	errE = b.river.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "river"))
	if errE != nil {
		return nil, errE
	}

	// The session document cache sweep runs until ctx is cancelled.
	b.sessionDocs.Start(ctx)

	onShutdown := func() {
		// Wait for the client to stop.
		<-b.river.Client.Stopped()
	}

	// After that, we can start the listener.
	errE = b.listener.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "listener"))
	if errE != nil {
		return onShutdown, errE
	}

	return onShutdown, b.bridge.Start(internalStore.WithFallbackDBContext(ctx, b.Schema, "bridge"))
}

// documentsForLevel returns documents as seen at the given visibility level: each is run through the
// indexing normalize hooks at that level's visibility (the same hooks the bridge runs when fetching
// documents for indexing), dropping any the hooks deny. The hooks may mutate the document, so each runs
// on that level's own copy; the metadata is passed to the hooks as-is.
//
// With no indexing normalize hooks set (no per-level shaping) it returns the input documents unchanged.
func (b *B) documentsForLevel(ctx context.Context, level string, documents []StartDocument) ([]*document.D, errors.E) {
	out := make([]*document.D, 0, len(documents))

	if len(b.IndexingNormalizeHooks) == 0 {
		for _, sd := range documents {
			out = append(out, sd.Document)
		}
		return out, nil
	}

	ctx = auth.WithVisibility(ctx, level)
	ctx = zerolog.Ctx(ctx).With().Str("index", internalSearch.LevelIndex(b.IndexPrefix, level)).Logger().WithContext(ctx)

	for _, sd := range documents {
		doc, errE := sd.Document.Clone()
		if errE != nil {
			return nil, errE
		}
		denied := false
		for _, hook := range b.IndexingNormalizeHooks {
			doc, errE = hook(ctx, doc, sd.Metadata)
			if errors.Is(errE, store.ErrAccessDenied) {
				// The document is not visible at this level, so it is not part of this level's vocabulary.
				denied = true
				break
			}
			if errE != nil {
				return nil, errE
			}
		}
		if denied {
			continue
		}
		out = append(out, doc)
	}

	return out, nil
}
