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
	"net/http"
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
	// level must be the unfiltered superset containing every document, so the indexing normalize hooks
	// must not filter anything at it. The indexing source check is not bound by that: it may deny a
	// document at any level, including the highest, because a denied document keeps its own entry.
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

	// Roles maps role names to the permission actions each role grants. Indexing evaluates with it
	// which roles and users may read each document and records them on the document's search entry,
	// which the default search query hook matches against the caller (see DefaultSearchQueryHook),
	// and the default end-edit permission check evaluates it at completion time (see
	// DefaultEndEditPermissionCheck). Role grants are baked into the index, so changing them requires
	// a full reindex.
	Roles map[string]auth.RoleGrants

	// IndexingNormalizeHooks transform a document for indexing before it is augmented with embedded claims
	// and synthetic incoming inverse claims. The bridge runs them per visibility level, on that level's
	// copy of the document read from the store, with the level's visibility in ctx and no caller identity
	// (no subject and no roles), so a hook cannot, and must not, differentiate by caller: its output at a
	// level is one shared rendering for every searcher of the level. A hook may return
	// auth.ErrAccessDenied to hide the document at that level (it is then deleted from that level's
	// index). Because they shape the per-level document itself, they also drive the inverse-relation,
	// reference-target, and embedding accumulation. Together with IndexingSourceCheck they are the only
	// per-document hooks run when fetching documents for indexing: the read-path document pre-hooks and
	// post-hooks are not run during indexing, so a site which wants their filtering during indexing calls
	// them from a hook here itself. They are not run on the read/API path.
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

	// IndexingSourceCheck, when set, decides per visibility level whether a document is a source at that
	// level: visible to everyone who can search the level, not only to a subset of its readers. The bridge
	// runs it on the level's normalized document (after IndexingNormalizeHooks), with the level's
	// visibility in ctx. Returning auth.ErrAccessDenied keeps the document's own entry in the level's
	// index but excludes the document from everything which flows into other documents' entries at that
	// level: display labels and hierarchy paths of references to it, claims embedded from it, the inverse
	// relations its relation claims produce, reference counting (its claims are skipped by
	// counts.references), and the schema the level's converter is built from (its property, class, and
	// language documents). When its outcome for a document changes between the document's versions, every
	// document referencing it and every document embedding from it is re-indexed, so entries rendered
	// under the old outcome do not linger. Any other error aborts indexing.
	//
	// This maintains the indexing invariant: an entry may contain only data which everyone who can
	// retrieve that entry may see. A query filter (SearchQueryHook) admits or hides an entry whole, per
	// searcher, and search (matching, aggregations, sorting, counts) then exposes everything the entry
	// contains, so retrieval is never per entry part. For the document's own data the filter is
	// therefore enough even when the document is read-restricted: it can narrow the entry's retrievers
	// to exactly the document's readers. That does not extend to data rendered into the entry from
	// another document, which is exposed to this entry's retrievers rather than to the other document's
	// readers: such data may come only from documents everyone searching the level may see, and this
	// check decides which those are (i.e. they can be "source" of data for other documents during indexing).
	// A site whose levels each contain only documents everyone at the level may read does not need the
	// check (nil means every document is a source, which is then correct). A site which indexes
	// read-restricted documents into a level must set it, so those documents keep their own (query-filtered)
	// entries without leaking into entries which other searchers can retrieve.
	//
	// PeerDB assigns a default before the site customizer runs (see DefaultIndexingSourceCheck), so a
	// customizer can keep, wrap, replace, or set it to nil. It denies documents whose role grants do not
	// make them readable by every searcher of a level.
	//
	// The check must be a pure function of the document, its metadata, and the level: it is evaluated
	// only when the document is indexed, so an outcome change from anywhere else (for example a site
	// configuration change) requires a full reindex.
	IndexingSourceCheck func(ctx context.Context, doc *document.D, metadata *store.DocumentMetadata) errors.E

	// DocumentPreHooks are called before fetching the document from the store on the read/API path.
	// They are not called during indexing. PeerDB seeds the list with the default
	// permission-enforcing hook before the site customizer runs, so a customizer appending its own
	// hooks keeps the enforcement; replacing it means assigning the list anew.
	DocumentPreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// DocumentPostHooks are called after fetching the document from the store on the read/API path.
	// They are not called during indexing. PeerDB seeds the list with the default permission-enforcing
	// hook before the site customizer runs, so a customizer appending its own hooks keeps the enforcement.
	// Replacing it means assigning the list anew. Besides the fetched document they receive the document at
	// its latest version, fetched raw: for a read of the latest version it has the same content as
	// the fetched document but is never the same instance (so transforming the fetched document does
	// not affect it), and it is nil when the document is deleted at its latest version (the read
	// error then reports the deletion, so access to versions of a deleted document is for the hooks
	// to decide by transforming the error). It is input for permission checks and transformations,
	// is shared between the hooks, and must not be modified; hooks return (possibly transformed)
	// only the fetched document.
	DocumentPostHooks []func(
		ctx context.Context, doc, latest *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E)

	// SessionDocumentHooks are called on the state of an edit session (see SessionDocumentRaw) before it is
	// handed to a caller, the way DocumentPostHooks are called on a document read from the store. The
	// state is built raw, so nothing else applies a site's filtering to it: a site which filters
	// documents on the read path wants the same filtering here. They are called neither on the read/API
	// path for documents nor during indexing, and neither are the document hooks called on a session's
	// state. PeerDB seeds the list with the default permission-enforcing hook before the site customizer
	// runs, so a customizer appending its own hooks keeps the enforcement; replacing it means assigning
	// the list anew.
	//
	// Besides the state they receive the metadata the session began with: the version it edits from (nil
	// when it creates the document), who opened it and when. Permissions are decided on the state itself,
	// which is what the session's own checks do, so a permission claim added within the session counts
	// here as well. Returning auth.ErrAccessDenied hides the state from the caller, and so does returning
	// no document at all; the hooks after the one which does either are not called.
	SessionDocumentHooks []func(
		ctx context.Context, doc *document.D, beginMetadata *DocumentBeginMetadata,
	) (*document.D, errors.E)

	// FilePreHooks are called before fetching the file from the store. PeerDB seeds the list with
	// the default permission-enforcing hook before the site customizer runs, so a customizer
	// appending its own hooks keeps the enforcement; replacing it means assigning the list anew.
	FilePreHooks []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

	// FilePostHooks are called after fetching the file from the store. PeerDB seeds the list with
	// the default permission-enforcing hook before the site customizer runs, so a customizer
	// appending its own hooks keeps the enforcement. Teplacing it means assigning the list anew.
	// The file is an open handle on the contents. A hook that drops or replaces it (returns a different
	// handle or a non-nil error) is responsible for closing the handle it received. Besides the fetched
	// file they receive the version of the file's latest version, read raw: for a read of the latest
	// version it is the fetched version itself, and it is nil when the file is deleted at its latest
	// version (the read error then reports the deletion, so access to versions of a deleted file is
	// for the hooks to decide by transforming the error). It is input for permission checks, is
	// shared between the hooks, and must not be modified.
	FilePostHooks []func(
		ctx context.Context, file io.ReadSeekCloser, latestVersion *store.Version, metadata *storage.FileMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E)

	// EndEditPermissionCheck, when set, is called when an edit session with changes completes, both
	// when the session is committed and when it is discarded (discarding destroys the session and
	// its changes for everyone, so it requires the same permission as committing). It receives the
	// user who ended the session, the roles recorded when the session was ended, and the session's
	// resulting document. It verifies only that the ender may complete the session: the content of
	// the session's changes is not re-evaluated here, because each change was authorized against
	// its author when it was appended (see ChangePermissionCheck). A non-nil error rejects the
	// completion: the session completes as errored and nothing is committed. Sessions ended through
	// a context marked with WithSystemSession skip the check, and a session the application owns (see
	// DocumentBeginMetadata.System) can only be ended that way. PeerDB assigns a default before the
	// site customizer runs (see DefaultEndEditPermissionCheck), so a customizer can keep, wrap,
	// replace, or disable it (by setting it to nil).
	EndEditPermissionCheck func(user *store.User, roles []string, doc *document.D) errors.E

	// ChangePermissionCheck, when set, is called for every change appended to an edit session, with
	// the session's document state before the change, the state with the change applied, and the
	// change itself. It is where the content of a change is authorized: per change, against the
	// appending caller in ctx, at append time, and that authorization is final (it is not
	// re-evaluated when the session completes, see EndEditPermissionCheck). A non-nil error rejects
	// the append and nothing is stored. Both document states are shared and must not be modified.
	// Changes appended through a context marked with WithSystemSession skip the check, and a session
	// the application owns (see DocumentBeginMetadata.System) can only be appended to that way. PeerDB
	// assigns a default before the site customizer runs (authorizing each change against the
	// caller's permissions), so a customizer can keep, wrap, replace, or disable it (by setting it
	// to nil).
	ChangePermissionCheck func(ctx context.Context, before, after *document.D, change document.Change) errors.E

	// CreateDocumentSeed, when set, is called when a create session is opened through the HTTP API,
	// with the API request, and returns the session's first changes, before any client change: the
	// default processes the request (the initial claims requested through the query string) and
	// grants the creator the default permission actions. The changes have to be built for the
	// session and the document base as operations numbered from 1. The caller appends the
	// returned changes to the session and applies them to the session's initial document state.
	// PeerDB assigns its default before the site customizer runs, so a customizer can keep, wrap,
	// replace, or set it to nil (with nil nothing is seeded and the request is not processed).
	CreateDocumentSeed func(ctx context.Context, req *http.Request, session identifier.Identifier, docBase []string) (document.Changes, errors.E)

	// SearchQueryHook, when set, is called per request and returns an optional
	// filter query that is added (as a bool filter clause) to every search
	// query - results, facets and active-filter counts - so a site can limit
	// which documents searches can see based on the caller. A nil query means no
	// restriction. It is not applied to the corpus-wide ScoreFactor statistic or
	// the internal reference-score count, which run without a caller.
	//
	// PeerDB assigns a default before the site customizer runs (see DefaultSearchQueryHook), so a
	// customizer can keep, wrap, replace, or disable it (by setting it to nil, search is then
	// unrestricted). The default hides the documents the caller cannot read, matching the read path.
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
		MetadataIndex:            true,
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
		// The normalize hooks and the source check are set from the base's indexing hooks in Start, once the
		// site has populated them.
		NormalizeHooks: nil,
		SourceCheck:    nil,
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

