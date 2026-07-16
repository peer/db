package base_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// Helper IDs for tests.
//
//nolint:gochecknoglobals
var (
	testPropID = identifier.New()
	testDocID  = identifier.New()
)

// makeCoreClaim creates a CoreClaim with the given confidence and optional sub-claims.
func makeCoreClaim(confidence document.Confidence, sub *document.ClaimTypes) document.CoreClaim {
	return document.CoreClaim{
		ID:         identifier.New(),
		Confidence: confidence,
		Sub:        sub,
	}
}

// docPostHook is the document post-hook signature used by TestingWithDocumentHooks.
type docPostHook = func(
	ctx context.Context, doc, latest *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E)

// docPreHook is the document pre-hook signature used by TestingWithDocumentHooks.
type docPreHook = func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E

// fetchOf returns a TestingWithDocumentHooks fetch closure that yields doc marshaled as the latest version, or
// a deleted (nil data) result when doc is nil.
func fetchOf(t *testing.T, doc *document.D) func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	t.Helper()
	var data json.RawMessage
	if doc != nil {
		var errE errors.E
		data, errE = x.MarshalWithoutEscapeHTML(doc)
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	return func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		return data, nil, store.Version{}, nil, nil
	}
}

// addStringHook returns a post-hook that appends a string claim for testPropID with the given value.
func addStringHook(value string) docPostHook {
	return func(_ context.Context, doc, _ *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E) (
		*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E,
	) {
		if errE != nil {
			return doc, metadata, version, parentChangesets, errE
		}
		if doc.Claims == nil {
			doc.Claims = &document.ClaimTypes{}
		}
		doc.Claims.String = append(doc.Claims.String, document.StringClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: testPropID},
			String:    value,
		})
		return doc, metadata, version, parentChangesets, nil
	}
}

func TestWithDocumentHooksPostModifies(t *testing.T) {
	t.Parallel()

	b := &base.B{DocumentPostHooks: []docPostHook{addStringHook("injected")}}                     //nolint:exhaustruct
	in := &document.D{CoreDocument: document.CoreDocument{ID: testDocID}}                         //nolint:exhaustruct
	doc, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetchOf(t, in)) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, doc)
	assert.Len(t, doc.Get(testPropID), 1)
}

func TestWithDocumentHooksMultiplePost(t *testing.T) {
	t.Parallel()

	b := &base.B{DocumentPostHooks: []docPostHook{addStringHook("first"), addStringHook("second")}} //nolint:exhaustruct
	in := &document.D{CoreDocument: document.CoreDocument{ID: testDocID}}                           //nolint:exhaustruct
	doc, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetchOf(t, in))   //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, doc)
	assert.Len(t, doc.Get(testPropID), 2)
}

func TestWithDocumentHooksPostError(t *testing.T) {
	t.Parallel()

	post := []docPostHook{
		func(_ context.Context, _, _ *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, _ errors.E) (
			*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E,
		) {
			return nil, metadata, version, parentChangesets, errors.New("post hook failed")
		},
	}
	b := &base.B{DocumentPostHooks: post}                                                       //nolint:exhaustruct
	in := &document.D{CoreDocument: document.CoreDocument{ID: testDocID}}                       //nolint:exhaustruct
	_, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetchOf(t, in)) //nolint:dogsled
	assert.EqualError(t, errE, "post hook failed")
}

func TestWithDocumentHooksPreErrorSkipsFetch(t *testing.T) {
	t.Parallel()

	fetched := false
	pre := []docPreHook{
		func(_ context.Context, _ identifier.Identifier, _ *store.Version) errors.E {
			return errors.New("pre hook failed")
		},
	}
	fetch := func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		fetched = true
		return nil, nil, store.Version{}, nil, nil
	}
	b := &base.B{DocumentPreHooks: pre}                                                //nolint:exhaustruct
	_, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetch) //nolint:dogsled
	assert.EqualError(t, errE, "pre hook failed")
	assert.False(t, fetched, "fetch must not run when a pre-hook fails")
}

func TestWithDocumentHooksDeleted(t *testing.T) {
	t.Parallel()

	sawNil := false
	post := []docPostHook{
		func(_ context.Context, doc, _ *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E) (
			*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E,
		) {
			sawNil = doc == nil
			return doc, metadata, version, parentChangesets, errE
		},
	}
	b := &base.B{DocumentPostHooks: post}                                                          //nolint:exhaustruct
	doc, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetchOf(t, nil)) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, doc)
	assert.True(t, sawNil, "a post-hook runs with a nil document for a deleted document")
}

