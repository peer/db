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
		{Document: secretDoc, Metadata: nil, Version: store.Version{}, ParentChangesets: nil},
		{Document: publicDoc, Metadata: nil, Version: store.Version{}, ParentChangesets: nil},
	}

	// With no document post-hooks, the documents are returned unchanged (no per-level filtering).
	b := &base.B{}
	out, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []*document.D{secretDoc, publicDoc}, out)

	// A post-hook that denies the "secret" document at the public level only.
	b = &base.B{}
	b.DocumentPostHooks = []func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E){
		func(
			ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
		) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
			if errE != nil {
				return doc, metadata, version, parentChangesets, errE
			}
			if doc != nil && doc.ID == secret && auth.Visibility(ctx) == "public" {
				return doc, metadata, version, parentChangesets, errors.WithStack(store.ErrAccessDenied)
			}
			return doc, metadata, version, parentChangesets, nil
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

func TestDocumentsForLevelMetadataClone(t *testing.T) {
	t.Parallel()

	id := identifier.New()
	doc := &document.D{CoreDocument: document.CoreDocument{ID: id, Base: []string{"test", id.String()}}}
	meta := &store.DocumentMetadata{At: store.Time{}, Users: []store.User{{ID: "original"}}}
	docs := []base.StartDocument{
		{Document: doc, Metadata: meta, Version: store.Version{}, ParentChangesets: nil},
	}

	// A post-hook that mutates the metadata in place. documentsForLevel must hand the hook an independent copy
	// so the metadata shared across levels by the StartDocument is not corrupted for the other levels.
	b := &base.B{}
	b.DocumentPostHooks = []func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E){
		func(
			_ context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
		) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
			if errE == nil && metadata != nil {
				metadata.Users[0] = store.User{ID: "mutated"}
			}
			return doc, metadata, version, parentChangesets, errE
		},
	}

	_, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The shared metadata is untouched: only the per-level copy the hook received was mutated. A shallow copy
	// would have shared the slice backing array and leaked the mutation back here.
	require.Len(t, meta.Users, 1)
	assert.Equal(t, "original", meta.Users[0].ID)
}

func TestDocumentsForLevelPreHook(t *testing.T) {
	t.Parallel()

	secret := identifier.New()
	public := identifier.New()
	secretDoc := &document.D{CoreDocument: document.CoreDocument{ID: secret, Base: []string{"test", secret.String()}}}
	publicDoc := &document.D{CoreDocument: document.CoreDocument{ID: public, Base: []string{"test", public.String()}}}
	docs := []base.StartDocument{
		{Document: secretDoc, Metadata: nil, Version: store.Version{}, ParentChangesets: nil},
		{Document: publicDoc, Metadata: nil, Version: store.Version{}, ParentChangesets: nil},
	}

	// A pre-hook that denies the "secret" document at the public level only, before any document is fetched.
	b := &base.B{}
	b.DocumentPreHooks = []func(ctx context.Context, id identifier.Identifier, version *store.Version) errors.E{
		func(ctx context.Context, id identifier.Identifier, _ *store.Version) errors.E {
			if id == secret && auth.Visibility(ctx) == "public" {
				return errors.WithStack(store.ErrAccessDenied)
			}
			return nil
		},
	}

	// At the public level the secret document is dropped from the vocabulary by the pre-hook.
	pub, errE := b.TestingDocumentsForLevel(context.Background(), "public", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, pub, 1)
	assert.Equal(t, public, pub[0].ID)

	// At the editor level both documents are present.
	ed, errE := b.TestingDocumentsForLevel(context.Background(), "editor", docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, ed, 2)
}
