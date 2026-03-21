package base_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/search"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

func initBase(t *testing.T, properties []*document.D) (context.Context, *base.B) {
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

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(_ context.Context) (string, string) {
		return schema, "test"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpool.Close)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	esClient, errE := search.GetClient(cleanhttp.DefaultPooledClient(), zerolog.Nop(), os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	index := schema

	t.Cleanup(func() {
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		require.NoError(t, err)
	})

	errE = search.EnsureIndex(ctx, esClient, index)
	require.NoError(t, errE, "% -+#.1v", errE)

	riverClient, workers, errE := internal.NewRiver(ctx, logger, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internal.NewListener(dbpool)

	b := &base.B{
		Schema: schema,
		Index:  index,
	}
	errE = b.Init(ctx, dbpool, listener, esClient, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := riverClient.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		<-riverClient.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.Start(ctx, properties, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, b
}

// Well-known IDs for inverse relation tests.
//
//nolint:gochecknoglobals
var (
	testInstanceOfPropID        = identifier.From(core.Namespace, "INSTANCE_OF")
	testPropertyClassID         = identifier.From(core.Namespace, "PROPERTY")
	testInversePropertyOfPropID = identifier.From(core.Namespace, "INVERSE_PROPERTY_OF")
)

func makePropertyDoc(t *testing.T, id identifier.Identifier, inverseOf *identifier.Identifier) *document.D {
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
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

func TestInsertOrReplaceDocument(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t, nil)

	docID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
	}

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
	propX := identifier.New()
	propY := identifier.New()
	propXDoc := makePropertyDoc(t, propX, &propY)
	propYDoc := makePropertyDoc(t, propY, nil)

	ctx, b := initBase(t, []*document.D{propXDoc, propYDoc})

	// Insert property documents into the store.
	errE := b.InsertOrReplaceDocument(ctx, propXDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = b.InsertOrReplaceDocument(ctx, propYDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert docB (target) and docA with relation A --X--> B.
	docA := identifier.New()
	docB := identifier.New()

	errE = b.InsertOrReplaceDocument(ctx, &document.D{
		CoreDocument: document.CoreDocument{ID: docB}, //nolint:exhaustruct
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.InsertOrReplaceDocument(ctx, &document.D{
		CoreDocument: document.CoreDocument{ID: docA}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Relation: []document.RelationClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: propX},
					To:        document.Reference{ID: docB},
				},
			},
		},
	})
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
	}, 10*time.Second, 100*time.Millisecond)

	// Now replace docB. Inverse relations should be carried over.
	errE = b.InsertOrReplaceDocument(ctx, &document.D{
		CoreDocument: document.CoreDocument{ID: docB}, //nolint:exhaustruct
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

	ctx, b := initBase(t, nil)

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
