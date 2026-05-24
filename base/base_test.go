package base_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalBase "gitlab.com/peerdb/peerdb/internal/base"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

// initBaseInfra initializes the base infrastructure (PostgreSQL, Elasticsearch, River)
// without populating core documents. Callers must call populateBase or b.PopulateAndStart
// to insert documents and start the base.
func initBaseInfra(t *testing.T, languagePriority map[string][]string) (context.Context, *base.B, *elasticsearch.TypedClient) {
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
		_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(context.Background())
		require.NoError(t, err)
	})

	b, _, errE := internalBase.InitComponents(ctx, logger, dbpool, esClient, schema, index, 1)
	b.LanguagePriority = languagePriority
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

	onShutdown, errE := b.PopulateAndStart(ctx, transformed, nil, nil, nil, nil)
	t.Cleanup(onShutdown)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func initBase(t *testing.T) (context.Context, *base.B) {
	t.Helper()

	ctx, b, _ := initBaseWithES(t)
	return ctx, b
}

func initBaseWithES(t *testing.T) (context.Context, *base.B, *elasticsearch.TypedClient) {
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

func makePropertyDoc(t *testing.T, id identifier.Identifier, base []string, inverseOf *identifier.Identifier) *document.D {
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

	// Verify it exists and changeset ID for FIRST insert is derivable.
	_, _, version1, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	expectedFirstChangeset := identifier.From(append(append([]string{}, doc.Base...), "CHANGESET", "FIRST")...)
	assert.Equal(t, expectedFirstChangeset, version1.Changeset)

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

	// Verify the document was replaced and changeset ID for REPLACE is derivable.
	_, _, version2, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEqual(t, version1, version2)
	expectedReplaceChangeset := identifier.From(
		append(append([]string{}, doc.Base...), "CHANGESET", "REPLACE", version1.Changeset.String())...,
	)
	assert.Equal(t, expectedReplaceChangeset, version2.Changeset)
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
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
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

	fileBase := []string{"test", identifier.New().String()}
	data := []byte("hello world")

	// Insert.
	fileID, errE := b.InsertOrReplaceFile(ctx, fileBase, data, "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify file ID is derived from base.
	assert.Equal(t, identifier.From(fileBase...), fileID)

	// Verify it exists and changeset ID for FIRST is derivable.
	fileData, metadata, fileVersion1, _, errE := b.GetFileLatest(ctx, fileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, data, fileData)
	assert.Equal(t, "test.txt", metadata.Filename)

	// Verify file metadata Base is recorded.
	assert.Equal(t, fileBase, metadata.Base)
	assert.Equal(t, fileID, identifier.From(metadata.Base...))

	// Verify changeset ID for FIRST insert is derivable.
	expectedFirstChangeset := identifier.From(append(append([]string{}, fileBase...), "CHANGESET", "FIRST")...)
	assert.Equal(t, expectedFirstChangeset, fileVersion1.Changeset)

	// Replace with new content.
	newData := []byte("updated content")
	fileID2, errE := b.InsertOrReplaceFile(ctx, fileBase, newData, "test2.txt")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, fileID, fileID2)

	// Verify it was replaced and changeset ID for REPLACE is derivable.
	fileData, metadata, fileVersion2, _, errE := b.GetFileLatest(ctx, fileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newData, fileData)
	assert.Equal(t, "test2.txt", metadata.Filename)

	// Verify file metadata Base is still recorded after replace.
	assert.Equal(t, fileBase, metadata.Base)

	// Verify changeset ID for REPLACE is derivable.
	expectedReplaceChangeset := identifier.From(
		append(append([]string{}, fileBase...), "CHANGESET", "REPLACE", fileVersion1.Changeset.String())...,
	)
	assert.Equal(t, expectedReplaceChangeset, fileVersion2.Changeset)
}

// marshalChange marshals a single document change to JSON for use with AppendDocumentChange.
func marshalChange(t *testing.T, change document.Change) []byte {
	t.Helper()
	data, errE := document.ChangeMarshalJSON(change)
	require.NoError(t, errE, "% -+#.1v", errE)
	return data
}

// documentCommitUser fetches the document changeset by ID, asserts it is
// committed to exactly one view, and returns the CommitMetadata.User from
// that commit. Lets tests verify who actually committed a changeset (vs the
// contributor union on DocumentMetadata.Users).
func documentCommitUser(ctx context.Context, t *testing.T, b *base.B, changesetID identifier.Identifier) *store.User {
	t.Helper()
	cs, errE := b.DocumentChangeset(ctx, changesetID)
	require.NoError(t, errE, "% -+#.1v", errE)
	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, views, 1, "document changeset committed to exactly one view")
	md, errE := views[0].Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	return md.User
}

