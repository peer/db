package search

import (
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

// queryJSON converts an elastic.Query to a compact JSON string for golden comparisons.
func queryJSON(t *testing.T, q elastic.Query) string {
	t.Helper()
	src, err := q.Source()
	require.NoError(t, err)
	data, errE := x.Marshal(src)
	require.NoError(t, errE, "% -+#.1v", errE)
	return string(data)
}

func TestDocumentTextSearchQuery(t *testing.T) {
	t.Parallel()

	t.Run("NonEmpty", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("hello", "OR"))
		//nolint:lll
		expected := `{"bool":{"should":[{"term":{"id":"hello"}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("", "OR"))
		assert.Equal(t, `{"bool":{}}`, got) //nolint:testifylint
	})

	t.Run("ANDOperator", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("hello", "AND"))
		//nolint:lll
		expected := `{"bool":{"should":[{"term":{"id":"hello"}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"and","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"and","fields":["claims.ref.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.und"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})
}

func TestAmountUnitFilter(t *testing.T) {
	t.Parallel()

	t.Run("WithUnit", func(t *testing.T) {
		t.Parallel()
		unit := identifier.From("unit")
		got := queryJSON(t, amountUnitFilter(&unit))
		assert.Equal(t, `{"term":{"claims.amount.unit":"7xgMSp3wauK811A8Fwk3rY"}}`, got) //nolint:testifylint
	})

	t.Run("WithoutUnit", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, amountUnitFilter(nil))
		assert.Equal(t, `{"bool":{"must_not":{"exists":{"field":"claims.amount.unit"}}}}`, got) //nolint:testifylint
	})
}
