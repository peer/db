package base_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	internalBase "gitlab.com/peerdb/peerdb/internal/base"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// initBaseInfra initializes the base infrastructure (PostgreSQL, Elasticsearch, River)
// without populating core documents. Callers must call populateBase or b.PopulateAndStart
// to insert documents and start the base.
func initBaseInfra(t *testing.T, languagePriority map[string][]string) (context.Context, *base.B, *elastic.Client) {
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

	// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
	// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
	dbpool, errE := internalStore.InitPostgres(context.WithoutCancel(ctx), os.Getenv("POSTGRES"), logger, func(_ context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpool.Close)

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		require.NoError(t, err)
	})

	b, _, onShutdown, errE := internalBase.InitAndStartComponents(ctx, logger, dbpool, esClient, schema, index, languagePriority)
	t.Cleanup(onShutdown)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, b, esClient
}

// populateBase generates core documents, transforms them, inserts them into the store,
// starts the base, and waits for Elasticsearch to catch up.
// Additional already-transformed documents can be appended to the population.
func populateBase(ctx context.Context, t *testing.T, b *base.B, additionalDocs []*document.D) {
	t.Helper()

	_, transformed, errE := base.GenerateCoreDocuments(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	transformed = append(transformed, additionalDocs...)

	errE = b.PopulateAndStart(ctx, transformed, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func initBase(t *testing.T) (context.Context, *base.B) {
	t.Helper()

	ctx, b, _ := initBaseWithES(t)
	return ctx, b
}

func initBaseWithES(t *testing.T) (context.Context, *base.B, *elastic.Client) {
	t.Helper()

	ctx, b, esClient := initBaseInfra(t, nil)
	populateBase(ctx, t, b, nil)
	return ctx, b, esClient
}

// newDocID creates a new document ID with a valid Base.
func newDocID() (identifier.Identifier, []string) {
	base := []string{"test", identifier.New().String()}
	return identifier.From(base...), base
}

// newDoc creates a new empty document with a valid ID and Base.
func newDoc() *document.D {
	id, base := newDocID()
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id, Base: base},
	}
}

// Well-known IDs for inverse relation tests.
//
//nolint:gochecknoglobals
var (
	testInstanceOfPropID        = identifier.From(core.Namespace, "INSTANCE_OF")
	testPropertyClassID         = identifier.From(core.Namespace, "PROPERTY")
	testInversePropertyOfPropID = identifier.From(core.Namespace, "INVERSE_PROPERTY_OF")
)

func makePropertyDoc(t *testing.T, id identifier.Identifier, base []string, inverseOf *identifier.Identifier) *document.D {
	t.Helper()
	claims := &document.ClaimTypes{}
	claims.Relation = append(claims.Relation, document.RelationClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: testInstanceOfPropID},
		To:        document.Reference{ID: testPropertyClassID},
	})
	if inverseOf != nil {
		claims.Relation = append(claims.Relation, document.RelationClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: testInversePropertyOfPropID},
			To:        document.Reference{ID: *inverseOf},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id, Base: base},
		Claims:       claims,
	}
}

func TestInsertOrReplaceDocument(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	doc := newDoc()
	docID := doc.ID

	// Insert.
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify it exists.
	_, _, version1, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	// Replace with modified document.
	doc.Claims = &document.ClaimTypes{
		String: []document.StringClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: identifier.New()},
				String:    "updated",
			},
		},
	}

	errE = b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the document was replaced (new version).
	_, _, version2, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEqual(t, version1, version2)
}

func TestInsertOrReplaceDocumentCarriesOverMetadata(t *testing.T) {
	t.Parallel()

	// Set up inverse properties: propX has inversePropertyOf propY.
	propX, propXBase := newDocID()
	propY, propYBase := newDocID()
	propXDoc := makePropertyDoc(t, propX, propXBase, &propY)
	propYDoc := makePropertyDoc(t, propY, propYBase, nil)

	ctx, b, _ := initBaseInfra(t, nil)
	populateBase(ctx, t, b, []*document.D{propXDoc, propYDoc})

	// Insert docB (target) and docA with relation A --X--> B.
	docADoc := newDoc()
	docBDoc := newDoc()
	docB := docBDoc.ID

	errE := b.InsertOrReplaceDocument(ctx, docBDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	docADoc.Claims = &document.ClaimTypes{
		Relation: []document.RelationClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB's metadata to have inverse relations from the bridge.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB should have inverse relations")
	}, 30*time.Second, 100*time.Millisecond)

	// Now replace docB. Inverse relations should be carried over.
	errE = b.InsertOrReplaceDocument(ctx, &document.D{
		CoreDocument: docBDoc.CoreDocument,
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: identifier.New()},
					String:    "replaced",
				},
			},
		},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify inverse relations were carried over after replace.
	_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, metadata.InverseRelations, "docB should still have inverse relations after replace")
}

