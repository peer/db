package search

import (
	"math"
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
		expected := `{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"dis_max":{"queries":[{"simple_query_string":{"default_operator":"or","fields":["text.en"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"or","fields":["text.en"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"or","fields":["text.en.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"or","fields":["text.pt"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"or","fields":["text.pt"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"or","fields":["text.pt.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"or","fields":["text.sl"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"or","fields":["text.sl"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"or","fields":["text.sl.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"or","fields":["text.und"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"or","fields":["text.und"],"query":"hello"}}],"tie_breaker":0.1}},{"nested":{"path":"claims.amount","query":{"simple_query_string":{"default_operator":"or","fields":["claims.amount.propDisplay.en^0.2","claims.amount.propDisplay.pt^0.2","claims.amount.propDisplay.sl^0.2","claims.amount.propDisplay.und^0.2","claims.amount.propNaming.en^0.2","claims.amount.propNaming.pt^0.2","claims.amount.propNaming.sl^0.2","claims.amount.propNaming.und^0.2","claims.amount.fromDisplay^0.2","claims.amount.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.has","query":{"simple_query_string":{"default_operator":"or","fields":["claims.has.propDisplay.en^0.2","claims.has.propDisplay.pt^0.2","claims.has.propDisplay.sl^0.2","claims.has.propDisplay.und^0.2","claims.has.propNaming.en^0.2","claims.has.propNaming.pt^0.2","claims.has.propNaming.sl^0.2","claims.has.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.none","query":{"simple_query_string":{"default_operator":"or","fields":["claims.none.propDisplay.en^0.2","claims.none.propDisplay.pt^0.2","claims.none.propDisplay.sl^0.2","claims.none.propDisplay.und^0.2","claims.none.propNaming.en^0.2","claims.none.propNaming.pt^0.2","claims.none.propNaming.sl^0.2","claims.none.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.propDisplay.en^0.2","claims.ref.propDisplay.pt^0.2","claims.ref.propDisplay.sl^0.2","claims.ref.propDisplay.und^0.2","claims.ref.propNaming.en^0.2","claims.ref.propNaming.pt^0.2","claims.ref.propNaming.sl^0.2","claims.ref.propNaming.und^0.2","claims.ref.toDisplay.en^0.2","claims.ref.toDisplay.pt^0.2","claims.ref.toDisplay.sl^0.2","claims.ref.toDisplay.und^0.2","claims.ref.toNaming.en^0.2","claims.ref.toNaming.pt^0.2","claims.ref.toNaming.sl^0.2","claims.ref.toNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.time","query":{"simple_query_string":{"default_operator":"or","fields":["claims.time.propDisplay.en^0.2","claims.time.propDisplay.pt^0.2","claims.time.propDisplay.sl^0.2","claims.time.propDisplay.und^0.2","claims.time.propNaming.en^0.2","claims.time.propNaming.pt^0.2","claims.time.propNaming.sl^0.2","claims.time.propNaming.und^0.2","claims.time.fromDisplay^0.2","claims.time.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.unknown","query":{"simple_query_string":{"default_operator":"or","fields":["claims.unknown.propDisplay.en^0.2","claims.unknown.propDisplay.pt^0.2","claims.unknown.propDisplay.sl^0.2","claims.unknown.propDisplay.und^0.2","claims.unknown.propNaming.en^0.2","claims.unknown.propNaming.pt^0.2","claims.unknown.propNaming.sl^0.2","claims.unknown.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.subRef","query":{"simple_query_string":{"default_operator":"or","fields":["claims.subRef.propDisplay.en^0.2","claims.subRef.propDisplay.pt^0.2","claims.subRef.propDisplay.sl^0.2","claims.subRef.propDisplay.und^0.2","claims.subRef.propNaming.en^0.2","claims.subRef.propNaming.pt^0.2","claims.subRef.propNaming.sl^0.2","claims.subRef.propNaming.und^0.2","claims.subRef.toDisplay.en^0.2","claims.subRef.toDisplay.pt^0.2","claims.subRef.toDisplay.sl^0.2","claims.subRef.toDisplay.und^0.2","claims.subRef.toNaming.en^0.2","claims.subRef.toNaming.pt^0.2","claims.subRef.toNaming.sl^0.2","claims.subRef.toNaming.und^0.2"],"query":"hello"}}}}]}}`
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
		expected := `{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"dis_max":{"queries":[{"simple_query_string":{"default_operator":"and","fields":["text.en"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"and","fields":["text.en"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"and","fields":["text.en.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"and","fields":["text.pt"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"and","fields":["text.pt"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"and","fields":["text.pt.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"and","fields":["text.sl"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"default_operator":"and","fields":["text.sl"],"query":"hello"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"and","fields":["text.sl.unstemmed"],"query":"hello"}},{"simple_query_string":{"default_operator":"and","fields":["text.und"],"query":"hello","quote_field_suffix":".exact"}},{"simple_query_string":{"analyze_wildcard":true,"default_operator":"and","fields":["text.und"],"query":"hello"}}],"tie_breaker":0.1}},{"nested":{"path":"claims.amount","query":{"simple_query_string":{"default_operator":"and","fields":["claims.amount.propDisplay.en^0.2","claims.amount.propDisplay.pt^0.2","claims.amount.propDisplay.sl^0.2","claims.amount.propDisplay.und^0.2","claims.amount.propNaming.en^0.2","claims.amount.propNaming.pt^0.2","claims.amount.propNaming.sl^0.2","claims.amount.propNaming.und^0.2","claims.amount.fromDisplay^0.2","claims.amount.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.has","query":{"simple_query_string":{"default_operator":"and","fields":["claims.has.propDisplay.en^0.2","claims.has.propDisplay.pt^0.2","claims.has.propDisplay.sl^0.2","claims.has.propDisplay.und^0.2","claims.has.propNaming.en^0.2","claims.has.propNaming.pt^0.2","claims.has.propNaming.sl^0.2","claims.has.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.none","query":{"simple_query_string":{"default_operator":"and","fields":["claims.none.propDisplay.en^0.2","claims.none.propDisplay.pt^0.2","claims.none.propDisplay.sl^0.2","claims.none.propDisplay.und^0.2","claims.none.propNaming.en^0.2","claims.none.propNaming.pt^0.2","claims.none.propNaming.sl^0.2","claims.none.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"and","fields":["claims.ref.propDisplay.en^0.2","claims.ref.propDisplay.pt^0.2","claims.ref.propDisplay.sl^0.2","claims.ref.propDisplay.und^0.2","claims.ref.propNaming.en^0.2","claims.ref.propNaming.pt^0.2","claims.ref.propNaming.sl^0.2","claims.ref.propNaming.und^0.2","claims.ref.toDisplay.en^0.2","claims.ref.toDisplay.pt^0.2","claims.ref.toDisplay.sl^0.2","claims.ref.toDisplay.und^0.2","claims.ref.toNaming.en^0.2","claims.ref.toNaming.pt^0.2","claims.ref.toNaming.sl^0.2","claims.ref.toNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.time","query":{"simple_query_string":{"default_operator":"and","fields":["claims.time.propDisplay.en^0.2","claims.time.propDisplay.pt^0.2","claims.time.propDisplay.sl^0.2","claims.time.propDisplay.und^0.2","claims.time.propNaming.en^0.2","claims.time.propNaming.pt^0.2","claims.time.propNaming.sl^0.2","claims.time.propNaming.und^0.2","claims.time.fromDisplay^0.2","claims.time.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.unknown","query":{"simple_query_string":{"default_operator":"and","fields":["claims.unknown.propDisplay.en^0.2","claims.unknown.propDisplay.pt^0.2","claims.unknown.propDisplay.sl^0.2","claims.unknown.propDisplay.und^0.2","claims.unknown.propNaming.en^0.2","claims.unknown.propNaming.pt^0.2","claims.unknown.propNaming.sl^0.2","claims.unknown.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.subRef","query":{"simple_query_string":{"default_operator":"and","fields":["claims.subRef.propDisplay.en^0.2","claims.subRef.propDisplay.pt^0.2","claims.subRef.propDisplay.sl^0.2","claims.subRef.propDisplay.und^0.2","claims.subRef.propNaming.en^0.2","claims.subRef.propNaming.pt^0.2","claims.subRef.propNaming.sl^0.2","claims.subRef.propNaming.und^0.2","claims.subRef.toDisplay.en^0.2","claims.subRef.toDisplay.pt^0.2","claims.subRef.toDisplay.sl^0.2","claims.subRef.toDisplay.und^0.2","claims.subRef.toNaming.en^0.2","claims.subRef.toNaming.pt^0.2","claims.subRef.toNaming.sl^0.2","claims.subRef.toNaming.und^0.2"],"query":"hello"}}}}]}}`
		assert.Equal(t, expected, got)
	})
}

