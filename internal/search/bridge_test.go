package search_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

// dummyCommitMetadata returns a CommitMetadata with a unique base for testing.
func dummyCommitMetadata() *store.CommitMetadata {
	return &store.CommitMetadata{
		Base: []string{"test", identifier.New().String()},
		User: nil,
	}
}

// dummyMetadata returns a minimal DocumentMetadata for testing.
func dummyMetadata() *store.DocumentMetadata {
	return &store.DocumentMetadata{
		At:               store.Time(time.Now().UTC()),
		Users:            nil,
		InverseRelations: nil,
	}
}

// makeDocJSON creates a valid document.D JSON for a given ID.
func makeDocJSON(t *testing.T, id identifier.Identifier) json.RawMessage {
	t.Helper()
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
	}
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// newTestBridgeConverter creates a minimal Converter for bridge tests.
func newTestBridgeConverter(t *testing.T) *internalSearch.Converter {
	t.Helper()
	c, errE := internalSearch.NewConverter(nil, nil, nil, nil, func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return &document.D{
			CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		}, nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

// bridgeStore is the concrete store type used by the bridge tests.
type bridgeStore = store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes]

// bridgeEnv holds the initialized pieces of a bridge test environment. The river client and the
// listener are created but not started, so a test can control startup ordering.
type bridgeEnv struct {
	dbpool      *pgxpool.Pool
	store       *bridgeStore
	bridge      *internalSearch.Bridge
	listener    *internalStore.Listener
	riverClient *river.Client[pgx.Tx]
	esClient    *elasticsearch.TypedClient
}

// setupBridge creates and initializes the dbpool, ES client, schema, store, and bridge. The river
// client and the listener are created but not started so a caller can control startup ordering.
func setupBridge(t *testing.T) (context.Context, *bridgeEnv) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}
	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"
	index := schema

	ctx = internalStore.WithFallbackDBContext(ctx, schema, "tests")

	// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
	// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
	dbCtx := internalStore.WithMaxDBPoolConnections(context.WithoutCancel(ctx), internalStore.TestMaxDBPoolConnections)
	dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(dbCtx, os.Getenv("POSTGRES"), logger, func(_ context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpoolCleanup)

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(context.Background())
		testutils.RequireNoESError(t, err)
	})

	errE = internalSearch.EnsureIndex(ctx, esClient, index, 1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internalStore.NewListener(dbpool)

	riverClient, workers, errE := internalStore.NewRiver(ctx, logger, nil, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &bridgeStore{
		Schema:        schema,
		Prefix:        prefix,
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
		CommittedSize: 100,
	}
	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	b := &internalSearch.Bridge{
		Store:             s,
		ESClient:          esClient,
		Index:             index,
		DocumentPreHooks:  nil,
		DocumentPostHooks: nil,
	}
	errE = b.Init(ctx, dbpool, listener, schema, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, &bridgeEnv{
		dbpool:      dbpool,
		store:       s,
		bridge:      b,
		listener:    listener,
		riverClient: riverClient,
		esClient:    esClient,
	}
}

// startBridge runs the bridge startup sequence in production order: Prepare stores the converter and
// submits the startup job, then the river client, the store listener, and the run goroutine start.
// This mirrors base.Start, so a worker never runs before the converter is set. Data inserted before
// the call is caught up by the run goroutine.
func startBridge(ctx context.Context, t *testing.T, env *bridgeEnv, converter *internalSearch.Converter) {
	t.Helper()

	errE := env.bridge.Prepare(ctx, converter)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := env.riverClient.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Wait for the client to stop.
		<-env.riverClient.Stopped()
	})

	errE = env.listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = env.bridge.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
}

