package search_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
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
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Time:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: internalSearch.AmountClaims{{
					Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session with an amount filter.
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "10", metadata["from"])
	assert.Equal(t, "90", metadata["to"])
	assertIntervalPrefix(t, "0.8", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify total count across all histogram bins equals 3.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(3), totalCount)

	// Verify the three non-zero buckets.
	// Value 10 -> bucket [0].
	assert.InDelta(t, 10.0, results[0].From, 1e-10)
	assert.Equal(t, int64(1), results[0].Count)
	// Value 50 -> bucket [49].
	assert.InDelta(t, 49.2, results[49].From, 1e-10)
	assert.Equal(t, int64(1), results[49].Count)
	// Value 90 -> bucket [99].
	assert.InDelta(t, 89.2, results[99].From, 1e-10)
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
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("amountDoc1"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &ten, LessThan: nil, LessThanOrEqual: &ten,
				},
				From: &ten, FromDisplay: "", To: &ten, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	// Doc without the amount prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("amountDoc2"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time:   nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	// Another doc without the amount prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("amountDoc3"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time:   nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
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
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("amountDoc1"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &ten, LessThan: nil, LessThanOrEqual: &ten,
				},
				From: &ten, FromDisplay: "", To: &ten, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
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
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Time:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: internalSearch.AmountClaims{{
					Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for amount histogram using the session's full query, prop and unit from outside the session.
	f := search.AmountFilter{Unit: &unitID} //nolint:exhaustruct
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), amountProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "10", metadata["from"])
	assert.Equal(t, "90", metadata["to"])
	assertIntervalPrefix(t, "0.8", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify total count across all histogram bins equals 3.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(3), totalCount)
}

func TestAmountFilterGetSameValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unit")
	fortyTwo := 42.0

	for i := range 2 {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From("sameDoc", string(rune('0'+i))),
			Display: nil,
			Text:    nil,
			Time:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: internalSearch.AmountClaims{{
					Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: &fortyTwo, LessThan: nil, LessThanOrEqual: &fortyTwo,
					},
					From: &fortyTwo, FromDisplay: "", To: &fortyTwo, ToDisplay: "",
				}},
				Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
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
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
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

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("noUnitDoc"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &twentyFive, LessThan: nil, LessThanOrEqual: &twentyFive,
				},
				From: &twentyFive, FromDisplay: "", To: &twentyFive, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
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
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Time:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: internalSearch.AmountClaims{{
					Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "100", metadata["to"])
	assertIntervalPrefix(t, "1.000000000000000", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Total count = 3.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(3), totalCount)

	// Values 0 and 1 both fall in bucket [0] because interval > 1.
	assert.InDelta(t, 0.0, results[0].From, 1e-10)
	assert.Equal(t, int64(2), results[0].Count)

	// All buckets from index 1 to 98 should be empty (the gap).
	for i := 1; i < 99; i++ {
		assert.Equal(t, int64(0), results[i].Count, "bucket %d should be empty", i)
	}

	// Value 100 falls in bucket [99].
	assert.InDelta(t, 99.0, results[99].From, 1e-10)
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
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Time:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: internalSearch.AmountClaims{{
					Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Session filter with wider range [0, 100] than data [40, 60].
	gte := 0.0
	lte := 100.0
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit: &unitID, Gte: &gte, Lte: &lte, Missing: false,
			},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// Histogram uses session bounds [0, 100], not data bounds [40, 60].
	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "100", metadata["to"])
	assertIntervalPrefix(t, "1.000000000000000", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify all 100 buckets: From values increase by ~1.0 from 0, counts are 0
	// except bucket [39] (value 40) and [59] (value 60).
	for i, r := range results {
		assert.InDelta(t, float64(i), r.From, 1e-10, "bucket %d From", i)
		switch i {
		case 39:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 40)", i)
		case 59:
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

	// Two amount intervals: [0, 20] and [80, 100].
	zero := 0.0
	twenty := 20.0
	eighty := 80.0
	hundred := 100.0

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("hardDoc1"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &zero, LessThan: nil, LessThanOrEqual: &twenty,
				},
				From: &zero, FromDisplay: "", To: &twenty, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("hardDoc2"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &eighty, LessThan: nil, LessThanOrEqual: &hundred,
				},
				From: &eighty, FromDisplay: "", To: &hundred, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Search session filters amounts between 10 and 90.
	// Both documents match because their ranges overlap [10, 90].
	gte := 10.0
	lte := 90.0
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit: &unitID, Gte: &gte, Lte: &lte, Missing: false,
			},
		}},
		Reverse: nil,
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

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideDoc1"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &five, LessThan: nil, LessThanOrEqual: &five,
				},
				From: &five, FromDisplay: "", To: &five, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideDoc2"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &twenty, LessThan: nil, LessThanOrEqual: &eighty,
				},
				From: &twenty, FromDisplay: "", To: &eighty, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideDoc3"),
		Display: nil,
		Text:    nil,
		Time:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &ninetyFive, LessThan: nil, LessThanOrEqual: &ninetyFive,
				},
				From: &ninetyFive, FromDisplay: "", To: &ninetyFive, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Amount.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "5", metadata["from"])
	assert.Equal(t, "95", metadata["to"])
	assertIntervalPrefix(t, "0.9", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// The wide-range document [20, 80] is counted in every bucket it overlaps.
	// Point value 5 -> bucket[0] (count 1), point value 95 -> bucket[99] (count 1).
	// Wide range [20, 80] overlaps many buckets in the middle (count 1 each).
	// Total count = 70 (1 + 68 range buckets + 1).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 5.0+float64(i)*0.9, r.From, 0.1, "bucket %d From", i)
		totalCount += r.Count
	}
	assert.Equal(t, int64(70), totalCount)

	// First and last buckets have the point values.
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[99].Count)
}
