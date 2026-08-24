package peerdb_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

	ctx, b, _ := initBaseInfra(t)

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
// returns the base together with the connection pool, for tests to reach around it.
func initBaseInfra(t *testing.T) (context.Context, *base.B, *pgxpool.Pool) {
	t.Helper()

	infra := testutils.NewPostgresAndElastic(t)

	testutils.DeleteIndexOnCleanup(t, infra.ESClient, internalSearch.LevelIndex(infra.Name, internalSite.AllVisibilityLevel))

	b, _, errE := internalBase.InitComponents(
		infra.Ctx, infra.Logger, nil, infra.DBPool, infra.ESClient, infra.Name, infra.Name, 1, t.TempDir(), nil, nil, []string{internalSite.AllVisibilityLevel},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	return infra.Ctx, b, infra.DBPool
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
// its index deleted, a fresh base loads the schema documents from PostgreSQL alone, enqueues every
// document for re-indexing, and starts, so that the drain fills the recreated index by rendering each
// document at its latest version from the store and the materialized state, which are trusted and
// persist across the recreation.
func TestRecreateIndex(t *testing.T) {
	t.Parallel()

	infra := testutils.NewPostgresAndElastic(t)
	ctx, logger, dbpool, esClient := infra.Ctx, infra.Logger, infra.DBPool, infra.ESClient

	topIndex := internalSearch.LevelIndex(infra.Name, internalSite.AllVisibilityLevel)
	levels := []string{internalSite.AllVisibilityLevel}

	testutils.DeleteIndexOnCleanup(t, esClient, topIndex)

	// The first base populates the store: the core documents and a pair of documents with a
	// DISTINCT_FROM claim between them.
	ctx1, cancel1 := context.WithCancel(ctx)
	b1, _, errE := internalBase.InitComponents(ctx1, logger, nil, dbpool, esClient, infra.Name, infra.Name, 1, t.TempDir(), nil, nil, levels)
	require.NoError(t, errE, "% -+#.1v", errE)

	baseA := []string{"test", "recreate", "A"}
	baseB := []string{"test", "recreate", "B"}
	docB := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(baseB...), Base: baseB},
	}
	docA := distinctFromDoc(baseA, docB.ID)

	_, transformed, errE := base.GenerateCoreDocuments(ctx1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	transformed = append(transformed, docA, docB)

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

	// Seed a leftover reindex queue entry, the state an interrupted run leaves behind. It is drained
	// together with the enqueued entries, rendering its document from the trusted materialized state.
	_, err := dbpool.Exec(ctx, `INSERT INTO "docsBridgeReindexQueue" ("id", "seq") VALUES ($1, $2)`, docA.ID.String(), int64(1))
	require.NoError(t, err)

	// The flow of db reindex --recreate-index: the index is deleted first, so everything below runs
	// against a store whose search index is empty.
	errE = internalSearch.DeleteIndex(ctx, esClient, topIndex)
	require.NoError(t, errE, "% -+#.1v", errE)

	ctx2, cancel2 := context.WithCancel(ctx)
	t.Cleanup(cancel2)
	b2, _, errE := internalBase.InitComponents(ctx2, logger, nil, dbpool, esClient, infra.Name, infra.Name, 1, t.TempDir(), nil, nil, levels)
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

	// The reindex flow: enqueue every document before the base is started, so that the drain job the
	// start submits already finds the entries.
	enqueued, errE := b2.EnqueueAllForReindex(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Positive(t, enqueued)

	onShutdown2, errE := b2.Start(ctx2, converterDocs)
	if onShutdown2 != nil {
		t.Cleanup(onShutdown2)
	}
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b2.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Indices.Refresh().Index(topIndex).Do(ctx)
	testutils.RequireNoESError(ctx, t, err)

	// The drain rendered every document with a working converter: the documents are back and the target
	// document again renders the synthetic inverse claim, which requires both the schema in the converter
	// and the persisted inverse relations.
	assert.True(t, testutils.DocExists(ctx, t, esClient, topIndex, docA.ID.String()))
	assert.True(t, testutils.DocExists(ctx, t, esClient, topIndex, docB.ID.String()))
	assert.True(t, testutils.DocHasReference(ctx, t, esClient, topIndex, docB.ID, internalCore.DistinctFromPropID, docA.ID))
}