func TestInsertOrReplaceFile(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	fileID := identifier.New()
	data := []byte("hello world")

	// Insert.
	errE := b.InsertOrReplaceFile(ctx, fileID, data, "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify it exists.
	fileData, metadata, errE := b.GetFile(ctx, fileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, data, fileData)
	assert.Equal(t, "test.txt", metadata.Filename)

	// Replace with new content.
	newData := []byte("updated content")
	errE = b.InsertOrReplaceFile(ctx, fileID, newData, "test2.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify it was replaced.
	fileData, metadata, errE = b.GetFile(ctx, fileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newData, fileData)
	assert.Equal(t, "test2.txt", metadata.Filename)
}

// marshalChange marshals a single document change to JSON for use with AppendDocumentChange.
func marshalChange(t *testing.T, change document.Change) []byte {
	t.Helper()
	data, errE := document.ChangeMarshalJSON(change)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// docExists checks if a document exists in the Elasticsearch index.
func docExists(ctx context.Context, t *testing.T, esClient *elastic.Client, index, id string) bool {
	t.Helper()
	resp, err := esClient.Get().Index(index).Id(id).Do(ctx)
	if err != nil {
		if elastic.IsNotFound(err) {
			return false
		}
		t.Fatalf("unexpected ES error: %v", err)
	}
	return resp.Found
}

// docHasRelation checks if an ES document has a nested relation claim with the given prop and target.
func docHasRelation(ctx context.Context, t *testing.T, esClient *elastic.Client, index string, docID, propID, targetID identifier.Identifier) bool {
	t.Helper()
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.rel.prop", propID.String()),
		elastic.NewTermQuery("claims.rel.to", targetID.String()),
	)
	query := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("id", docID.String()),
		elastic.NewNestedQuery("claims.rel", boolQuery),
	)
	res, err := esClient.Search().Index(index).Query(query).Size(1).Do(ctx)
	if err != nil {
		t.Fatalf("ES search error: %v", err)
	}
	return res.Hits.TotalHits.Value > 0
}

func TestDocumentEditSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session.
	session, version, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify session is active.
	beginMetadata, errE := b.GetDocumentEditSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, docID, beginMetadata.Document)

	// Create a change that adds a string claim.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := []string{docID.String(), "SESSION", session.String(), "1"}
	claimID := identifier.From(changeBase...)

	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "test value",
		},
	}

	changeJSON := marshalChange(t, addChange)
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// List changes.
	changes, errE := b.ListDocumentChanges(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{1}, changes)

	// Get change data.
	changeData, errE := b.GetDocumentChange(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, changeData)

	// End session (commit).
	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Session should be ended now.
	_, errE = b.GetDocumentEditSession(ctx, session)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	// Wait for async completion to update the document.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version, newVersion, "document version should change after session completes")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify the document has the new claim.
	updatedDoc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "test value", updatedDoc.Claims.String[0].String)
}

func TestDocumentEditSessionDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session.
	session, version, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a change.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := []string{docID.String(), "SESSION", session.String(), "1"}
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "discarded value",
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Discard the session.
	errE = b.EndDocumentEdit(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion to run, then verify document is unchanged.
	time.Sleep(500 * time.Millisecond)

	// TODO: We should record that job completed in session metadata and check it here.

	_, _, versionAfter, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, versionAfter, "document version should not change after discarded session")

	updatedDoc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, updatedDoc.Claims, "document should have no claims after discarded session")
}

