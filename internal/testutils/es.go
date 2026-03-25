package testutils

import (
	"context"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

// DocExists checks whether a document with the given ID exists in the given ES index.
func DocExists(ctx context.Context, t *testing.T, esClient *elasticsearch.TypedClient, index, id string) bool {
	t.Helper()
	exists, err := esClient.Exists(index, id).IsSuccess(ctx)
	if err != nil {
		t.Fatalf("unexpected ES error: %v", err)
	}
	return exists
}

// DocHasReference checks if an ES document has a nested reference claim with the given prop and target.
func DocHasReference(ctx context.Context, t *testing.T, esClient *elasticsearch.TypedClient, index string, docID, propID, targetID identifier.Identifier) bool {
	t.Helper()

	nestedQuery := esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(propID.String())),
			esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(targetID.String())),
		),
	).Path("claims.ref")
	query := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(docID.String())),
		nestedQuery,
	)
	res, err := esClient.Search().Index(index).Query(query).Size(1).Do(ctx)
	if err != nil {
		t.Fatalf("ES search error: %v", err)
	}
	return res.Hits.Total.Value > 0
}

// QueryJSON serializes a types.QueryVariant to a JSON string for comparison.
func QueryJSON(t *testing.T, q types.QueryVariant) string {
	t.Helper()
	data, errE := x.MarshalWithoutEscapeHTML(q.QueryCaster())
	require.NoError(t, errE, "% -+#.1v", errE)
	return string(data)
}
