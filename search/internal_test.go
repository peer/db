package search

import (
	"math"
	"math/big"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/testutils"
)

func TestDocumentTextSearchQuery(t *testing.T) {
	t.Parallel()

	t.Run("NonEmpty", func(t *testing.T) {
		t.Parallel()
		got := testutils.QueryJSON(t, documentTextSearchQuery("hello", operator.Or))
		//nolint:lll
		expected := `{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := testutils.QueryJSON(t, documentTextSearchQuery("", operator.Or))
		assert.Equal(t, `{"bool":{}}`, got) //nolint:testifylint
	})

	t.Run("ANDOperator", func(t *testing.T) {
		t.Parallel()
		got := testutils.QueryJSON(t, documentTextSearchQuery("hello", operator.And))
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
		// Large values where float64 precision matters.
		{Name: "0_to_1e15", From: 0, To: 1e15, WantInterval: 10000000000000.002, WantUpperBound: 1e15, WantIntervalStr: "10000000000000.002", WantBins: 100},
		{Name: "0_to_1e18", From: 0, To: 1e18, WantInterval: 1.0000000000000002e+16, WantUpperBound: 1e18, WantIntervalStr: "10000000000000002", WantBins: 100},
		{Name: "-1e18_to_1e18", From: -1e18, To: 1e18, WantInterval: 2.0000000000000004e+16, WantUpperBound: 1e18, WantIntervalStr: "20000000000000004", WantBins: 100},
		// Tiny range at large magnitude — ULP limits precision.
		{Name: "1e15_to_1e15+1", From: 1e15, To: 1e15 + 1, WantInterval: 0.01125, WantUpperBound: 1e15 + 1, WantIntervalStr: "0.01125", WantBins: 89},
		// Huge float64 value.
		//nolint:lll
		{Name: "0_to_maxfloat64/2", From: 0, To: math.MaxFloat64 / 2, WantInterval: 8.98846567431157972576e+305, WantUpperBound: math.MaxFloat64 / 2, WantIntervalStr: "898846567431158000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", WantBins: 100},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			interval, upperBound, intervalStr := amountComputeInterval(tt.From, tt.To)
			assert.Equal(t, tt.WantInterval, interval)     //nolint:testifylint
			assert.Equal(t, tt.WantUpperBound, upperBound) //nolint:testifylint
			assert.Equal(t, tt.WantIntervalStr, intervalStr)

			// Verify expected number of bins: floor((to - from) / interval) + 1.
			bins := int(math.Floor((tt.To-tt.From)/interval)) + 1
			assert.Equal(t, tt.WantBins, bins)

			// Interval must be a positive number.
			assert.Greater(t, interval, 0.0)
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
		// Large values — still exact integers in float64.
		{Name: "0_to_1e15", From: 0, To: 1e15, WantInterval: 10101010101010, WantUpperBound: 1e15, WantIntervalStr: "10101010101010", WantBins: 100},
		// Beyond 2^53 — float64 can't represent all integers.
		{Name: "0_to_1e18", From: 0, To: 1e18, WantInterval: 1.0101010101010102e+16, WantUpperBound: 1e18, WantIntervalStr: "10101010101010102", WantBins: 99},
		{Name: "-1e18_to_1e18", From: -1e18, To: 1e18, WantInterval: 2.0202020202020204e+16, WantUpperBound: 1e18, WantIntervalStr: "20202020202020204", WantBins: 99},
		// Range beyond 2^53 where individual integers are not distinguishable.
		//nolint:lll
		{Name: "2^53_to_2^53+1M", From: float64(1 << 53), To: float64(1<<53 + 1000000), WantInterval: 10101, WantUpperBound: float64(1<<53 + 1000000), WantIntervalStr: "10101", WantBins: 100},
		// Near MaxInt64 (as float64).
		//nolint:lll
		{Name: "maxint64/2_to_maxint64", From: float64(math.MaxInt64 / 2), To: float64(math.MaxInt64), WantInterval: 4.6582687054822104e+16, WantUpperBound: float64(math.MaxInt64), WantIntervalStr: "46582687054822104", WantBins: 99},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			interval, upperBound, intervalStr := timeComputeInterval(tt.From, tt.To)
			assert.Equal(t, tt.WantInterval, interval)     //nolint:testifylint
			assert.Equal(t, tt.WantUpperBound, upperBound) //nolint:testifylint
			assert.Equal(t, tt.WantIntervalStr, intervalStr)

			// Verify expected number of bins: floor((to - from) / interval) + 1.
			bins := int(math.Floor((tt.To-tt.From)/interval)) + 1
			assert.Equal(t, tt.WantBins, bins)

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
		got := testutils.QueryJSON(t, amountUnitFilter(&unit))
		assert.Equal(t, `{"term":{"claims.amount.unit":{"value":"7xgMSp3wauK811A8Fwk3rY"}}}`, got) //nolint:testifylint
	})

	t.Run("WithoutUnit", func(t *testing.T) {
		t.Parallel()
		got := testutils.QueryJSON(t, amountUnitFilter(nil))
		assert.Equal(t, `{"bool":{"must_not":[{"exists":{"field":"claims.amount.unit"}}]}}`, got) //nolint:testifylint
	})
}