func TestDocumentEditSessionCarriesOverMetadata(t *testing.T) {
	t.Parallel()

	// Set up inverse properties: propX has inversePropertyOf propY.
	propX, propXBase := newDocID()
	propY, propYBase := newDocID()
	propXDoc := makePropertyDoc(t, propX, propXBase, &propY)
	propYDoc := makePropertyDoc(t, propY, propYBase, nil)

	ctx, b, _ := initBaseInfra(t, nil)
	populateBase(ctx, t, b, []*document.D{propXDoc, propYDoc})

	// Insert docB (target) and docA with relation A --X--> B.
	docADoc := newDoc()
	docBDoc := newDoc()
	docB := docBDoc.ID

	errE := b.InsertOrReplaceDocument(ctx, docBDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	docADoc.Claims = &document.ClaimTypes{
		Relation: []document.RelationClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB's metadata to have inverse relations from the bridge.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB should have inverse relations")
	}, 30*time.Second, 100*time.Millisecond)

	// Begin edit session for docB.
	session, versionB, errE := b.BeginDocumentEdit(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a string claim to docB through the edit session.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := []string{docB.String(), "SESSION", session.String(), "1"}
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "edited via session",
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session (commit).
	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion to update the document.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		// The version should change because the session added a claim.
		assert.NotEqual(c, versionB.Changeset, newVersion.Changeset, "document changeset should change after session completes")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify the document has the new claim AND inverse relations were carried over.
	updatedDoc, metadata, _, _, errE := b.GetDocumentLatestDoc(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "edited via session", updatedDoc.Claims.String[0].String)
	assert.NotEmpty(t, metadata.InverseRelations, "inverse relations should be carried over after edit session")
}

func TestDocumentEditSessionMetadataChangedDuringEdit(t *testing.T) {
	t.Parallel()

	// Set up inverse properties: propX has inversePropertyOf propY.
	propX, propXBase := newDocID()
	propY, propYBase := newDocID()
	propXDoc := makePropertyDoc(t, propX, propXBase, &propY)
	propYDoc := makePropertyDoc(t, propY, propYBase, nil)

	ctx, b, _ := initBaseInfra(t, nil)
	populateBase(ctx, t, b, []*document.D{propXDoc, propYDoc})

	// Insert target document docB.
	docBDoc := newDoc()
	docB := docBDoc.ID
	errE := b.InsertOrReplaceDocument(ctx, docBDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session for docB BEFORE any inverse relations exist.
	session, versionB, errE := b.BeginDocumentEdit(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now insert docA with relation A --X--> B, which causes the bridge to update
	// docB's metadata (adding inverse relations), bumping its revision DURING the edit session.
	docADoc := newDoc()
	docADoc.Claims = &document.ClaimTypes{
		Relation: []document.RelationClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB's metadata to be updated with inverse relations by the bridge.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB should have inverse relations from bridge")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify that docB's revision changed (metadata was updated by bridge during our session).
	_, _, versionBAfterBridge, _, errE := b.GetDocumentLatest(ctx, docB) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionB.Changeset, versionBAfterBridge.Changeset, "changeset should be the same")
	assert.Greater(t, versionBAfterBridge.Revision, versionB.Revision, "revision should have been bumped by bridge")

	// Now append a change to the session and end it. The session should still succeed
	// because BeginDocumentEdit sets Revision to 0, allowing metadata-only updates.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := []string{docB.String(), "SESSION", session.String(), "1"}
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "added after metadata change",
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session (commit). This should NOT fail despite metadata revision change.
	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, versionBAfterBridge.Changeset, newVersion.Changeset,
			"document changeset should change after session completes")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify the document has the new claim AND the new (post-bridge) metadata was carried over.
	updatedDoc, metadata, _, _, errE := b.GetDocumentLatestDoc(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "added after metadata change", updatedDoc.Claims.String[0].String)
	assert.NotEmpty(t, metadata.InverseRelations,
		"new metadata (with inverse relations added during edit) should be carried over")
}

func TestDocumentEditSessionIndexing(t *testing.T) {
	t.Parallel()

	// Set up inverse properties for relation indexing tests.
	propX, propXBase := newDocID()
	propY, propYBase := newDocID()
	propXDoc := makePropertyDoc(t, propX, propXBase, &propY)
	propYDoc := makePropertyDoc(t, propY, propYBase, nil)

	ctx, b, esClient := initBaseInfra(t, nil)
	populateBase(ctx, t, b, []*document.D{propXDoc, propYDoc})

	// Insert two documents.
	docADoc := newDoc()
	docA := docADoc.ID
	docBDoc := newDoc()
	docB := docBDoc.ID

	errE := b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.InsertOrReplaceDocument(ctx, docBDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// Both documents should be in ES.
	assert.True(t, docExists(ctx, t, esClient, b.Index, docA.String()), "docA should exist in ES")
	assert.True(t, docExists(ctx, t, esClient, b.Index, docB.String()), "docB should exist in ES")

	// Add a relation from docA to docB via edit session.
	session, versionA, errE := b.BeginDocumentEdit(ctx, docA)
	require.NoError(t, errE, "% -+#.1v", errE)

	confidence := document.HighConfidence
	changeBase := []string{docA.String(), "SESSION", session.String(), "1"}
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.RelationClaimPatch{
			Confidence: &confidence,
			Prop:       &propX,
			To:         &docB,
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for session completion and ES indexing.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docA)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, versionA.Changeset, newVersion.Changeset, "docA changeset should change")
	}, 30*time.Second, 100*time.Millisecond)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the relation A --X--> B is indexed in ES.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docA, propX, docB),
			"docA should have relation A --X--> B in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify docB gets inverse relation B --Y--> A.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse relation B --Y--> A in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Now remove the relation from docA via another edit session.
	session2, versionA2, errE := b.BeginDocumentEdit(ctx, docA)
	require.NoError(t, errE, "% -+#.1v", errE)

	removeChangeJSON := marshalChange(t, document.RemoveClaimChange{
		ID: claimID,
	})
	seqNo = 1
	_, errE = b.AppendDocumentChange(ctx, session2, removeChangeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndDocumentEdit(ctx, session2, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for session completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docA)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, versionA2.Changeset, newVersion.Changeset, "docA changeset should change after removal")
	}, 30*time.Second, 100*time.Millisecond)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the relation is removed from ES.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.False(c, docHasRelation(ctx, t, esClient, b.Index, docA, propX, docB),
			"docA should no longer have relation A --X--> B in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify docB's inverse relation is also removed.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.Empty(c, metadata.InverseRelations, "docB should have no inverse relations after removal")
	}, 30*time.Second, 100*time.Millisecond)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.False(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should no longer have inverse relation B --Y--> A in ES")
	}, 30*time.Second, 100*time.Millisecond)
}

