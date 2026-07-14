package base_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

func TestDocumentsForLevel(t *testing.T) {
	t.Parallel()

	secret := identifier.New()
	public := identifier.New()
	secretDoc := &document.D{CoreDocument: document.CoreDocument{ID: secret, Base: []string{"test", secret.String()}}}
	publicDoc := &document.D{CoreDocument: document.CoreDocument{ID: public, Base: []string{"test", public.String()}}}
	docs := []base.StartDocument{
		{Document: secretDoc, Metadata: nil},
		{Document: publicDoc, Metadata: nil},
	}

	// With no indexing normalize hooks, the documents are returned unchanged (no per-level shaping).
	b := &base.B{}
	out, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []*document.D{secretDoc, publicDoc}, out)

	// A normalize hook that denies the "secret" document at the public level only.
	b = &base.B{}
	b.IndexingNormalizeHooks = []func(ctx context.Context, doc *document.D, metadata *store.DocumentMetadata) (*document.D, errors.E){
		func(ctx context.Context, doc *document.D, _ *store.DocumentMetadata) (*document.D, errors.E) {
			if doc.ID == secret && auth.Visibility(ctx) == "public" {
				return doc, errors.WithStack(store.ErrAccessDenied)
			}
			return doc, nil
		},
	}

	// At the public level the secret document is dropped from the vocabulary.
	pub, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, pub, 1)
	assert.Equal(t, public, pub[0].ID)

	// At the editor level both documents are present.
	ed, errE := b.TestingDocumentsForLevel(context.Background(), "editor", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, ed, 2)
}

func TestDocumentsForLevelDocumentClone(t *testing.T) {
	t.Parallel()

	id := identifier.New()
	doc := &document.D{CoreDocument: document.CoreDocument{ID: id, Base: []string{"test", id.String()}}}
	meta := &store.DocumentMetadata{At: store.Time{}, Users: []store.User{{ID: "original"}}}
	docs := []base.StartDocument{
		{Document: doc, Metadata: meta},
	}

	// A normalize hook that mutates the document in place. documentsForLevel must hand the hook an
	// independent copy so the document shared across levels is not corrupted for the other levels. The
	// metadata is passed through as-is (hooks must not mutate it).
	b := &base.B{}
	var seenMetadata *store.DocumentMetadata
	b.IndexingNormalizeHooks = []func(ctx context.Context, doc *document.D, metadata *store.DocumentMetadata) (*document.D, errors.E){
		func(_ context.Context, doc *document.D, metadata *store.DocumentMetadata) (*document.D, errors.E) {
			seenMetadata = metadata
			doc.Base[0] = "mutated"
			return doc, nil
		},
	}

	out, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, out, 1)
	assert.Equal(t, "mutated", out[0].Base[0])

	// The hook received the metadata the document was read with.
	assert.Same(t, meta, seenMetadata)

	// The shared document is untouched: only the per-level copy the hook received was mutated. Reusing
	// the input document would have shared the slice backing array and leaked the mutation back here.
	assert.Equal(t, "test", doc.Base[0])
}
