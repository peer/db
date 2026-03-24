package search

import (
	"math"
	"math/big"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

// queryJSON converts a types.QueryVariant to a compact JSON string for golden comparisons.
func queryJSON(t *testing.T, q types.QueryVariant) string {
	t.Helper()
	data, errE := x.MarshalWithoutEscapeHTML(q.QueryCaster())
	require.NoError(t, errE, "% -+#.1v", errE)
	return string(data)
}

func TestDocumentTextSearchQuery(t *testing.T) {
	t.Parallel()

	t.Run("NonEmpty", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("hello", operator.Or))
		//nolint:lll
		expected := `{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("", operator.Or))
		assert.Equal(t, `{"bool":{}}`, got) //nolint:testifylint
	})

	t.Run("ANDOperator", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, documentTextSearchQuery("hello", operator.And))
		//nolint:lll
		expected := `{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"and","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"and","fields":["claims.ref.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"and","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"and","fields":["claims.html.html.und"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})
}

func TestAmountComputeInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name            string
		From            float64
		To              float64
		WantInterval    float64
		WantUpperBound  float64
		WantIntervalStr string
		WantBins        int
	}{
		{Name: "0_to_100", From: 0, To: 100, WantInterval: 1.0000000000000002, WantUpperBound: 100, WantIntervalStr: "1.0000000000000002", WantBins: 100},
		{Name: "10_to_90", From: 10, To: 90, WantInterval: 0.8000000000000002, WantUpperBound: 90, WantIntervalStr: "0.8000000000000002", WantBins: 100},
		{Name: "0_to_1", From: 0, To: 1, WantInterval: 0.010000000000000002, WantUpperBound: 1, WantIntervalStr: "0.010000000000000002", WantBins: 100},
		{Name: "0_to_1000000", From: 0, To: 1000000, WantInterval: 10000.000000000002, WantUpperBound: 1000000, WantIntervalStr: "10000.000000000002", WantBins: 100},
		{Name: "-100_to_100", From: -100, To: 100, WantInterval: 2.0000000000000004, WantUpperBound: 100, WantIntervalStr: "2.0000000000000004", WantBins: 100},
		{Name: "-1_to_0", From: -1, To: 0, WantInterval: 0.010000000000000002, WantUpperBound: 0, WantIntervalStr: "0.010000000000000002", WantBins: 100},
		{Name: "0.5_to_1.5", From: 0.5, To: 1.5, WantInterval: 0.010000000000000002, WantUpperBound: 1.5, WantIntervalStr: "0.010000000000000002", WantBins: 100},
		{Name: "40_to_60", From: 40, To: 60, WantInterval: 0.20000000000000007, WantUpperBound: 60, WantIntervalStr: "0.20000000000000007", WantBins: 100},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			interval, upperBound, intervalStr := amountComputeInterval(tt.From, tt.To)
			assert.Equal(t, tt.WantInterval, interval)     //nolint:testifylint
			assert.Equal(t, tt.WantUpperBound, upperBound) //nolint:testifylint
			assert.Equal(t, tt.WantIntervalStr, intervalStr)

			// Key invariant: from + histogramBins*interval > to.
			// This ensures "to" falls inside the last bin, not in a 101st bin.
			assert.Greater(t, tt.From+float64(histogramBins)*interval, tt.To)

			// The interval should produce exactly histogramBins buckets:
			// from + (histogramBins-1)*interval <= to.
			assert.LessOrEqual(t, tt.From+float64(histogramBins-1)*interval, tt.To)

			// Verify expected number of bins: floor((to - from) / interval) + 1.
			bins := int(math.Floor((tt.To-tt.From)/interval)) + 1
			assert.Equal(t, tt.WantBins, bins)
		})
	}
}

func TestTimeComputeInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name            string
		From            float64
		To              float64
		WantInterval    float64
		WantUpperBound  float64
		WantIntervalStr string
		WantBins        int
	}{
		{Name: "1000_to_9000", From: 1000, To: 9000, WantInterval: 80, WantUpperBound: 9000, WantIntervalStr: "80", WantBins: 101},
		{Name: "0_to_10000", From: 0, To: 10000, WantInterval: 101, WantUpperBound: 10000, WantIntervalStr: "101", WantBins: 100},
		{Name: "-500_to_500", From: -500, To: 500, WantInterval: 10, WantUpperBound: 500, WantIntervalStr: "10", WantBins: 101},
		{Name: "0_to_100", From: 0, To: 100, WantInterval: 1, WantUpperBound: 100, WantIntervalStr: "1", WantBins: 101},
		{Name: "0_to_99", From: 0, To: 99, WantInterval: 1, WantUpperBound: 99, WantIntervalStr: "1", WantBins: 100},
		{Name: "0_to_1000", From: 0, To: 1000, WantInterval: 10, WantUpperBound: 1000, WantIntervalStr: "10", WantBins: 101},
		{Name: "0_to_10", From: 0, To: 10, WantInterval: 1, WantUpperBound: 10, WantIntervalStr: "1", WantBins: 11},
		{Name: "-1000_to_1000", From: -1000, To: 1000, WantInterval: 20, WantUpperBound: 1000, WantIntervalStr: "20", WantBins: 101},
		{Name: "0_to_1000000", From: 0, To: 1000000, WantInterval: 10101, WantUpperBound: 1000000, WantIntervalStr: "10101", WantBins: 100},
		{Name: "0_to_1", From: 0, To: 1, WantInterval: 1, WantUpperBound: 1, WantIntervalStr: "1", WantBins: 2},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			interval, upperBound, intervalStr := timeComputeInterval(tt.From, tt.To)
			assert.Equal(t, tt.WantInterval, interval)     //nolint:testifylint
			assert.Equal(t, tt.WantUpperBound, upperBound) //nolint:testifylint
			assert.Equal(t, tt.WantIntervalStr, intervalStr)

			// Key invariant: bins >= histogramBins when possible (interval > 1).
			// When interval == 1 and range is small, fewer bins are acceptable.
			bins := int(math.Floor((tt.To-tt.From)/interval)) + 1
			assert.Equal(t, tt.WantBins, bins)
			if interval > 1 {
				assert.GreaterOrEqual(t, bins, histogramBins)
			}

			// Interval must be a positive integer.
			assert.Equal(t, interval, math.Trunc(interval)) //nolint:testifylint
			assert.Greater(t, interval, 0.0)
		})
	}
}

// float64ToBigInt converts a float64 to a big.Int using big.Float for exact comparison.
func float64ToBigInt(f float64) *big.Int {
	bf := new(big.Float).SetFloat64(f)
	bi, _ := bf.Int(nil)
	return bi
}

func TestInt64ToFloat64Floor(t *testing.T) { //nolint:dupl
	t.Parallel()

	tests := []struct {
		Name  string
		Input int64
	}{
		{Name: "zero", Input: 0},
		{Name: "one", Input: 1},
		{Name: "negative", Input: -1},
		{Name: "small", Input: 1000},
		{Name: "max_exact", Input: 1 << 53},         // Largest int64 exactly representable as float64.
		{Name: "max_exact_plus1", Input: 1<<53 + 1}, // Not exactly representable.
		{Name: "large", Input: 1<<53 + 100},         // Not exactly representable.
		{Name: "neg_large", Input: -(1<<53 + 100)},  // Not exactly representable, negative.
		{Name: "max_int64", Input: math.MaxInt64},   // Far beyond float64 precision.
		{Name: "min_int64", Input: math.MinInt64},   // Far beyond float64 precision.
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			f := int64ToFloat64Floor(tt.Input)
			// Use big.Int to compare without lossy int64->float64 conversion.
			// The float64 value (as exact integer) must be <= the original int64.
			inputBig := big.NewInt(tt.Input)
			resultBig := float64ToBigInt(f)
			assert.LessOrEqual(t, resultBig.Cmp(inputBig), 0, "float64(%v) = %v should be <= %v", f, resultBig, inputBig)
		})
	}
}

func TestInt64ToFloat64Ceil(t *testing.T) { //nolint:dupl
	t.Parallel()

	tests := []struct {
		Name  string
		Input int64
	}{
		{Name: "zero", Input: 0},
		{Name: "one", Input: 1},
		{Name: "negative", Input: -1},
		{Name: "small", Input: 1000},
		{Name: "max_exact", Input: 1 << 53},
		{Name: "max_exact_plus1", Input: 1<<53 + 1},
		{Name: "large", Input: 1<<53 + 100},
		{Name: "neg_large", Input: -(1<<53 + 100)},
		{Name: "max_int64", Input: math.MaxInt64},
		{Name: "min_int64", Input: math.MinInt64},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			f := int64ToFloat64Ceil(tt.Input)
			// Use big.Int to compare without lossy int64->float64 conversion.
			// The float64 value (as exact integer) must be >= the original int64.
			inputBig := big.NewInt(tt.Input)
			resultBig := float64ToBigInt(f)
			assert.GreaterOrEqual(t, resultBig.Cmp(inputBig), 0, "float64(%v) = %v should be >= %v", f, resultBig, inputBig)
		})
	}
}

func TestAmountUnitFilter(t *testing.T) {
	t.Parallel()

	t.Run("WithUnit", func(t *testing.T) {
		t.Parallel()
		unit := identifier.From("unit")
		got := queryJSON(t, amountUnitFilter(&unit))
		assert.Equal(t, `{"term":{"claims.amount.unit":{"value":"7xgMSp3wauK811A8Fwk3rY"}}}`, got) //nolint:testifylint
	})

	t.Run("WithoutUnit", func(t *testing.T) {
		t.Parallel()
		got := queryJSON(t, amountUnitFilter(nil))
		assert.Equal(t, `{"bool":{"must_not":[{"exists":{"field":"claims.amount.unit"}}]}}`, got) //nolint:testifylint
	})
}