// fileCommitUser is the file-store equivalent of documentCommitUser.
func fileCommitUser(ctx context.Context, t *testing.T, b *base.B, changesetID identifier.Identifier) *store.User {
	t.Helper()
	cs, errE := b.FileChangeset(ctx, changesetID)
	require.NoError(t, errE, "% -+#.1v", errE)
	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, views, 1, "file changeset committed to exactly one view")
	md, errE := views[0].Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	return md.User
}

// docExists checks if a document exists in the Elasticsearch index.

func TestDocumentEditSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)
	const editorSubject = "user-editor"
	ctx = auth.WithSubject(ctx, editorSubject)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session.
	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify session is active.
	beginMetadata, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, beginMetadata)
	assert.Equal(t, docID, beginMetadata.DocumentID)
	require.NotNil(t, beginMetadata.Version)
	assert.False(t, sessionEnded)
	assert.Nil(t, completeMetadata)

	// Create a change that adds a string claim.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
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

	// Verify claim ID is derivable from its base.
	assert.Equal(t, claimID, identifier.From(changeBase...))

	// List changes.
	changes, errE := b.ListDocumentChanges(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{1}, changes)

	// Get change data.
	changeData, errE := b.GetDocumentChange(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, changeData)

	// End session (commit).
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Session should be ended now but not yet completed.
	beginMetadata2, sessionEnded2, completeMetadata2, errE2 := b.GetEditDocumentSession(ctx, session)
	require.NoError(t, errE2, "% -+#.1v", errE2)
	require.NotNil(t, beginMetadata2)
	assert.Equal(t, docID, beginMetadata2.DocumentID)
	assert.True(t, sessionEnded2)
	// Complete metadata may or may not be available yet (async).

	// Wait for async completion to update the document.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version, newVersion, "document version should change after session completes")
	}, 30*time.Second, 100*time.Millisecond)

	// After completion, GetEditDocumentSession should return complete metadata.
	beginMetadata3, sessionEnded3, completeMetadata3, errE3 := b.GetEditDocumentSession(ctx, session)
	require.NoError(t, errE3, "% -+#.1v", errE3)
	require.NotNil(t, beginMetadata3)
	assert.Equal(t, docID, beginMetadata3.DocumentID)
	assert.True(t, sessionEnded3)
	if assert.NotNil(t, completeMetadata3) {
		assert.False(t, completeMetadata3.Discarded)
		assert.NotNil(t, completeMetadata3.Changeset)
	}

	// Verify the document has the new claim.
	updatedDoc, metadata, newVersion, _, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "test value", updatedDoc.Claims.String[0].String)

	// Single editor session: Users union is the begin+per-change user (same subject).
	assert.Equal(t, []store.User{{ID: editorSubject}}, metadata.Users)
	// And the committer (CommitMetadata.User) is the same subject too.
	assert.Equal(t, &store.User{ID: editorSubject}, documentCommitUser(ctx, t, b, newVersion.Changeset))

	// Verify changeset ID is derived from doc Base + "SESSION" + session.
	expectedChangesetBase := append(append([]string{}, doc.Base...), "SESSION", session.String())
	expectedChangesetID := identifier.From(expectedChangesetBase...)
	assert.Equal(t, expectedChangesetID, newVersion.Changeset)

	// Verify claim ID is derivable from its base.
	assert.Equal(t, claimID, updatedDoc.Claims.String[0].ID)
	assert.Equal(t, identifier.From(changeBase...), updatedDoc.Claims.String[0].ID)

	// Verify complete metadata changeset matches the document version.
	_ = completeMetadata2
	assert.Equal(t, newVersion.Changeset, *completeMetadata3.Changeset)
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
	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a change.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
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
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion using GetEditDocumentSession.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		beginMetadata, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, beginMetadata) {
			assert.Equal(c, docID, beginMetadata.DocumentID)
		}
		assert.True(c, sessionEnded)
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// Verify document is unchanged.
	_, _, versionAfter, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, versionAfter, "document version should not change after discarded session")

	updatedDoc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, updatedDoc.Claims, "document should have no claims after discarded session")
}

func TestDocumentCreateSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Pre-allocate the document ID and base, just like DocumentCreatePostAPI does.
	docID, docBase := newDocID()

	// The document should not yet exist in the store.
	_, _, _, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.ErrorIs(t, errE, store.ErrValueNotFound)

	session, errE := b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Active session has nil Version (create session marker).
	beginMetadata, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, beginMetadata)
	assert.Equal(t, docID, beginMetadata.DocumentID)
	assert.Equal(t, docBase, beginMetadata.Base)
	assert.Nil(t, beginMetadata.Version, "create session should have nil Version")
	assert.False(t, sessionEnded)
	assert.Nil(t, completeMetadata)

	// Append a single string-claim change against the session.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, docBase...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "first value",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Save (end without discard).
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Discarded)
			assert.NotNil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// The materialized document has the appended claim.
	updatedDoc, metadata, latestVersion, parents, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "first value", updatedDoc.Claims.String[0].String)
	assert.Equal(t, claimID, updatedDoc.Claims.String[0].ID)

	// Unauthenticated session (no subject attached to ctx): Users union is empty
	// and CommitMetadata.User is nil.
	assert.Empty(t, metadata.Users)
	assert.Nil(t, documentCommitUser(ctx, t, b, latestVersion.Changeset))

	// Latest version sits at the SESSION changeset; its parent is the FIRST changeset.
	expectedSessionChangeset := identifier.From(append(append([]string{}, docBase...), "SESSION", session.String())...)
	expectedFirstChangeset := identifier.From(append(append([]string{}, docBase...), "CHANGESET", "FIRST")...)
	assert.Equal(t, expectedSessionChangeset, latestVersion.Changeset)
	require.Len(t, parents, 1)
	assert.Equal(t, expectedFirstChangeset, parents[0].Changeset)
}

func TestDocumentCreateSessionDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	docID, docBase := newDocID()

	session, errE := b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append a change so we can also exercise the "had changes but discarded" branch.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, docBase...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "thrown away",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// No document was ever materialized in the store.
	_, _, _, _, errE = b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestDocumentCreateSessionNoChanges(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	docID, docBase := newDocID()

	session, errE := b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End without appending any change. Same effect as discard: no document inserted.
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	_, _, _, _, errE = b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.ErrorIs(t, errE, store.ErrValueNotFound)
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
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for docB's metadata to have inverse relations from the bridge.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, _, errE := b.GetDocumentLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB should have inverse relations")
	}, 30*time.Second, 100*time.Millisecond)

	// Run the edit session as an authenticated user.
	const editorSubject = "user-editor"
	sessionCtx := auth.WithSubject(ctx, editorSubject)

	// Begin edit session for docB.
	session, versionB, errE := b.BeginEditDocumentLatest(sessionCtx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a string claim to docB through the edit session.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, docBDoc.Base...), "SESSION", session.String(), "1")
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
	_, errE = b.AppendDocumentChange(sessionCtx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session (commit).
	errE = b.EndEditDocument(sessionCtx, session, false)
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
	updatedDoc, metadata, newVersion, _, errE := b.GetDocumentLatestDoc(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "edited via session", updatedDoc.Claims.String[0].String)
	assert.NotEmpty(t, metadata.InverseRelations, "inverse relations should be carried over after edit session")
	// Single editor session: Users union is the begin+per-change user (same subject).
	assert.Equal(t, []store.User{{ID: editorSubject}}, metadata.Users)
	assert.Equal(t, &store.User{ID: editorSubject}, documentCommitUser(ctx, t, b, newVersion.Changeset))
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

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Run the edit session as an authenticated user.
	const editorSubject = "user-editor"
	sessionCtx := auth.WithSubject(ctx, editorSubject)

	// Begin edit session for docB BEFORE any inverse relations exist.
	session, versionB, errE := b.BeginEditDocumentLatest(sessionCtx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now insert docA with relation A --X--> B, which causes the bridge to update
	// docB's metadata (adding inverse relations), bumping its revision DURING the edit session.
	docADoc := newDoc()
	docADoc.Claims = &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: propX},
				To:        document.Reference{ID: docB},
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, docADoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
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
	// because BeginEditDocumentLatest sets Revision to 0, allowing metadata-only updates.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, docBDoc.Base...), "SESSION", session.String(), "1")
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
	_, errE = b.AppendDocumentChange(sessionCtx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session (commit). This should NOT fail despite metadata revision change.
	errE = b.EndEditDocument(sessionCtx, session, false)
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
	updatedDoc, metadata, newVersion, _, errE := b.GetDocumentLatestDoc(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 1)
	assert.Equal(t, "added after metadata change", updatedDoc.Claims.String[0].String)
	assert.NotEmpty(t, metadata.InverseRelations,
		"new metadata (with inverse relations added during edit) should be carried over")
	// Single editor session: Users union is the begin+per-change user (same subject).
	assert.Equal(t, []store.User{{ID: editorSubject}}, metadata.Users)
	assert.Equal(t, &store.User{ID: editorSubject}, documentCommitUser(ctx, t, b, newVersion.Changeset))
}

// TestDocumentEditSessionMultipleActors exercises the case where the begin,
// per-change, and end actors are three distinct users. The contributor union
// (DocumentMetadata.Users) collects begin + per-change actors sorted by ID;
// the committer (CommitMetadata.User) is the end actor and is NOT in the
// contributor union.
func TestDocumentEditSessionMultipleActors(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Three distinct subjects, one per session phase.
	const (
		beginSubject  = "user-a"
		appendSubject = "user-b"
		endSubject    = "user-c"
	)
	beginCtx := auth.WithSubject(ctx, beginSubject)
	appendCtx := auth.WithSubject(ctx, appendSubject)
	endCtx := auth.WithSubject(ctx, endSubject)

	session, version, errE := b.BeginEditDocumentLatest(beginCtx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "multi-actor value",
		},
	})
	_, errE = b.AppendDocumentChange(appendCtx, session, changeJSON, int64(1))
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocument(endCtx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for async completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version, newVersion, "document version should change after session completes")
	}, 30*time.Second, 100*time.Millisecond)

	_, metadata, newVersion, _, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Contributor union: begin + per-change actor, sorted by ID. End actor (user-c) excluded.
	assert.Equal(t, []store.User{{ID: beginSubject}, {ID: appendSubject}}, metadata.Users)
	// Committer: end actor.
	assert.Equal(t, &store.User{ID: endSubject}, documentCommitUser(ctx, t, b, newVersion.Changeset))
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

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
	require.NoError(t, err)

	// Both documents should be in ES.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.True(c, testutils.DocExists(ctx, t, esClient, b.Index, docA.String()), "docA should exist in ES")
		assert.True(c, testutils.DocExists(ctx, t, esClient, b.Index, docB.String()), "docB should exist in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Add a relation from docA to docB via edit session.
	session, versionA, errE := b.BeginEditDocumentLatest(ctx, docA)
	require.NoError(t, errE, "% -+#.1v", errE)

	confidence := document.HighConfidence
	changeBase := append(append([]string{}, docADoc.Base...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.ReferenceClaimPatch{
			Confidence: &confidence,
			Prop:       &propX,
			To:         &docB,
		},
	})
	seqNo := int64(1)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for session completion and ES indexing.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docA)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, versionA.Changeset, newVersion.Changeset, "docA changeset should change")
	}, 30*time.Second, 100*time.Millisecond)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the relation A --X--> B is indexed in ES.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docA, propX, docB),
			"docA should have relation A --X--> B in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Verify docB gets inverse relation B --Y--> A.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse relation B --Y--> A in ES")
	}, 30*time.Second, 100*time.Millisecond)

	// Now remove the relation from docA via another edit session.
	session2, versionA2, errE := b.BeginEditDocumentLatest(ctx, docA)
	require.NoError(t, errE, "% -+#.1v", errE)

	removeChangeJSON := marshalChange(t, document.RemoveClaimChange{
		ID: claimID,
	})
	seqNo = 1
	_, errE = b.AppendDocumentChange(ctx, session2, removeChangeJSON, seqNo)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocument(ctx, session2, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for session completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docA)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, versionA2.Changeset, newVersion.Changeset, "docA changeset should change after removal")
	}, 30*time.Second, 100*time.Millisecond)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify the relation is removed from ES.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.False(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docA, propX, docB),
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
		_, err := esClient.Indices.Refresh().Index(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.False(c, testutils.DocHasReference(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should no longer have inverse relation B --Y--> A in ES")
	}, 30*time.Second, 100*time.Millisecond)
}

