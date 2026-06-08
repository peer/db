package base_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

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
	ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
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
	return func(_ context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E) (
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
		func(_ context.Context, _ *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, _ errors.E) (
			*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E,
		) {
			return nil, metadata, version, parentChangesets, errors.New("post hook failed")
		},
	}
	b := &base.B{DocumentPostHooks: post}                                                       //nolint:exhaustruct
	in := &document.D{CoreDocument: document.CoreDocument{ID: testDocID}}                       //nolint:exhaustruct
	_, _, _, _, errE := b.TestingWithDocumentHooks(t.Context(), testDocID, nil, fetchOf(t, in)) //nolint:dogsled
	require.Error(t, errE)
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
	require.Error(t, errE)
	assert.EqualError(t, errE, "pre hook failed")
	assert.False(t, fetched, "fetch must not run when a pre-hook fails")
}

func TestWithDocumentHooksDeleted(t *testing.T) {
	t.Parallel()

	sawNil := false
	post := []docPostHook{
		func(_ context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E) (
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