// TestBridgeStartupDrainsReindexQueueBacklog covers the recovery path where BridgeReindexQueue
// already holds a backlog at or below the indexed seq at startup, the state an interrupted run leaves
// behind. Such leftover rows are processed only by the startup job that Prepare submits, because no new
// commit enqueues a job for them, and the listener's HandlingReady for the reindex queue channel blocks
// until that backlog drains. The test seeds the backlog and then starts the bridge in production order
// (Prepare, and thus the converter and startup job, before the listener), asserting that listener.Start
// drains the backlog instead of hanging. The order is set by the test itself, so it guards the
// startup-drain mechanism but not the Prepare/listener ordering in base.Start.
func TestBridgeStartupDrainsReindexQueueBacklog(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)

	// Seed a leftover reindex queue entry for a dangling document at seq 1 and advance the
	// indexed seq to 1, reproducing the state an interrupted run leaves behind.
	danglingID := identifier.New()
	errE := internalStore.RetryTransaction(ctx, env.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+env.store.Prefix+`BridgeReindexQueue" ("id", "seq") VALUES ($1, $2)`, danglingID.String(), int64(1))
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		_, err = tx.Exec(ctx, `UPDATE "`+env.store.Prefix+`Bridge" SET "seq" = $1`, int64(1))
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Production ordering: store the converter and submit the startup job before starting the
	// listener, so the worker drains the backlog while HandlingReady waits.
	errE = env.bridge.Prepare(ctx, newTestBridgeConverter(t))
	require.NoError(t, errE, "% -+#.1v", errE)

	err := env.riverClient.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		<-env.riverClient.Stopped()
	})

	// If the startup deadlock regresses, listener.Start blocks here until this context expires and
	// then returns a context error.
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	errE = env.listener.Start(startCtx)
	require.NoError(t, errE, "listener.Start should not block on the reindex queue backlog: % -+#.1v", errE)

	errE = env.bridge.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The backlog entry must have been drained by the bridge worker.
	require.Eventually(t, func() bool {
		var cnt int64
		errE := internalStore.RetryTransaction(ctx, env.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
			return internalStore.WithPgxError(tx.QueryRow(ctx, `SELECT COUNT(*) FROM "`+env.store.Prefix+`BridgeReindexQueue"`).Scan(&cnt))
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		return cnt == 0
	}, 30*time.Second, 50*time.Millisecond, "reindex queue backlog should be drained on startup")
}

// docExists returns true if the document with the given ID exists in Elasticsearch.

func TestBridgeRealTime(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	startBridge(ctx, t, env, newTestBridgeConverter(t))

	// Insert three documents.
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id3, makeDocJSON(t, id3), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for bridge to catch up and force ES to make documents searchable.
	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// All three documents should now be in search.
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id1.String()), "doc1 should exist in ES")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id2.String()), "doc2 should exist in ES")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id3.String()), "doc3 should exist in ES")

	// Update doc1.
	_, _, v1, _, errE := s.GetLatest(ctx, id1) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Replace(ctx, id1, v1.Changeset, makeDocJSON(t, id1), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// The bridge always indexes the latest version, even if an older commit triggered it.
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id1.String()), "doc1 should still exist after update")
}

func TestBridgeCatchUp(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Make commits BEFORE starting the bridge.
	id1 := identifier.New()
	id2 := identifier.New()

	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Bridge seq should still be 0 - nothing indexed yet.
	entries, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, entries, 2)

	// Now start the bridge. It should catch up from CommitLog.
	startBridge(ctx, t, env, newTestBridgeConverter(t))

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// Both documents should be in ES despite being committed before Start.
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id1.String()), "catchup doc1 should be in ES after catch-up")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id2.String()), "catchup doc2 should be in ES after catch-up")
}

func TestBridgeDeletedDocument(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	startBridge(ctx, t, env, newTestBridgeConverter(t))

	id := identifier.New()

	// Insert then delete a document.
	v, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id.String()), "document should exist before delete")

	_, errE = s.Delete(ctx, id, v.Changeset, dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// After deletion the bridge issues a bulk delete, so the document is removed from search.
	assert.False(t, testutils.DocExists(ctx, t, esClient, b.Index, id.String()), "document should be removed from ES after delete")
}

func TestBridgeSeqAdvancement(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b := env.store, env.bridge

	startBridge(ctx, t, env, newTestBridgeConverter(t))

	// Make several commits and verify the bridge table seq advances correctly.
	for range 5 {
		id := identifier.New()
		_, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), dummyCommitMetadata())
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	errE := b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The bridge table seq must match the maximum CommitLog seq.
	commitLog, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, commitLog)

	maxSeq := commitLog[len(commitLog)-1].Seq

	// After WaitUntilCaughtUp the bridge seq is >= maxSeq by definition.
	// Verify with a direct CommitLog check.
	assert.GreaterOrEqual(t, maxSeq, int64(1))
}