func TestComputeInterval(t *testing.T) {
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
		{Name: "0_to_1000", From: 0, To: 1000, WantInterval: 10.000000000000002, WantUpperBound: 1000, WantIntervalStr: "10.000000000000002", WantBins: 100},
		{Name: "-500_to_500", From: -500, To: 500, WantInterval: 10.000000000000002, WantUpperBound: 500, WantIntervalStr: "10.000000000000002", WantBins: 100},
		{Name: "0_to_10000", From: 0, To: 10000, WantInterval: 100.00000000000001, WantUpperBound: 10000, WantIntervalStr: "100.00000000000001", WantBins: 100},
		{Name: "-1000_to_1000", From: -1000, To: 1000, WantInterval: 20.000000000000004, WantUpperBound: 1000, WantIntervalStr: "20.000000000000004", WantBins: 100},
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
		// Tiny range at large magnitude - ULP limits precision.
		{Name: "1e15_to_1e15+1", From: 1e15, To: 1e15 + 1, WantInterval: 0.01125, WantUpperBound: 1e15 + 1, WantIntervalStr: "0.01125", WantBins: 89},
		// Huge float64 value.
		//nolint:lll
		{Name: "0_to_maxfloat64/2", From: 0, To: math.MaxFloat64 / 2, WantInterval: 8.98846567431157972576e+305, WantUpperBound: math.MaxFloat64 / 2, WantIntervalStr: "898846567431158000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", WantBins: 100},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			interval, upperBound, intervalStr := computeInterval(tt.From, tt.To)
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
