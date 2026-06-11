package search_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/search"
)

// assertIntervalPrefix checks that the interval metadata string starts with the expected prefix.
func assertIntervalPrefix(t *testing.T, expected string, metadata map[string]any) {
	t.Helper()
	interval, ok := metadata["interval"].(string)
	require.True(t, ok, "interval metadata should be a string")
	assert.True(t, strings.HasPrefix(interval, expected), "interval %q should start with %q", interval, expected)
}

func TestAmountFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	ten := 10.0
	fifty := 50.0
	ninety := 90.0

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"amountDoc1", &ten},
		{"amountDoc2", &fifty},
		{"amountDoc3", &ninety},
	} {
		indexAmountDoc(t, ctx, esClient, index, tc.id, amountProp, unitID, tc.value)
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session with an amount filter.
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The histogram spans the known endpoint window edges [9.5, 90.5).
	assert.Equal(t, "9.5", metadata["from"])
	assert.Equal(t, "90.5", metadata["to"])
	assertIntervalPrefix(t, "0.8", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Each precision window is wider than a bucket so every value is counted in two buckets.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(6), totalCount)

	// Verify the non-zero buckets.
	// Window [9.5, 10.5) -> buckets [0] and [1].
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[1].Count)
	// Window [49.5, 50.5) -> buckets [49] and [50].
	assert.Equal(t, int64(1), results[49].Count)
	assert.Equal(t, int64(1), results[50].Count)
	// Window [89.5, 90.5) -> buckets [98] and [99].
	assert.Equal(t, int64(1), results[98].Count)
	assert.Equal(t, int64(1), results[99].Count)
}

func TestAmountFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	ten := 10.0

	// Doc with the amount prop.
	indexAmountDoc(t, ctx, esClient, index, "amountDoc1", amountProp, unitID, &ten)
	// Doc without the amount prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("amountDoc2"),
		Display:     nil,
		Text:        nil,
		Time:        nil,
		LastUpdated: nil,
		Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
	})
	// Another doc without the amount prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("amountDoc3"),
		Display:     nil,
		Text:        nil,
		Time:        nil,
		LastUpdated: nil,
		Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.AmountFilter{Unit: &unitID} //nolint:exhaustruct
	_, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), amountProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Missing count should be 2 (two documents without the amount prop).
	assert.Equal(t, int64(2), metadata["missing"])
}

func TestAmountFilterGetNoMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	ten := 10.0

	// All docs have the amount prop.
	indexAmountDoc(t, ctx, esClient, index, "amountDoc1", amountProp, unitID, &ten)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.AmountFilter{Unit: &unitID} //nolint:exhaustruct
	_, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), amountProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing documents.
	assert.Equal(t, int64(0), metadata["missing"])
}

func TestAmountFilterGetInactiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	ten := 10.0
	fifty := 50.0
	ninety := 90.0

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"amountDoc1", &ten},
		{"amountDoc2", &fifty},
		{"amountDoc3", &ninety},
	} {
		indexAmountDoc(t, ctx, esClient, index, tc.id, amountProp, unitID, tc.value)
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for amount histogram using the session's full query, prop and unit from outside the session.
	f := search.AmountFilter{Unit: &unitID} //nolint:exhaustruct
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), amountProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "9.5", metadata["from"])
	assert.Equal(t, "90.5", metadata["to"])
	assertIntervalPrefix(t, "0.8", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify total count across all histogram bins equals 6 (each precision window is
	// wider than a bucket so every value is counted in two buckets).
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(6), totalCount)
}

func TestAmountFilterGetSameValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")
	fortyTwo := 42.0

	// Two open-ended claims with the same known endpoint: the only known endpoint value is
	// 42, so the histogram collapses to a single bucket.
	indexAmountIntervalDoc(t, ctx, esClient, index, "sameDoc0", amountProp, &unitID, &fortyTwo, nil)
	indexAmountIntervalDoc(t, ctx, esClient, index, "sameDoc1", amountProp, &unitID, &fortyTwo, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// All values the same -> single bucket.
	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "42", metadata["from"])
	assert.Equal(t, "42", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 42.0, Count: 2}}, results)
}

func TestAmountFilterGetEmptyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	_, getSearchService, _ := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HistogramResult{}, results)
	assert.Equal(t, 0, metadata["total"])
}

func TestAmountFilterGetWithoutUnitIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	twentyFive := 25.0

	// An open-ended claim without a unit: the only known endpoint value is 25, so the
	// histogram collapses to a single bucket.
	indexAmountIntervalDoc(t, ctx, esClient, index, "noUnitDoc", amountProp, nil, &twentyFive, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "25", metadata["from"])
	assert.Equal(t, "25", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 25.0, Count: 1}}, results)
}

func TestAmountFilterGetGapIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	// Two values very close together (0 and 1) and one far away (100).
	// This creates a large gap in the middle with empty buckets.
	zero := 0.0
	one := 1.0
	hundred := 100.0

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"gapDoc1", &zero},
		{"gapDoc2", &one},
		{"gapDoc3", &hundred},
	} {
		indexAmountDoc(t, ctx, esClient, index, tc.id, amountProp, unitID, tc.value)
	}
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The histogram spans the known endpoint window edges [-0.5, 100.5).
	assert.Equal(t, "-0.5", metadata["from"])
	assert.Equal(t, "100.5", metadata["to"])
	assertIntervalPrefix(t, "1.01", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Total count = 4 (the window [0.5, 1.5) straddles buckets [0] and [1]).
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(4), totalCount)

	// Window [-0.5, 0.5) falls in bucket [0], window [0.5, 1.5) straddles buckets [0] and [1].
	assert.InDelta(t, -0.5, results[0].From, 1e-10)
	assert.Equal(t, int64(2), results[0].Count)
	assert.InDelta(t, 0.51, results[1].From, 1e-10)
	assert.Equal(t, int64(1), results[1].Count)

	// All buckets from index 2 to 98 should be empty (the gap).
	for i := 2; i < 99; i++ {
		assert.Equal(t, int64(0), results[i].Count, "bucket %d should be empty", i)
	}

	// Window [99.5, 100.5) falls in bucket [99].
	assert.InDelta(t, 99.49, results[99].From, 1e-10)
	assert.Equal(t, int64(1), results[99].Count)
}

func TestAmountFilterGetExtendedBoundsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	// Two values at 40 and 60. Session filter range [0, 100] is wider than data,
	// so the histogram should extend to cover the full session range.
	forty := 40.0
	sixty := 60.0

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"extDoc1", &forty},
		{"extDoc2", &sixty},
	} {
		indexAmountDoc(t, ctx, esClient, index, tc.id, amountProp, unitID, tc.value)
	}
	refreshIndex(t, ctx, esClient, index)

	// Session filter with wider range [0, 100] than data [40, 60].
	gte := 0.0
	lte := 100.0
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit: &unitID, Gte: &gte, Lte: &lte, Missing: false,
			},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// Histogram uses session bounds [0, 100], not data bounds [40, 60].
	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "100", metadata["to"])
	assertIntervalPrefix(t, "1.000000000000000", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify all 100 buckets: From values increase by ~1.0 from 0, counts are 0 except
	// the buckets straddled by the windows [39.5, 40.5) and [59.5, 60.5).
	for i, r := range results {
		assert.InDelta(t, float64(i), r.From, 1e-10, "bucket %d From", i)
		switch i {
		case 39, 40:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 40)", i)
		case 59, 60:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 60)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
}

func TestAmountFilterGetHardBoundsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	// Two amount intervals: [0, 20) and [80, 100), with the to values being window ends.
	zero := 0.0
	twenty := 20.0
	eighty := 80.0
	hundred := 100.0

	indexAmountIntervalDoc(t, ctx, esClient, index, "hardDoc1", amountProp, &unitID, &zero, &twenty)
	indexAmountIntervalDoc(t, ctx, esClient, index, "hardDoc2", amountProp, &unitID, &eighty, &hundred)
	refreshIndex(t, ctx, esClient, index)

	// Search session filters amounts between 10 and 90.
	// Both documents match because their ranges overlap [10, 90].
	gte := 10.0
	lte := 90.0
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit: &unitID, Gte: &gte, Lte: &lte, Missing: false,
			},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The session filter provides bounds [10, 90], so the histogram uses those
	// instead of the data range [0, 100].
	assert.Equal(t, "10", metadata["from"])
	assert.Equal(t, "90", metadata["to"])

	// With hard_bounds, the histogram is clipped to [10, 90] and has exactly 100 buckets.
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Doc [0,20] overlaps bins 0-12 (From 10 to ~19.6), doc [80,100] overlaps bins 87-99 (From ~79.6 to ~89.2).
	// Total count = 13 + 13 = 26.
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 10.0+float64(i)*0.8, r.From, 0.1, "bucket %d From", i)
		totalCount += r.Count
		switch {
		case i <= 12:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (from doc [0,20])", i)
		case i >= 87:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (from doc [80,100])", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(26), totalCount)
}

func TestAmountFilterGetWideRangeIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")

	// Doc1: point value at 5.
	five := 5.0
	// Doc2: wide range [20, 80] - spans many histogram bins.
	twenty := 20.0
	eighty := 80.0
	// Doc3: point value at 95.
	ninetyFive := 95.0

	indexAmountDoc(t, ctx, esClient, index, "wideDoc1", amountProp, unitID, &five)
	indexAmountIntervalDoc(t, ctx, esClient, index, "wideDoc2", amountProp, &unitID, &twenty, &eighty)
	indexAmountDoc(t, ctx, esClient, index, "wideDoc3", amountProp, unitID, &ninetyFive)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The histogram spans the known endpoint window edges [4.5, 95.5).
	assert.Equal(t, "4.5", metadata["from"])
	assert.Equal(t, "95.5", metadata["to"])
	assertIntervalPrefix(t, "0.9", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// The wide-range document [20, 80) is counted in every bucket it overlaps.
	// Point window [4.5, 5.5) straddles buckets [0] and [1], point window [94.5, 95.5)
	// straddles buckets [98] and [99]. Wide range [20, 80) overlaps 66 buckets in the
	// middle. Total count = 70 (2 + 66 range buckets + 2).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 4.5+float64(i)*0.91, r.From, 0.1, "bucket %d From", i)
		totalCount += r.Count
	}
	assert.Equal(t, int64(70), totalCount)

	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[1].Count)
	assert.Equal(t, int64(1), results[98].Count)
	assert.Equal(t, int64(1), results[99].Count)
}

// An open-start claim combined with a bounded claim: the combined min comes from the
// open-start claim's to value 10, so the histogram start is lowered by one step of its
// apparent decimal precision (ten) and the document is counted in the first bucket.
func TestAmountFilterGetOpenStartWithBoundedIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")
	ten := 10.0
	thirty := 30.0
	fifty := 50.0

	indexAmountIntervalDoc(t, ctx, esClient, index, "openStartDoc1", amountProp, &unitID, nil, &ten)
	indexAmountIntervalDoc(t, ctx, esClient, index, "openStartDoc2", amountProp, &unitID, &thirty, &fifty)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.AmountFilter{Unit: &unitID} //nolint:exhaustruct
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), amountProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "100", metadata["total"])
	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "50", metadata["to"])
	assertIntervalPrefix(t, "0.5", metadata)
	require.Len(t, results, 100)

	// The open-start document overlaps the buckets below 10 and the bounded document
	// overlaps the buckets spanning [30, 50).
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
	assert.Equal(t, int64(61), totalCount)
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
			interval, upperBound, intervalStr := search.TestingComputeInterval(tt.From, tt.To)
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
		got := testutils.QueryJSON(t, search.TestingAmountUnitFilter(&unit))
		assert.Equal(t, `{"term":{"claims.amount.unit":{"value":"7xgMSp3wauK811A8Fwk3rY"}}}`, got) //nolint:testifylint
	})

	t.Run("WithoutUnit", func(t *testing.T) {
		t.Parallel()
		got := testutils.QueryJSON(t, search.TestingAmountUnitFilter(nil))
		assert.Equal(t, `{"bool":{"must_not":[{"exists":{"field":"claims.amount.unit"}}]}}`, got) //nolint:testifylint
	})
}