func TestFileUpload(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	data := []byte("hello world, this is a test file")

	// Begin upload.
	session, errE := b.BeginUpload(ctx, int64(len(data)), "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Upload in two chunks.
	errE = b.UploadChunk(ctx, session, data[:15], 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.UploadChunk(ctx, session, data[15:], 15)
	require.NoError(t, errE, "% -+#.1v", errE)

	// List chunks.
	chunks, errE := b.ListChunks(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, chunks, 2)

	// Get chunk info.
	start, length, errE := b.GetChunk(ctx, session, chunks[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.GreaterOrEqual(t, start, int64(0))
	assert.Positive(t, length)

	// End upload.
	errE = b.EndUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for the file to become available.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		fileData, metadata, errE := b.GetFile(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.Equal(c, data, fileData)
		assert.Equal(c, "test.txt", metadata.Filename)
		assert.Equal(c, "text/plain", metadata.MediaType)
		assert.Equal(c, int64(len(data)), metadata.Size)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestFileUploadDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	data := []byte("should be discarded")

	// Begin upload.
	session, errE := b.BeginUpload(ctx, int64(len(data)), "text/plain", "discard.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Upload a chunk.
	errE = b.UploadChunk(ctx, session, data, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Discard upload.
	errE = b.DiscardUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// File should not be available.
	_, _, errE = b.GetFile(ctx, session)
	assert.Error(t, errE, "file should not exist after discarded upload")
}

func TestGetDocument(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	doc := newDoc()
	docID := doc.ID

	// Insert.
	errE := b.InsertDocument(ctx, docID, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// GetDocumentLatest.
	data, metadata, version, parentChangesets, errE := b.GetDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, data)
	assert.NotNil(t, metadata)
	assert.NotZero(t, version.Changeset)
	assert.Empty(t, parentChangesets)

	// GetDocument with specific version.
	data2, metadata2, version2, _, errE := b.GetDocument(ctx, docID, version)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, data, data2)
	assert.Equal(t, metadata, metadata2)
	assert.Equal(t, version, version2)

	// GetDocumentLatestDoc.
	docResult, metadata3, version3, _, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, docID, docResult.ID)
	assert.Equal(t, metadata, metadata3)
	assert.Equal(t, version, version3)
}

func TestInsertDocument(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	doc := newDoc()
	docID := doc.ID
	doc.Claims = &document.ClaimTypes{
		String: []document.StringClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: identifier.New()},
				String:    "inserted",
			},
		},
	}

	errE := b.InsertDocument(ctx, docID, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the document content.
	result, _, _, _, errE := b.GetDocumentLatestDoc(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, result.Claims)
	require.Len(t, result.Claims.String, 1)
	assert.Equal(t, "inserted", result.Claims.String[0].String)
}

func TestDocumentEditSessionMultipleChanges(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session.
	session, version, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add first claim (operation 0).
	confidence := document.HighConfidence
	propID1 := identifier.New()
	changeBase0 := []string{docID.String(), "SESSION", session.String(), "1"}
	claimID0 := identifier.From(changeBase0...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID0,
		Base: changeBase0,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID1,
			String:     "first",
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add second claim (operation 1).
	propID2 := identifier.New()
	changeBase1 := []string{docID.String(), "SESSION", session.String(), "2"}
	claimID1 := identifier.From(changeBase1...)
	changeJSON = marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID1,
		Base: changeBase1,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID2,
			String:     "second",
		},
	})
	seqNo = 2
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify both changes listed.
	changes, errE := b.ListDocumentChanges(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, changes, 2)

	// End session.
	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version, newVersion)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify both claims exist.
	updatedDoc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 2)
	stringValues := []string{updatedDoc.Claims.String[0].String, updatedDoc.Claims.String[1].String}
	assert.Contains(t, stringValues, "first")
	assert.Contains(t, stringValues, "second")
}