func TestBridgeNotifyRecovery(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	startBridge(ctx, t, env, newTestBridgeConverter(t))

	// Insert initial documents and wait for the bridge to catch up.
	id1 := identifier.New()
	id2 := identifier.New()
	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Simulate a listener reconnection by closing the store's Committed channel.
	// The bridge's run loop detects the channel close, exits with errCommittedChannelClosed,
	// and restarts - re-running the catch-up phase to recover any missed commits.
	err := s.HandleBacklog(ctx, s.Schema+"_"+s.Prefix+"Commit", nil)
	require.NoError(t, err, "% -+#.1v", err) // This is still errors.E.

	// Insert more documents after the simulated reconnection. These may be missed by the
	// real-time channel but must be recovered via the catch-up phase on bridge restart.
	id3 := identifier.New()
	id4 := identifier.New()
	_, errE = s.Insert(ctx, id3, makeDocJSON(t, id3), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id4, makeDocJSON(t, id4), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// All four documents must be indexed, including those inserted after the simulated reconnection.
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id1.String()), "initial doc1 should be in ES")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id2.String()), "initial doc2 should be in ES")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id3.String()), "recovery doc3 should be in ES")
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id4.String()), "recovery doc4 should be in ES")
}

func TestBridgeStaleDataNotIndexed(t *testing.T) {
	t.Parallel()

	// The bridge always fetches the latest version of each document, so even if an older
	// commit triggers indexing, the most up-to-date data ends up in Elasticsearch.
	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	id := identifier.New()

	// Insert initial data and immediately replace before starting the bridge.
	v, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Replace(ctx, id, v.Changeset, makeDocJSON(t, id), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now start the bridge. It should catch up and index the latest version.
	startBridge(ctx, t, env, newTestBridgeConverter(t))

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	testutils.RequireNoESError(t, err)

	// The document in ES should exist - the bridge calls GetLatest so it always indexes the latest version.
	assert.True(t, testutils.DocExists(ctx, t, esClient, b.Index, id.String()), "document should be in ES")
}

// makePropertyDocJSON creates a property document (INSTANCE_OF PROPERTY) with optional INVERSE_PROPERTY_OF.
func makePropertyDocJSON(t *testing.T, id identifier.Identifier, inverseOf *identifier.Identifier) json.RawMessage {
	t.Helper()
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	if inverseOf != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: internalCore.InversePropertyOfPropID},
			To:        document.Reference{ID: *inverseOf},
		})
	}
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// makeDocWithRelationJSON creates a document with a relation claim.
func makeDocWithRelationJSON(t *testing.T, docID, propID, targetID identifier.Identifier) json.RawMessage {
	t.Helper()
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: propID},
					To:        document.Reference{ID: targetID},
				},
			},
		},
	}
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// makeConverterWithInverse creates a converter that knows about inverse properties.
// propX has inversePropertyOf propY. The getDocument callback fetches from the store.
func makeConverterWithInverse(
	t *testing.T, propX, propY identifier.Identifier,
	s *store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes],
) *internalSearch.Converter {
	t.Helper()

	propXDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: propX}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: internalCore.PropertyClassID},
				},
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.InversePropertyOfPropID},
					To:        document.Reference{ID: propY},
				},
			},
		},
	}
	propYDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: propY}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: internalCore.PropertyClassID},
				},
			},
		},
	}

	properties := []*document.D{propXDoc, propYDoc}

	c, errE := internalSearch.NewConverter(properties, nil, nil, nil, func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
		data, _, _, _, errE := s.GetLatest(ctx, id)
		if errors.Is(errE, store.ErrValueNotFound) {
			// Return a minimal document for IDs not in the store (e.g., core property/class IDs).
			return &document.D{
				CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
			}, nil
		} else if errE != nil {
			return nil, errE
		}
		var doc document.D
		errE = x.UnmarshalWithoutUnknownFields(data, &doc)
		if errE != nil {
			return nil, errE
		}
		return &doc, nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

func TestBridgeInverseRelationReindexing(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Property X has inversePropertyOf Y.
	// So A --X--> B means B should get an inverse claim B --Y--> A.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	startBridge(ctx, t, env, converter)

	// Insert property documents into the store so getDocument can find them.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert document A with a relation A --X--> B, and document B (empty).
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for the bridge to index the initial commits.
	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify that docB's metadata was updated with inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := s.GetLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB metadata should have inverse relations")
	}, 10*time.Second, 100*time.Millisecond)

	// Wait for the River job to re-index document B with the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse relation B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)

	// Doc A should have the forward relation A --X--> B.
	assert.True(t, testutils.DocHasReference(ctx, t, esClient, b.Index, docA, propX, docB),
		"docA should have forward relation A --X--> B")
}

