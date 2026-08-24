package base_test

import (
	"context"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/internal/testutils"
)

// docFieldContains reports whether the ES document with the given ID matches the term in the given
// full-text field, proving the term is searchable on that document.
func docFieldContains(
	ctx context.Context, t *testing.T, esClient *elasticsearch.TypedClient, index string, docID identifier.Identifier, field, term string,
) bool {
	t.Helper()

	query := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(docID.String())),
		esdsl.NewMatchQuery(field, term),
	)
	res, err := esClient.Search().Index(index).Query(query).Size(1).Do(ctx)
	testutils.RequireNoESError(ctx, t, err)
	return res.Hits.Total.Value > 0
}

// TestStartWithoutDocuments tests that a base starts without any property, class, or language document,
// so that it can run on an ontology of its own, and that documents are then still indexed and searchable.
func TestStartWithoutDocuments(t *testing.T) {
	t.Parallel()

	ctx, b, esClient := initBaseInfra(t, nil)

	onShutdown, errE := b.Start(ctx, nil)
	if onShutdown != nil {
		t.Cleanup(onShutdown)
	}
	require.NoError(t, errE, "% -+#.1v", errE)

	// Without property documents the naming properties are just the core naming property itself, and
	// without language documents nothing maps a language document to a language code.
	assert.Equal(t, []identifier.Identifier{internalCore.NamingPropID}, b.NamingProperties())
	assert.Empty(t, b.LanguageCodes())

	// The document names itself with the core naming property, while its other claim uses a property
	// no document describes.
	doc := newDoc()
	doc.Claims = &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: internalCore.NamingPropID},
				String:    "Zaporedje",
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: identifier.New()},
				String:    "Kremenpecina",
			},
		},
	}
	errE = b.InsertOrReplaceDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	index := internalSearch.LevelIndex(b.IndexPrefix, internalSite.AllVisibilityLevel)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(index).Do(ctx)
		if !testutils.AssertNoESError(ctx, c, err) {
			return
		}
		assert.True(c, testutils.DocExists(ctx, t, esClient, index, doc.ID.String()), "document should be indexed")
	}, 30*time.Second, 100*time.Millisecond)

	// Both claims are searchable, the one using the core naming property and the one using a property
	// nothing describes.
	assert.True(t, docFieldContains(ctx, t, esClient, index, doc.ID, "text.und", "Zaporedje"), "naming claim should be searchable")
	assert.True(t, docFieldContains(ctx, t, esClient, index, doc.ID, "text.und", "Kremenpecina"), "claim of an undescribed property should be searchable")

	// The core naming property is enough for the display label to resolve, through the undetermined
	// language the claim falls back to.
	assert.True(t, docFieldContains(ctx, t, esClient, index, doc.ID, "display.en", "Zaporedje"), "display label should resolve")
}