func TestDocumentEditSessionSequential(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// First edit session: add a string claim.
	session1, version1, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	confidence := document.HighConfidence
	propID1 := identifier.New()
	changeBase1 := []string{docID.String(), "SESSION", session1.String(), "1"}
	claimID1 := identifier.From(changeBase1...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID1,
		Base: changeBase1,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID1,
			String:     "from session 1",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session1, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndDocumentEdit(ctx, session1, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for first session to complete.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version1.Changeset, newVersion.Changeset, "changeset should change after first session")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify first claim exists and get the version after first session.
	docAfter1, _, version2, parentChangesets1, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, docAfter1.Claims)
	require.Len(t, docAfter1.Claims.String, 1)
	assert.Equal(t, "from session 1", docAfter1.Claims.String[0].String)
	// The first edit created a new changeset from the initial one.
	assert.NotEqual(t, version1.Changeset, version2.Changeset)

	// Second edit session: add another string claim.
	session2, version2Again, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version2.Changeset, version2Again.Changeset, "BeginDocumentEdit should return the latest version")

	propID2 := identifier.New()
	changeBase2 := []string{docID.String(), "SESSION", session2.String(), "1"}
	claimID2 := identifier.From(changeBase2...)
	changeJSON = marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID2,
		Base: changeBase2,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID2,
			String:     "from session 2",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session2, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndDocumentEdit(ctx, session2, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for second session to complete.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version2.Changeset, newVersion.Changeset, "changeset should change after second session")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify both claims exist.
	docAfter2, _, version3, parentChangesets2, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, docAfter2.Claims)
	require.Len(t, docAfter2.Claims.String, 2)
	stringValues := []string{docAfter2.Claims.String[0].String, docAfter2.Claims.String[1].String}
	assert.Contains(t, stringValues, "from session 1")
	assert.Contains(t, stringValues, "from session 2")

	// Verify the history: three distinct changesets.
	assert.NotEqual(t, version1.Changeset, version2.Changeset)
	assert.NotEqual(t, version2.Changeset, version3.Changeset)
	assert.NotEqual(t, version1.Changeset, version3.Changeset)

	// The latest version should have the previous version as a parent changeset.
	require.NotEmpty(t, parentChangesets2)
	assert.Equal(t, version2.Changeset, parentChangesets2[0].Changeset, "parent of version3 should be version2")

	// version2 should have version1 as parent.
	require.NotEmpty(t, parentChangesets1)
	assert.Equal(t, version1.Changeset, parentChangesets1[0].Changeset, "parent of version2 should be version1")
}