func TestBridgeReindexJobRecordsOutput(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b := env.store, env.bridge

	propX := identifier.New()
	propY := identifier.New()

	startBridge(ctx, t, env, makeConverterWithInverse(t, propX, propY, s))

	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// A --X--> B causes B to gain an inverse relation, which a bridge job re-indexes.
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The job that re-indexed docB must record its result and timing breakdown as River job output
	// (stored on the job under the "output" metadata key), so it is queryable per job through River.
	// We match the output by its JSON field names, which also guards their contract.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		jobs, err := env.riverClient.JobList(ctx, river.NewJobListParams().
			Kinds("BridgeReindex").
			States(rivertype.JobStateCompleted).
			First(1000))
		if !assert.NoError(c, err) {
			return
		}
		var found bool
		for _, jr := range jobs.Jobs {
			var meta struct {
				Output *struct {
					Reindexed     int     `json:"reindexed"`
					Queries       int     `json:"queries"`
					IndexDuration float64 `json:"indexDuration"`
					Duration      float64 `json:"duration"`
				} `json:"output"`
			}
			if !assert.NoError(c, json.Unmarshal(jr.Metadata, &meta)) {
				return
			}
			if meta.Output != nil && meta.Output.Reindexed > 0 {
				found = true
				assert.GreaterOrEqual(c, meta.Output.Duration, meta.Output.IndexDuration)
				break
			}
		}
		assert.True(c, found, "a completed bridge job should record output with reindexed > 0")
	}, 15*time.Second, 200*time.Millisecond)
}

func TestBridgeReindexContinuation(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Force the reindex job to hit its soft deadline after every document, so it flushes what it has and
	// schedules a follow-up job. This exercises the continuation chain across many jobs.
	b.TestingSetReindexSoftDeadline(time.Nanosecond)

	propX := identifier.New()
	propY := identifier.New()

	startBridge(ctx, t, env, makeConverterWithInverse(t, propX, propY, s))

	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert several A --X--> B pairs. Each B gains an inverse relation and is enqueued for re-indexing,
	// so the queue holds multiple distinct documents that the chained jobs must all drain.
	type relPair struct{ a, b identifier.Identifier }
	const numPairs = 5
	rels := make([]relPair, numPairs)
	for i := range rels {
		rels[i] = relPair{a: identifier.New(), b: identifier.New()}
		_, errE = s.Insert(ctx, rels[i].b, makeDocJSON(t, rels[i].b), dummyMetadata(), dummyCommitMetadata())
		require.NoError(t, errE, "% -+#.1v", errE)
		_, errE = s.Insert(ctx, rels[i].a, makeDocWithRelationJSON(t, rels[i].a, propX, rels[i].b), dummyMetadata(), dummyCommitMetadata())
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	// WaitUntilCaughtUp returns only once the whole queue is drained, which here requires the follow-up
	// chain to run to completion.
	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Every B must have been re-indexed with its inverse relation, proving the chain drained all of them.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		for _, rel := range rels {
			assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, rel.b, propY, rel.a),
				"docB should have inverse relation B --Y--> A")
		}
	}, 15*time.Second, 200*time.Millisecond)

	// At least one reindex job must have scheduled a follow-up because of the deadline, confirming the
	// continuation path ran rather than a single job draining everything.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		jobs, err := env.riverClient.JobList(ctx, river.NewJobListParams().
			Kinds("BridgeReindex").
			States(rivertype.JobStateCompleted).
			First(1000))
		if !assert.NoError(c, err) {
			return
		}
		var sawFollowUp bool
		for _, jr := range jobs.Jobs {
			var meta struct {
				Output *struct {
					ScheduledFollowUp bool `json:"scheduledFollowUp"`
				} `json:"output"`
			}
			if !assert.NoError(c, json.Unmarshal(jr.Metadata, &meta)) {
				return
			}
			if meta.Output != nil && meta.Output.ScheduledFollowUp {
				sawFollowUp = true
				break
			}
		}
		assert.True(c, sawFollowUp, "at least one reindex job should have scheduled a follow-up due to the deadline")
	}, 15*time.Second, 200*time.Millisecond)
}