func TestFileUpload(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)
	const uploaderSubject = "user-uploader"
	ctx = auth.WithSubject(ctx, uploaderSubject)

	data := []byte("hello world, this is a test file")

	fileBase := []string{"test", identifier.New().String()}

	// Begin upload.
	session, errE := b.BeginUploadNew(ctx, fileBase, int64(len(data)), "text/plain", "test.txt")
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

	// Wait for the file to become available. File ID is derived from base + "STORAGE" + session.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
	expectedFileID := identifier.From(expectedBase...)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		fileData, metadata, _, _, errE := b.GetFileLatest(ctx, expectedFileID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.Equal(c, data, fileData)
		assert.Equal(c, "test.txt", metadata.Filename)
		assert.Equal(c, "text/plain", metadata.MediaType)
		assert.Equal(c, int64(len(data)), metadata.Size)
		// Verify file metadata Base is recorded and file ID is derivable from it.
		assert.Equal(c, expectedBase, metadata.Base)
		assert.Equal(c, expectedFileID, identifier.From(metadata.Base...))
		// Single uploader: Users union is the begin+per-chunk user (same subject).
		assert.Equal(c, []store.User{{ID: uploaderSubject}}, metadata.Users)
	}, 30*time.Second, 100*time.Millisecond)

	// File changeset ID is derived from base + "STORAGE" + session + "SESSION" + session
	// (see storage.completeStorageSessionTx standalone path). The committer is the End-upload actor.
	fileChangesetID := identifier.From(append(append([]string{}, expectedBase...), "SESSION", session.String())...)
	assert.Equal(t, &store.User{ID: uploaderSubject}, fileCommitUser(ctx, t, b, fileChangesetID))
}

func TestFileUploadDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	data := []byte("should be discarded")

	fileBase := []string{"test", identifier.New().String()}

	// Begin upload.
	session, errE := b.BeginUploadNew(ctx, fileBase, int64(len(data)), "text/plain", "discard.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Upload a chunk.
	errE = b.UploadChunk(ctx, session, data, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Discard upload.
	errE = b.DiscardUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for completion and verify it is marked as discarded.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, completeMetadata, errE := b.GetUploadSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.ID)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// File should not be available.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
	expectedFileID := identifier.From(expectedBase...)
	_, _, _, _, errE = b.GetFileLatest(ctx, expectedFileID) //nolint:dogsled
	assert.EqualError(t, errE, "value not found")
}