// StartDocument is a schema document (a property, class, or language document) passed to Start for
// building the converters, together with the metadata it was read with. For a freshly generated,
// not-yet-read document the metadata is nil.
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
	// incoming inverse claims. The source check runs on the normalized documents and gates what may flow
	// from a document into other documents' entries.
	b.bridge.NormalizeHooks = b.IndexingNormalizeHooks
	b.bridge.SourceCheck = b.IndexingSourceCheck

	// Build one converter and one ElasticSearch index per visibility level. We build them first so that
	// invalid input (e.g., an unsupported language priority) fails fast without leaving any resources running.
	targets := make([]internalSearch.Target, 0, len(b.Levels))
	for i, level := range b.Levels {
		index := internalSearch.LevelIndex(b.IndexPrefix, level)
		// Each level's converter resolves its schema (properties, classes, languages) as that level sees it,
		// so a schema document or claim hidden at the level does not contribute to resolution there (for
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
		converter.CountReferences = b.bridge.CountReferencesFunc(level)
		converter.Roles = b.Roles
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
// documents for indexing), dropping any the hooks deny, and then through the indexing source check,
// dropping any it denies. These schema documents shape every entry converted at the level (property and
// class labels, hierarchy, inverse and embed configuration), so they are source data and only sources
// belong among them. The hooks may mutate the document, so each runs on that level's own copy; the
// metadata is passed to the hooks as-is.
//
// With no indexing normalize hooks and no source check set (no per-level shaping) it returns the input
// documents unchanged.
func (b *B) documentsForLevel(ctx context.Context, level string, documents []StartDocument) ([]*document.D, errors.E) {
	out := make([]*document.D, 0, len(documents))

	if len(b.IndexingNormalizeHooks) == 0 && b.IndexingSourceCheck == nil {
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
			if errors.Is(errE, auth.ErrAccessDenied) {
				// The document is not visible at this level, so it is not part of this level's schema.
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
		if b.IndexingSourceCheck != nil {
			errE = b.IndexingSourceCheck(ctx, doc, sd.Metadata)
			if errors.Is(errE, auth.ErrAccessDenied) {
				// The document is not a source at this level, so it is not part of this level's schema.
				continue
			}
			if errE != nil {
				return nil, errE
			}
		}
		out = append(out, doc)
	}

	return out, nil
}