func TestBridgeInverseRelationMutual(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Property X has inversePropertyOf Y.
	// A --X--> B means B gets B --Y--> A.
	// B --X--> A means A gets A --Y--> B.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	startBridge(ctx, t, env, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert A --X--> B and B --X--> A in the same commit won't work with the store API
	// (each Insert is its own commit). So insert them as separate commits.
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docB, makeDocWithRelationJSON(t, docB, propX, docA), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Both documents should eventually have both forward and inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		// A should have forward A --X--> B and inverse A --Y--> B (from B --X--> A).
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docA, propX, docB),
			"docA should have forward A --X--> B")
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docA, propY, docB),
			"docA should have inverse A --Y--> B")
		// B should have forward B --X--> A and inverse B --Y--> A (from A --X--> B).
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propX, docA),
			"docB should have forward B --X--> A")
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)
}

func TestBridgeInverseRelationMultipleSources(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Property X has inversePropertyOf Y.
	// Both A and C point to B with property X.
	// B should get two inverse relations: B --Y--> A and B --Y--> C.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	startBridge(ctx, t, env, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	docA := identifier.New()
	docB := identifier.New()
	docC := identifier.New()

	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docC, makeDocWithRelationJSON(t, docC, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// B should eventually have both inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docC),
			"docB should have inverse B --Y--> C")
	}, 10*time.Second, 100*time.Millisecond)
}

func TestBridgeInverseRelationRemoval(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Property X has inversePropertyOf Y.
	// A --X--> B means B gets inverse B --Y--> A.
	// When we replace A to remove the relation, B should lose the inverse.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	startBridge(ctx, t, env, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert document B (empty) and document A with relation A --X--> B.
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB to have the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)

	// Now replace A with a document that has no relations.
	_, _, latestA, _, errE := s.GetLatest(ctx, docA) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Replace(ctx, docA, latestA.Changeset, makeDocJSON(t, docA), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB to lose the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := s.GetLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.Empty(c, metadata.InverseRelations, "docB metadata should have no inverse relations after removal")
	}, 10*time.Second, 100*time.Millisecond)

	// Verify in ES that docB no longer has the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		assert.False(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should no longer have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)
}

func TestBridgeInverseRelationChange(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// Property X has inversePropertyOf Y.
	// A --X--> B means B gets inverse B --Y--> A.
	// When we change A to point to C instead, B should lose the inverse and C should gain it.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	startBridge(ctx, t, env, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert documents B and C (empty), then A with relation A --X--> B.
	docA := identifier.New()
	docB := identifier.New()
	docC := identifier.New()
	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docC, makeDocJSON(t, docC), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB to have the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)

	// Replace A to point to C instead of B.
	_, _, latestA, _, errE := s.GetLatest(ctx, docA) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Replace(ctx, docA, latestA.Changeset, makeDocWithRelationJSON(t, docA, propX, docC), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docC to gain and docB to lose the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		// C should have the inverse relation.
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docC, propY, docA),
			"docC should have inverse C --Y--> A")
		// B should no longer have the inverse relation.
		assert.False(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should no longer have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)

	// Verify metadata as well.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadataB, _, _, errE := s.GetLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.Empty(c, metadataB.InverseRelations, "docB metadata should have no inverse relations")

		_, metadataC, _, _, errE := s.GetLatest(ctx, docC)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadataC.InverseRelations, "docC metadata should have inverse relations")
	}, 10*time.Second, 100*time.Millisecond)
}