// TestDocumentPostHooksLatest verifies that document post-hooks receive the document at its latest
// version, always as a separate instance of the fetched document: with the same content for a
// latest read and for a versioned read of the latest version, and fetched separately for a
// versioned read of another version (here from a changeset).
func TestDocumentPostHooksLatest(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	docBase := []string{"test", identifier.New().String()}
	id := identifier.From(docBase...)
	in := &document.D{CoreDocument: document.CoreDocument{ID: id, Base: docBase}}
	errE := b.InsertDocument(ctx, in)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, version, _, errE := b.GetDocumentLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	// Commit a second version through an edit session, so the insert changeset is not the latest
	// version anymore.
	session, errE := b.BeginEditDocument(ctx, version, in)
	require.NoError(t, errE, "% -+#.1v", errE)
	confidence := document.HighConfidence
	propID := identifier.New()
	changeBase := append(append([]string{}, docBase...), "SESSION", session.String(), "1")
	changeJSON, errE := document.ChangeMarshalJSON(document.AddClaimChange{
		Under: nil,
		ID:    identifier.From(changeBase...),
		Base:  changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "value",
		},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = b.AppendDocumentChange(ctx, session, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	var latestVersion store.Version
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, latestVersion, _, errE = b.GetDocumentLatest(ctx, id)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEqual(c, version.Changeset, latestVersion.Changeset)
	}, 30*time.Second, 100*time.Millisecond)

	var sawAliased, sawLatest bool
	var sawLatestID identifier.Identifier
	b.DocumentPostHooks = append(b.DocumentPostHooks, func(
		_ context.Context, doc, latest *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		sawAliased = doc != nil && doc == latest
		sawLatest = latest != nil
		if latest != nil {
			sawLatestID = latest.ID
		}
		return doc, metadata, version, parentChangesets, errE
	})

	// For a latest read the hook receives the latest document as a separate instance of the fetched
	// document.
	_, _, _, _, errE = b.GetDocumentLatestDoc(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, sawAliased)
	assert.True(t, sawLatest)
	assert.Equal(t, id, sawLatestID)

	// For a versioned read of another version the hook receives the latest version fetched
	// separately.
	_, _, _, _, errE = b.GetDocumentFromChangeset(ctx, version.Changeset, id, 0) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, sawAliased)
	assert.True(t, sawLatest)
	assert.Equal(t, id, sawLatestID)

	// For a versioned read of the latest version the hook receives the latest document as a separate
	// instance of the single fetched document.
	_, _, _, _, errE = b.GetDocumentFromChangeset(ctx, latestVersion.Changeset, id, 0) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, sawAliased)
	assert.True(t, sawLatest)
	assert.Equal(t, id, sawLatestID)

	// Once the document is deleted at its latest version, a versioned read still reads the requested
	// version and the hook receives it without a latest document and with the deleted error: access
	// to versions of a deleted document is for the post-hooks to decide (by transforming the error).
	errE = b.DeleteDocument(ctx, id)
	require.NoError(t, errE, "% -+#.1v", errE)
	data, _, _, _, errE := b.GetDocumentFromChangeset(ctx, version.Changeset, id, 0) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.NotNil(t, data)
	assert.False(t, sawAliased)
	assert.False(t, sawLatest)
}

// TestGetDocumentFromChangesetRunsPreHooks verifies that GetDocumentFromChangeset runs the document
// pre-hooks. A denying pre-hook blocks the read before the store is reached, so the (nil) store is
// never touched.
func TestGetDocumentFromChangesetRunsPreHooks(t *testing.T) {
	t.Parallel()

	b := &base.B{ //nolint:exhaustruct
		DocumentPreHooks: []docPreHook{
			func(_ context.Context, _ identifier.Identifier, _ *store.Version) errors.E {
				return errors.WithStack(auth.ErrAccessDenied)
			},
		},
	}
	_, _, _, _, errE := b.GetDocumentFromChangeset(t.Context(), identifier.New(), testDocID, 0) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}

// TestGetFileFromChangesetRunsPreHooks verifies that GetFileFromChangeset runs the file pre-hooks. A
// denying pre-hook blocks the read before the store is reached, so the (nil) store is never touched.
func TestGetFileFromChangesetRunsPreHooks(t *testing.T) {
	t.Parallel()

	b := &base.B{ //nolint:exhaustruct
		FilePreHooks: []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E{
			func(_ context.Context, _ identifier.Identifier, _ *store.Version) errors.E {
				return errors.WithStack(auth.ErrAccessDenied)
			},
		},
	}
	_, _, _, _, errE := b.GetFileFromChangeset(t.Context(), identifier.New(), identifier.New(), 0) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}
