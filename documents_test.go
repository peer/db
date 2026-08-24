package peerdb_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalBase "gitlab.com/peerdb/peerdb/internal/base"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

// classDoc returns a minimal document which is an instance of the given class.
func classDoc(classID identifier.Identifier) *document.D {
	docBase := []string{"test", identifier.New().String()}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(docBase...), Base: docBase},
		Claims: &document.ClaimTypes{
			Reference: document.ReferenceClaims{{
				CoreClaim: document.CoreClaim{
					ID:         identifier.New(),
					Confidence: document.HighConfidence,
					Sub:        nil,
				},
				Prop: document.Reference{ID: internalCore.InstanceOfPropID},
				To:   document.Reference{ID: classID},
			}},
		},
	}
}

// initBaseForDocuments creates a started base with the core documents populated, for the tests of
// loading documents out of the store.
func initBaseForDocuments(t *testing.T) (context.Context, *base.B) {
	t.Helper()

	ctx, b, _, _ := initBaseInfra(t)

	_, transformed, errE := base.GenerateCoreDocuments(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	onShutdown, errE := b.PopulateAndStart(ctx, transformed, nil, nil, nil, nil)
	if onShutdown != nil {
		t.Cleanup(onShutdown)
	}
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, b
}

// initBaseInfra initializes PostgreSQL, ElasticSearch and River for a base, without populating it. It
// returns the base together with what a test needs to reach around it: the ES client, the connection
// pool, and the index the base indexes into.
func initBaseInfra(t *testing.T) (context.Context, *base.B, *elasticsearch.TypedClient, *pgxpool.Pool) {
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
		errE := internalSearch.DeleteIndex(context.Background(), esClient, internalSearch.LevelIndex(index, internalSite.AllVisibilityLevel))
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	b, _, errE := internalBase.InitComponents(ctx, logger, nil, dbpool, esClient, schema, index, 1, t.TempDir(), nil, nil, []string{internalSite.AllVisibilityLevel})
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, b, esClient, dbpool
}

// TestFetchDocumentsSkipsDeleted tests that a document which is listed but is not there anymore when it is
// read is skipped rather than failing the whole listing. It is the state a document deleted between the
// listing and the read is in, the two being separate transactions.
func TestFetchDocumentsSkipsDeleted(t *testing.T) {
	t.Parallel()

	ctx, b := initBaseForDocuments(t)

	classID := identifier.New()

	// Two documents of the same class, one of which is then deleted.
	kept := classDoc(classID)
	deleted := classDoc(classID)
	for _, doc := range []*document.D{kept, deleted} {
		errE := b.InsertDocument(ctx, doc)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	documents, errE := peerdb.TestingFetchDocuments(ctx, b, classID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, documents, 2)

	// Delete the document while keeping the classes on the deleted version, which is the state a document
	// deleted after it was listed is read in: it is listed as a document of the class, but reading it says
	// it is gone. DeleteDocument records no classes, so the delete is made through the store to get there.
	_, _, version, _, errE := b.Documents().GetLatest(ctx, deleted.ID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	changesetBase := append(slices.Clone(deleted.Base), "CHANGESET", identifier.New().String())
	_, errE = b.Documents().Delete(ctx, deleted.ID, version.Changeset, &store.DocumentMetadata{
		At:         store.Time(time.Now().UTC()),
		Users:      nil,
		InstanceOf: []identifier.Identifier{classID},
	}, &store.CommitMetadata{Base: changesetBase, User: nil})
	require.NoError(t, errE, "% -+#.1v", errE)

	// The deleted document is still listed as a document of the class.
	ids, errE := b.Documents().List(ctx, json.RawMessage(`{"instanceOf":["`+classID.String()+`"]}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, ids, deleted.ID)

	// Fetching the documents of the class skips it instead of failing.
	documents, errE = peerdb.TestingFetchDocuments(ctx, b, classID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, documents, 1)
	assert.Equal(t, kept.ID, documents[0].Document.ID)
}

// distinctFromDoc returns a document with the given base whose only claim states it is distinct from
// the target document. DISTINCT_FROM is its own inverse, so the target's entry renders a synthetic
// inverse claim back, which is what the test observes.
func distinctFromDoc(docBase []string, target identifier.Identifier) *document.D {
	return &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(docBase...), Base: docBase},
		Claims: &document.ClaimTypes{
			Reference: document.ReferenceClaims{{
				CoreClaim: document.CoreClaim{
					ID:         identifier.New(),
					Confidence: document.HighConfidence,
					Sub:        nil,
				},
				Prop: document.Reference{ID: internalCore.DistinctFromPropID},
				To:   document.Reference{ID: target},
			}},
		},
	}
}

// TestRecreateIndex tests the flow of db reindex --recreate-index: after a populated base is stopped and
// its index deleted, a fresh base loads the schema documents from PostgreSQL alone, cancels the reindex
// queue a previous run left behind, clears the bridge-maintained metadata and resets the bridge, and only
// then starts, so that the replay fills the recreated index with a working converter.
//
// The bridge is left behind on a commit log whose first commits are the documents the assertions are about,
// which is the state a previous run interrupted mid-replay leaves. Only a replay from the beginning covers
// those commits, so the test fails if the reset does not take effect: a base started before the reset keeps
// replaying from where it was left and raises the bridge seq past it again.
func TestRecreateIndex(t *testing.T) {
	t.Parallel()

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
	index := schema
	topIndex := internalSearch.LevelIndex(index, internalSite.AllVisibilityLevel)
	levels := []string{internalSite.AllVisibilityLevel}

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
		errE := internalSearch.DeleteIndex(context.Background(), esClient, topIndex)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	// The first base populates the store: the core documents and a pair of documents with a
	// DISTINCT_FROM claim between them.
	ctx1, cancel1 := context.WithCancel(ctx)
	b1, _, errE := internalBase.InitComponents(ctx1, logger, nil, dbpool, esClient, schema, index, 1, t.TempDir(), nil, nil, levels)
	require.NoError(t, errE, "% -+#.1v", errE)

	baseA := []string{"test", "recreate", "A"}
	baseB := []string{"test", "recreate", "B"}
	docB := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(baseB...), Base: baseB},
	}
	docA := distinctFromDoc(baseA, docB.ID)

	_, transformed, errE := base.GenerateCoreDocuments(ctx1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	// The pair goes first, so that its commits are at the beginning of the commit log.
	transformed = append([]*document.D{docA, docB}, transformed...)

	onShutdown1, errE := b1.PopulateAndStart(ctx1, transformed, nil, nil, nil, nil)
	if onShutdown1 != nil {
		t.Cleanup(onShutdown1)
	}
	require.NoError(t, errE, "% -+#.1v", errE)

	// The populated index renders the synthetic inverse claim onto the target document.
	assert.True(t, testutils.DocHasReference(ctx, t, esClient, topIndex, docB.ID, internalCore.DistinctFromPropID, docA.ID))

	// Stop the first base, so that its bridge and workers do not race the second one.
	cancel1()
	onShutdown1()

	// Seed a leftover reindex queue entry, the state an interrupted reindex leaves behind.
	_, err := dbpool.Exec(ctx, `INSERT INTO "docsBridgeReindexQueue" ("id", "seq") VALUES ($1, $2)`, identifier.New().String(), int64(1))
	require.NoError(t, err)

	// Leave the bridge in the middle of the commit log, the state a run interrupted mid-replay leaves behind.
	// The pair's commits are below it, so they are indexed only by a replay which starts from the beginning.
	var maxSeq int64
	err = dbpool.QueryRow(ctx, `SELECT MAX("seq") FROM "docsCommitLog"`).Scan(&maxSeq)
	require.NoError(t, err)
	require.Positive(t, maxSeq)
	_, err = dbpool.Exec(ctx, `UPDATE "docsBridge" SET "seq" = $1`, maxSeq/2)
	require.NoError(t, err)

	// The flow of db reindex --recreate-index: the index is deleted first, so everything below runs
	// against a store whose search index is empty.
	errE = internalSearch.DeleteIndex(ctx, esClient, topIndex)
	require.NoError(t, errE, "% -+#.1v", errE)

	ctx2, cancel2 := context.WithCancel(ctx)
	t.Cleanup(cancel2)
	b2, _, errE := internalBase.InitComponents(ctx2, logger, nil, dbpool, esClient, schema, index, 1, t.TempDir(), nil, nil, levels)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The schema documents are loaded from PostgreSQL, so they are complete although the index is empty.
	converterDocs, errE := peerdb.TestingConverterDocuments(ctx, b2)
	require.NoError(t, errE, "% -+#.1v", errE)
	kinds := map[identifier.Identifier]int{}
	for _, sd := range converterDocs {
		for _, classID := range sd.Document.InstanceOf() {
			kinds[classID]++
		}
	}
	assert.Positive(t, kinds[internalCore.PropertyClassID])
	assert.Positive(t, kinds[internalCore.LanguageClassID])
	assert.Positive(t, kinds[internalCore.ClassClassID])

	errE = b2.CancelReindexQueue(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	var queued int64
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM "docsBridgeReindexQueue"`).Scan(&queued)
	require.NoError(t, err)
	assert.Zero(t, queued)

	errE = b2.ClearSystemManagedMetadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = b2.ResetBridgeProgress(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The base is started only once the bridge is back at the beginning of the commit log and the tables the
	// replay rebuilds are empty, which is what the replay the start begins depends on.
	var seq, inverseRelations int64
	err = dbpool.QueryRow(ctx, `SELECT "seq" FROM "docsBridge"`).Scan(&seq)
	require.NoError(t, err)
	assert.Zero(t, seq)
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM "docsInverseRelations"`).Scan(&inverseRelations)
	require.NoError(t, err)
	assert.Zero(t, inverseRelations)

	onShutdown2, errE := b2.Start(ctx2, converterDocs)
	if onShutdown2 != nil {
		t.Cleanup(onShutdown2)
	}
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b2.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Indices.Refresh().Index(topIndex).Do(ctx)
	testutils.RequireNoESError(ctx, t, err)

	// The replay ran with a working converter: the documents are back and the target document again
	// renders the synthetic inverse claim, which requires both the schema in the converter and the
	// rebuilt inverse relations.
	assert.True(t, testutils.DocExists(ctx, t, esClient, topIndex, docA.ID.String()))
	assert.True(t, testutils.DocExists(ctx, t, esClient, topIndex, docB.ID.String()))
	assert.True(t, testutils.DocHasReference(ctx, t, esClient, topIndex, docB.ID, internalCore.DistinctFromPropID, docA.ID))

	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM "docsInverseRelations"`).Scan(&inverseRelations)
	require.NoError(t, err)
	assert.Positive(t, inverseRelations)
}
