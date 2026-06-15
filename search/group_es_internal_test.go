package search

import (
	"net/http"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// TestGroupAggregationES validates the Go-built grouping aggregation and the response fold end-to-end
// against a live, already-populated ElasticSearch index. It is a development aid (not a CI test): it
// runs only when GROUP_TEST_INDEX names an index with hierarchical ref data, e.g.
//
//	GROUP_TEST_INDEX=razume GROUP_TEST_PROP=NkeB6fpAmk6awysbH77n8H ELASTIC=http://127.0.0.1:9200 \
//	  go test ./search/ -run TestGroupAggregationES -count=1
func TestGroupAggregationES(t *testing.T) {
	t.Parallel()

	index := os.Getenv("GROUP_TEST_INDEX")
	esURL := os.Getenv("ELASTIC")
	if index == "" || esURL == "" {
		t.Skip("GROUP_TEST_INDEX and ELASTIC must be set")
	}
	prop := os.Getenv("GROUP_TEST_PROP")
	if prop == "" {
		prop = "NkeB6fpAmk6awysbH77n8H"
	}
	prop2 := os.Getenv("GROUP_TEST_PROP2")
	lang := "en"

	client, errE := internalSearch.GetClient(http.DefaultClient, zerolog.Nop(), esURL)
	require.NoError(t, errE, "% -+#.1v", errE)

	groupCols := []SortKey{{Type: "ref", Prop: []string{prop}, Group: true}} //nolint:exhaustruct
	if prop2 != "" {
		groupCols = append(groupCols, SortKey{Type: "ref", Prop: []string{prop2}, Group: true}) //nolint:exhaustruct
	}

	res, err := client.Search().Index(index).Size(0).
		AddAggregation(groupAggName, buildGroupAggregation(groupCols, buildSort(nil, lang), lang)).
		Do(t.Context())
	require.NoError(t, err)

	results, errE := foldGroups(res.Aggregations, groupCols)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, results, "expected at least one group")

	var leaves func(rs []Result, depth int) int
	leaves = func(rs []Result, depth int) int {
		n := 0
		for _, r := range rs {
			require.NotEmpty(t, r.ID)
			if r.Group == nil {
				n++
			} else {
				n += leaves(r.Group, depth+1)
			}
		}
		return n
	}
	docs := leaves(results, 0)
	t.Logf("top-level groups=%d total leaf documents=%d", len(results), docs)
	if len(groupCols) == 1 {
		// A single group column places documents directly under their leaf value, so there must be some.
		require.Positive(t, docs, "expected at least one document under the groups")
	}
}