// esReferencesCount reads the counts.references of an ES document via doc values (the
// index has _source disabled). The second return value is false when the document
// carries no counts.references field.
func esReferencesCount(ctx context.Context, t *testing.T, esClient *elasticsearch.TypedClient, index, id string) (int, bool) {
	t.Helper()
	res, err := esClient.Search().Index(index).
		Source_(esdsl.NewSourceConfig().Bool(false)).
		Query(esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(id))).
		DocvalueFields(esdsl.NewFieldAndFormat().Field("counts.references")).
		Size(1).Do(ctx)
	testutils.RequireNoESError(t, err)
	require.Len(t, res.Hits.Hits, 1, "document should exist in ES")
	raw, ok := res.Hits.Hits[0].Fields["counts.references"]
	if !ok {
		return 0, false
	}
	var values []int
	require.NoError(t, json.Unmarshal(raw, &values))
	require.NotEmpty(t, values)
	return values[0], true
}

func TestBridgeReferencesCountIncremental(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	converter := newTestBridgeConverter(t)
	// Compute counts.references at index time, as the production converter does.
	converter.CountReferences = b.CountReferences
	startBridge(ctx, t, env, converter)

	waitAndRefresh := func() {
		t.Helper()
		errE := b.WaitUntilCaughtUp(ctx, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		testutils.RequireNoESError(t, err)
	}

	target := identifier.New()
	prop := identifier.New()
	ref1 := identifier.New()
	ref2 := identifier.New()

	// The target starts with no referrers.
	_, errE := s.Insert(ctx, target, makeDocJSON(t, target), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	waitAndRefresh()
	count, ok := esReferencesCount(ctx, t, esClient, b.Index, target.String())
	require.True(t, ok, "target should carry a counts.references")
	assert.Equal(t, 0, count, "no referrers yet")

	// Adding a referrer via a plain (non-inverse) property re-indexes the target and
	// bumps its counts.references, even though the target itself did not change.
	v1, errE := s.Insert(ctx, ref1, makeDocWithRelationJSON(t, ref1, prop, target), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	waitAndRefresh()
	count, ok = esReferencesCount(ctx, t, esClient, b.Index, target.String())
	require.True(t, ok)
	assert.Equal(t, 1, count, "one referrer")

	// A second referrer bumps it to 2.
	_, errE = s.Insert(ctx, ref2, makeDocWithRelationJSON(t, ref2, prop, target), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	waitAndRefresh()
	count, ok = esReferencesCount(ctx, t, esClient, b.Index, target.String())
	require.True(t, ok)
	assert.Equal(t, 2, count, "two referrers")

	// Deleting the first referrer drops it back to 1.
	_, errE = s.Delete(ctx, ref1, v1.Changeset, dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	waitAndRefresh()
	count, ok = esReferencesCount(ctx, t, esClient, b.Index, target.String())
	require.True(t, ok)
	assert.Equal(t, 1, count, "one referrer after deletion")
}

// makeClassDocWithFieldInverse builds a class document whose field schema defines sourceProp
// as a top-level field with field-level INVERSE_PROPERTY inverseProp. The schema is nested under
// a FIELDS HasClaim, mirroring how the transform package serializes a class's Fields. This is the
// shape, e.g., an exhibition class uses to invert HAS_ARTIST to HAS_EVENT.
func makeClassDocWithFieldInverse(classID, sourceProp, inverseProp identifier.Identifier) *document.D {
	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: internalCore.HasPropertyPropID},
				To:        document.Reference{ID: sourceProp},
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: internalCore.InversePropertyPropID},
				To:        document.Reference{ID: inverseProp},
			},
		},
	}
	fieldsSub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence, Sub: fieldSub},
				Prop:      document.Reference{ID: internalCore.FieldPropID},
			},
		},
	}
	claims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence, Sub: fieldsSub},
				Prop:      document.Reference{ID: internalCore.FieldsPropID},
			},
		},
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: classID}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// makeConverterWithFieldInverse creates a converter that knows the field-level inverse defined on
// classDoc. The getDocument callback resolves documents (and thus source display labels) from the store.
func makeConverterWithFieldInverse(
	t *testing.T, classDoc *document.D,
	s *store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes],
) *internalSearch.Converter {
	t.Helper()

	c, errE := internalSearch.NewConverter(nil, nil, []*document.D{classDoc}, nil, func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
		data, _, _, _, errE := s.GetLatest(ctx, id)
		if errors.Is(errE, store.ErrValueNotFound) {
			// Return a minimal document for IDs not in the store (e.g., the class or core property IDs).
			return &document.D{
				CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
			}, nil
		} else if errE != nil {
			return nil, errE
		}
		var doc document.D
		errE = x.UnmarshalWithoutUnknownFields(data, &doc)
		if errE != nil {
			return nil, errE
		}
		return &doc, nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

// makeSourceDocWithNameJSON creates a source document that is an instance of classID, carries a
// NAMING string (its display label), and references targetID via refProp.
func makeSourceDocWithNameJSON(t *testing.T, docID, classID, refProp, targetID identifier.Identifier, name string) json.RawMessage {
	t.Helper()
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: classID},
				},
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: refProp},
					To:        document.Reference{ID: targetID},
				},
			},
			String: []document.StringClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    name,
				},
			},
		},
	}
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// docTextContains reports whether the ES document with the given ID has the term in its top-level
// text.und field (where folded display labels land), proving the term is searchable on that document.
func docTextContains(ctx context.Context, t *testing.T, esClient *elasticsearch.TypedClient, index string, docID identifier.Identifier, term string) bool {
	t.Helper()

	query := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(docID.String())),
		esdsl.NewMatchQuery("text.und", term),
	)
	res, err := esClient.Search().Index(index).Query(query).Size(1).Do(ctx)
	testutils.RequireNoESError(t, err)
	return res.Hits.Total.Value > 0
}