func TestGetDocument(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	doc := newDoc()
	docID := doc.ID

	// Insert.
	errE := b.InsertDocument(ctx, doc)
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

	errE := b.InsertDocument(ctx, doc)
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
	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add first claim (operation 0).
	confidence := document.HighConfidence
	propID1 := identifier.New()
	changeBase0 := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
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
	changeBase1 := append(append([]string{}, doc.Base...), "SESSION", session.String(), "2")
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
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version, newVersion)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify both claims exist and their IDs are derivable from their bases.
	updatedDoc, _, newVersion, _, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, updatedDoc.Claims)
	require.Len(t, updatedDoc.Claims.String, 2)
	stringValues := []string{updatedDoc.Claims.String[0].String, updatedDoc.Claims.String[1].String}
	assert.Contains(t, stringValues, "first")
	assert.Contains(t, stringValues, "second")

	// Verify changeset ID is derived from doc Base + "SESSION" + session.
	expectedChangesetBase := append(append([]string{}, doc.Base...), "SESSION", session.String())
	assert.Equal(t, identifier.From(expectedChangesetBase...), newVersion.Changeset)

	// Verify each stored claim ID matches its derivation base.
	storedClaimIDs := map[identifier.Identifier]bool{
		updatedDoc.Claims.String[0].ID: true,
		updatedDoc.Claims.String[1].ID: true,
	}
	assert.True(t, storedClaimIDs[claimID0], "first claim ID should match derivation")
	assert.True(t, storedClaimIDs[claimID1], "second claim ID should match derivation")
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
	session1, version1, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	confidence := document.HighConfidence
	propID1 := identifier.New()
	changeBase1 := append(append([]string{}, doc.Base...), "SESSION", session1.String(), "1")
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

	errE = b.EndEditDocument(ctx, session1, false)
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

	// Verify changeset ID for first session is derived from doc Base + "SESSION" + session1.
	expectedChangeset1 := identifier.From(append(append([]string{}, doc.Base...), "SESSION", session1.String())...)
	assert.Equal(t, expectedChangeset1, version2.Changeset)

	// Verify claim ID from first session is stored correctly.
	assert.Equal(t, claimID1, docAfter1.Claims.String[0].ID)

	// Second edit session: add another string claim.
	session2, version2Again, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version2.Changeset, version2Again.Changeset, "BeginEditDocumentLatest should return the latest version")

	propID2 := identifier.New()
	changeBase2 := append(append([]string{}, doc.Base...), "SESSION", session2.String(), "1")
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

	errE = b.EndEditDocument(ctx, session2, false)
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

	// Verify changeset ID for second session is derived from doc Base + "SESSION" + session2.
	expectedChangeset2 := identifier.From(append(append([]string{}, doc.Base...), "SESSION", session2.String())...)
	assert.Equal(t, expectedChangeset2, version3.Changeset)

	// Verify claim IDs from both sessions are stored correctly.
	storedClaimIDs := map[identifier.Identifier]bool{
		docAfter2.Claims.String[0].ID: true,
		docAfter2.Claims.String[1].ID: true,
	}
	assert.True(t, storedClaimIDs[claimID1], "claim from session 1 should be stored")
	assert.True(t, storedClaimIDs[claimID2], "claim from session 2 should be stored")

	// Verify the history: three distinct changesets.
	assert.NotEqual(t, version1.Changeset, version2.Changeset)
	assert.NotEqual(t, version2.Changeset, version3.Changeset)
	assert.NotEqual(t, version1.Changeset, version3.Changeset)

	// Verify initial changeset is derivable from doc Base + "CHANGESET" + "FIRST".
	expectedFirstChangeset := identifier.From(append(append([]string{}, doc.Base...), "CHANGESET", "FIRST")...)
	assert.Equal(t, expectedFirstChangeset, version1.Changeset)

	// The latest version should have the previous version as a parent changeset.
	require.NotEmpty(t, parentChangesets2)
	assert.Equal(t, version2.Changeset, parentChangesets2[0].Changeset, "parent of version3 should be version2")

	// version2 should have version1 as parent.
	require.NotEmpty(t, parentChangesets1)
	assert.Equal(t, version1.Changeset, parentChangesets1[0].Changeset, "parent of version2 should be version1")
}

func TestGetEditDocumentSessionNotFound(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Getting a non-existent session should return an error.
	_, _, _, errE := b.GetEditDocumentSession(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)
}

func TestInsertOrReplaceFileDetectsMediaType(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	fileBase := []string{"test", identifier.New().String()}
	data := []byte("some binary data")

	// Use a filename with no recognizable extension so mime detection by content is used.
	fileID, errE := b.InsertOrReplaceFile(ctx, fileBase, data, "noext")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, metadata, _, _, errE := b.GetFileLatest(ctx, fileID) //nolint:dogsled
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

	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session immediately (discard).
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Trying to append after ending should fail.
	_, errE = b.AppendDocumentChange(ctx, session, []byte(`{}`), 1)
	assert.EqualError(t, errE, "change type not supported")

	// Trying to end again should fail.
	errE = b.EndEditDocument(ctx, session, false)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)
}

func TestBeginEditDocumentLatestNotFound(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Trying to begin edit for a non-existent document should fail.
	_, _, errE := b.BeginEditDocumentLatest(ctx, identifier.New())
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestStartInvalidLanguagePriority(t *testing.T) {
	t.Parallel()

	ctx, b, _ := initBaseInfra(t, map[string][]string{
		"invalid_language": {"en"},
	})

	// Start with invalid language priority should fail.
	onShutdown, errE := b.Start(ctx, nil)
	if onShutdown != nil {
		defer onShutdown()
	}
	assert.EqualError(t, errE, "unsupported language in priority key")
}

func TestAppendDocumentChangeWithBadBase(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
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
	assert.EqualError(t, errE, "invalid base")

	// Discard the session.
	errE = b.EndEditDocument(ctx, session, true)
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

	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
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
	errE = b.EndEditDocument(ctx, session, false)
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

	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append invalid change data.
	_, errE = b.AppendDocumentChange(ctx, session, []byte(`{"type":"invalid"}`), 1)
	assert.EqualError(t, errE, "change type not supported")

	// Discard the session.
	errE = b.EndEditDocument(ctx, session, true)
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

	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// List changes should return empty list.
	changes, errE := b.ListDocumentChanges(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, changes)

	// End session (discard since no changes).
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestEndEditDocumentEmptyNoDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document and begin edit without appending changes.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Get initial version.
	_, _, version, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session without discard but with no changes.
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Session should be ended now.
	_, sessionEnded, _, errE := b.GetEditDocumentSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, sessionEnded)

	// Wait for completion and verify it is treated as discarded (no changes).
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		beginMetadata, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, beginMetadata) {
			assert.Equal(c, docID, beginMetadata.DocumentID)
		}
		assert.True(c, sessionEnded)
		if assert.NotNil(c, completeMetadata) {
			// Empty changes are treated as discard.
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// Document version should not change (empty changes behave like discard).
	_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, newVersion, "document version should not change with empty changes")
}