func TestGetDocumentEditSessionNotFound(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Getting a non-existent session should return an error.
	_, errE := b.GetDocumentEditSession(ctx, identifier.New())
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)
}

func TestInsertOrReplaceFileDetectsMediaType(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	fileID := identifier.New()
	data := []byte("some binary data")

	// Use a filename with no recognizable extension so mime detection by content is used.
	errE := b.InsertOrReplaceFile(ctx, fileID, data, "noext")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, metadata, errE := b.GetFile(ctx, fileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "noext", metadata.Filename)
	// Should have detected a media type (content-based detection).
	assert.NotEmpty(t, metadata.MediaType)
}

func TestInitAlreadyInitialized(t *testing.T) {
	t.Parallel()

	ctx, b, _ := initBaseInfra(t, nil)

	// Second Init should fail with "already initialized".
	errE := b.Init(ctx, nil, nil, nil, nil, nil)
	assert.EqualError(t, errE, "already initialized")
}

func TestAppendDocumentChangeToEndedSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document and begin edit.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session immediately (discard).
	errE = b.EndDocumentEdit(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Trying to append after ending should fail.
	_, errE = b.AppendDocumentChange(ctx, session, []byte(`{}`), 1)
	assert.Error(t, errE)

	// Trying to end again should fail.
	errE = b.EndDocumentEdit(ctx, session, false)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)
}

func TestBeginDocumentEditNotFound(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Trying to begin edit for a non-existent document should fail.
	_, _, errE := b.BeginDocumentEdit(ctx, identifier.New())
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestStartInvalidLanguagePriority(t *testing.T) {
	t.Parallel()

	ctx, b, _ := initBaseInfra(t, map[string][]string{
		"invalid_language": {"en"},
	})

	// Start with invalid language priority should fail.
	errE := b.Start(ctx, nil)
	assert.Error(t, errE, "Start with invalid language priority should fail")
}

func TestAppendDocumentChangeWithBadBase(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append a change with wrong base.
	confidence := document.HighConfidence
	propID := identifier.New()
	wrongBase := []string{"wrong", "base", "1"}
	claimID := identifier.From(wrongBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: wrongBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "will fail validation",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	assert.Error(t, errE, "AppendDocumentChange should fail with wrong base")

	// Discard the session.
	errE = b.EndDocumentEdit(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestDocumentEditSessionCompletionWithApplyError(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document with no claims.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, version, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append a RemoveClaimChange for a non-existent claim.
	// Validate() returns nil for RemoveClaimChange, but Apply() will fail.
	changeJSON := marshalChange(t, document.RemoveClaimChange{
		ID: identifier.New(),
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session. The async completion will fail during Apply.
	errE = b.EndDocumentEdit(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for the River job to attempt and fail.
	time.Sleep(5 * time.Second)

	// TODO: We should record the failure in session metadata and check it here.

	// Document should remain unchanged.
	_, _, versionAfter, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, versionAfter, "document version should not change when Apply fails")
}

func TestAppendDocumentChangeWithInvalidJSON(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append invalid change data.
	_, errE = b.AppendDocumentChange(ctx, session, []byte(`{"type":"invalid"}`), 1)
	assert.Error(t, errE, "AppendDocumentChange should fail with invalid change JSON")

	// Discard the session.
	errE = b.EndDocumentEdit(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestListDocumentChangesEmpty(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document and begin edit without appending changes.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginDocumentEdit(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// List changes should return empty list.
	changes, errE := b.ListDocumentChanges(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, changes)

	// End session (discard since no changes).
	errE = b.EndDocumentEdit(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
}