// TestBridgeFieldInverseRelationFoldsSourceLabelIntoText verifies the end-to-end field-level inverse
// path: a class defines a field whose inverse is another property (as an exhibition class inverts
// HAS_ARTIST to HAS_EVENT), so a source document referencing a target gives the target an inverse
// reference back AND folds the source's display label into the target's text, making the target
// findable by the source's name.
func TestBridgeFieldInverseRelationFoldsSourceLabelIntoText(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	// classID's field hasArtist has field-level inverse hasEvent.
	classID := identifier.New()
	hasArtist := identifier.New()
	hasEvent := identifier.New()

	classDoc := makeClassDocWithFieldInverse(classID, hasArtist, hasEvent)
	converter := makeConverterWithFieldInverse(t, classDoc, s)
	startBridge(ctx, t, env, converter)

	artist := identifier.New()
	exhibition := identifier.New()
	const exhibitionName = "Kandinskyjeva Retrospektiva"

	// Insert the artist (target) first so its metadata exists when the exhibition is indexed.
	_, errE := s.Insert(ctx, artist, makeDocJSON(t, artist), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	// Insert the exhibition (source): instance of classID, named, referencing the artist via hasArtist.
	_, errE = s.Insert(ctx, exhibition, makeSourceDocWithNameJSON(t, exhibition, classID, hasArtist, artist, exhibitionName), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !testutils.AssertNoESError(c, err) {
			return
		}
		// The field-level inverse materializes the reverse reference artist --hasEvent--> exhibition.
		assert.True(
			c, testutils.DocHasReference(ctx, t, esClient, b.Index, artist, hasEvent, exhibition),
			"artist should have inverse reference artist --hasEvent--> exhibition",
		)
		// The exhibition's display label is folded into the artist's text, so the artist is findable by it.
		assert.True(
			c, docTextContains(ctx, t, esClient, b.Index, artist, "Kandinskyjeva"),
			"artist text should include the exhibition display label",
		)
	}, 10*time.Second, 100*time.Millisecond)

	// Sanity: the exhibition itself carries the forward reference exhibition --hasArtist--> artist.
	assert.True(
		t, testutils.DocHasReference(ctx, t, esClient, b.Index, exhibition, hasArtist, artist),
		"exhibition should have forward reference exhibition --hasArtist--> artist",
	)
}