func TestFileUploadDuringDocumentEdit(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin document edit session.
	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin a file upload as part of the document edit session.
	fileBase := append(append([]string{}, doc.Base...), "FILE", identifier.New().String())
	fileData := []byte("file content during edit")
	uploadSession, errE := b.BeginUploadNew(ctx, fileBase, int64(len(fileData)), "text/plain", "edit-file.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Upload the file data.
	errE = b.UploadChunk(ctx, uploadSession, fileData, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End the upload linked to the document edit session.
	errE = b.EndEditDocumentUpload(ctx, uploadSession, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for the upload session to complete (file inserted into changeset but not committed).
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, completeMetadata, errE := b.GetUploadSession(ctx, uploadSession)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotNil(c, completeMetadata)
	}, 30*time.Second, 100*time.Millisecond)

	// Add a change to the document.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "with file",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End document edit session (commit).
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for document edit session to complete.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version.Changeset, newVersion.Changeset)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify the file is now available (committed by the document session).
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", uploadSession.String())
	expectedFileID := identifier.From(expectedBase...)
	data, metadata, _, _, errE := b.GetFileLatest(ctx, expectedFileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, fileData, data)
	assert.Equal(t, "edit-file.txt", metadata.Filename)
	// Verify file metadata Base is recorded and file ID is derivable from it.
	assert.Equal(t, expectedBase, metadata.Base)
	assert.Equal(t, expectedFileID, identifier.From(metadata.Base...))

	// Verify the file is also accessible through the changeset.
	changesetBase := append(append([]string{}, doc.Base...), "SESSION", session.String())
	changesetID := identifier.From(changesetBase...)
	changesetData, changesetMetadata, changesetVersion, _, errE := b.GetFileFromChangeset(ctx, changesetID, expectedFileID, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, fileData, changesetData)
	assert.Equal(t, "edit-file.txt", changesetMetadata.Filename)
	assert.Equal(t, changesetID, changesetVersion.Changeset)
}

func TestFileUploadDuringDocumentEditDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin document edit session.
	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Upload a file as part of the document edit session.
	fileBase := append(append([]string{}, doc.Base...), "FILE", identifier.New().String())
	fileData := []byte("file to be discarded")
	uploadSession, errE := b.BeginUploadNew(ctx, fileBase, int64(len(fileData)), "text/plain", "discarded-file.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.UploadChunk(ctx, uploadSession, fileData, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocumentUpload(ctx, uploadSession, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Discard the document edit session (no document changes).
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for document edit session completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.True(c, sessionEnded)
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// The file should NOT be available because the session was discarded.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", uploadSession.String())
	expectedFileID := identifier.From(expectedBase...)
	_, _, _, _, errE = b.GetFileLatest(ctx, expectedFileID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestEndEditDocumentUploadEndedSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin and end document edit session.
	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Try to end a file upload with the ended document session.
	fileBase := append(append([]string{}, doc.Base...), "FILE", identifier.New().String())
	fileData := []byte("should fail")
	uploadSession, errE := b.BeginUploadNew(ctx, fileBase, int64(len(fileData)), "text/plain", "fail.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.UploadChunk(ctx, uploadSession, fileData, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndEditDocumentUpload(ctx, uploadSession, session)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	// The file upload itself can still be discarded.
	errE = b.DiscardUpload(ctx, uploadSession)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestFileUploadCompletionAfterEditSessionDiscard(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin document edit session.
	session, _, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin a file upload as part of the document edit session.
	fileBase := append(append([]string{}, doc.Base...), "FILE", identifier.New().String())
	fileData := []byte("upload before session ends")
	uploadSession, errE := b.BeginUploadNew(ctx, fileBase, int64(len(fileData)), "text/plain", "late-file.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.UploadChunk(ctx, uploadSession, fileData, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End the upload linked to the document edit session.
	// This succeeds because the edit session is still active.
	errE = b.EndEditDocumentUpload(ctx, uploadSession, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now discard the document edit session.
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for document edit session to complete as discarded.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.True(c, sessionEnded)
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Discarded)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// Wait for the upload session to complete. With the document session already discarded,
	// the upload completion either:
	// a) Ran before the document session discard: inserted the file into the changeset
	//    (completeMetadata has Discarded=false with an ID, but the changeset was later discarded).
	// b) Ran after the document session discard: found the primary session already ended
	//    and completed as discarded (completeMetadata has Discarded=true).
	// In both cases the file is never committed to the main view.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, completeMetadata, errE := b.GetUploadSession(ctx, uploadSession)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotNil(c, completeMetadata)
	}, 30*time.Second, 100*time.Millisecond)

	// The file should not be in the main view.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", uploadSession.String())
	expectedFileID := identifier.From(expectedBase...)
	_, _, _, _, errE = b.GetFileLatest(ctx, expectedFileID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// The file should not be accessible through the changeset either (changeset was discarded).
	changesetBase := append(append([]string{}, doc.Base...), "SESSION", session.String())
	changesetID := identifier.From(changesetBase...)
	_, _, _, _, errE = b.GetFileFromChangeset(ctx, changesetID, expectedFileID, 0) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Discarding the upload session should return ErrAlreadyEnded since it was already ended.
	errE = b.DiscardUpload(ctx, uploadSession)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)
}

func TestDocumentIDDerivation(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create and insert a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify document ID is derived from its Base.
	assert.Equal(t, identifier.From(doc.Base...), docID)

	// Verify Base has domain as first element.
	assert.Equal(t, "test", doc.Base[0])

	// Retrieve document and verify Base is preserved.
	result, _, version, _, errE := b.GetDocumentLatestDoc(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, doc.Base, result.Base)
	assert.Equal(t, docID, identifier.From(result.Base...))

	// Verify changeset ID is derived from base.
	// For InsertOrReplaceDocument, the first changeset base is doc.Base + "CHANGESET" + "FIRST".
	expectedChangesetBase := append(append([]string{}, doc.Base...), "CHANGESET", "FIRST")
	expectedChangesetID := identifier.From(expectedChangesetBase...)
	assert.Equal(t, expectedChangesetID, version.Changeset)
}

func TestGetUploadSession(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	fileBase := []string{"test", identifier.New().String()}
	data := []byte("upload session test")

	// Begin upload.
	session, errE := b.BeginUploadNew(ctx, fileBase, int64(len(data)), "text/plain", "session-test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Session should not be ended yet.
	ended, completeMetadata, errE := b.GetUploadSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, ended)
	assert.Nil(t, completeMetadata)

	// Upload data and end.
	errE = b.UploadChunk(ctx, session, data, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.EndUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Session should be ended.
	ended, _, errE = b.GetUploadSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, ended)

	// Wait for completion.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, completeMetadata, errE := b.GetUploadSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Discarded)
			assert.NotNil(c, completeMetadata.ID)
			// Verify the file ID is derivable from file metadata base.
			expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
			assert.Equal(c, identifier.From(expectedBase...), *completeMetadata.ID)
		}
	}, 30*time.Second, 100*time.Millisecond)
}

func TestGetEditDocumentSessionCompleted(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	// Create a document.
	doc := newDoc()
	docID := doc.ID
	errE := b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Begin edit session.
	session, version, errE := b.BeginEditDocumentLatest(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a change.
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, doc.Base...), "SESSION", session.String(), "1")
	claimID := identifier.From(changeBase...)
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "completion test",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End session.
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for completion and verify complete metadata.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		beginMetadata, sessionEnded, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, beginMetadata) {
			assert.Equal(c, docID, beginMetadata.DocumentID)
		}
		assert.True(c, sessionEnded)
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Discarded)
			assert.NotNil(c, completeMetadata.Changeset)
			// Verify changeset ID is derived from base.
			expectedChangesetBase := append(append([]string{}, doc.Base...), "SESSION", session.String())
			expectedChangesetID := identifier.From(expectedChangesetBase...)
			assert.Equal(c, expectedChangesetID, *completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)

	// Verify document was updated.
	_, _, newVersion, _, errE := b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEqual(t, version.Changeset, newVersion.Changeset)
}
