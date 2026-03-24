package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestTimeFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	t1000 := int64(1000)
	t5000 := int64(5000)
	t9000 := int64(9000)

	for _, tc := range []struct {
		id    string
		value *int64
	}{
		{"timeDoc1", &t1000},
		{"timeDoc2", &t5000},
		{"timeDoc3", &t9000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID: identifier.From(tc.id),
			Claims: internalSearch.ClaimTypes{
				Identifier: nil, String: nil, HTML: nil, Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeInt{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)

	// Histogram: interval = 80 (largest integer giving >= 100 bins), 101 buckets.
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9000", metadata["to"])
	assert.Equal(t, "80", metadata["interval"])
	assert.Equal(t, "101", metadata["total"])
	require.Len(t, results, 101)

	// Verify all 101 buckets: From values are exact integers at 1000 + i*80.
	// Value 1000 -> bucket[0], value 5000 -> bucket[50], value 9000 -> bucket[100].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 1000.0+float64(i)*80.0, r.From, 1e-10, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 0:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 1000)", i)
		case 50:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 5000)", i)
		case 100:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 9000)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(3), totalCount)
}

func TestTimeFilterGetSameValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t5000 := int64(5000)

	for i := range 2 {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID: identifier.From("sameTimeDoc", string(rune('0'+i))),
			Claims: internalSearch.ClaimTypes{
				Identifier: nil, String: nil, HTML: nil, Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeInt{
						GreaterThan: nil, GreaterThanOrEqual: &t5000, LessThan: nil, LessThanOrEqual: &t5000,
					},
					From: &t5000, FromDisplay: "", To: &t5000, ToDisplay: "",
				}},
				Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)
	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "5000", metadata["from"])
	assert.Equal(t, "5000", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 5000.0, Count: 2}}, results)
}

func TestTimeFilterGetNegativeValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	tNeg500 := int64(-500)
	t500 := int64(500)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("negTimeDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &tNeg500, LessThan: nil, LessThanOrEqual: &tNeg500,
				},
				From: &tNeg500, FromDisplay: "", To: &tNeg500, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("negTimeDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t500, LessThan: nil, LessThanOrEqual: &t500,
				},
				From: &t500, FromDisplay: "", To: &t500, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)

	// Histogram: interval = 10 (largest integer giving >= 100 bins), 101 buckets.
	assert.Equal(t, "-500", metadata["from"])
	assert.Equal(t, "500", metadata["to"])
	assert.Equal(t, "10", metadata["interval"])
	assert.Equal(t, "101", metadata["total"])
	require.Len(t, results, 101)

	// Verify all 101 buckets: value -500 in bucket [0], value 500 in bucket [100].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, -500.0+float64(i)*10.0, r.From, 1e-10, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 0:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value -500)", i)
		case 100:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 500)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(2), totalCount)
}

func TestTimeFilterGetEmptyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	_, getSearchService, _ := initES(t)

	timeProp := identifier.From("timeProp")

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)
	assert.Equal(t, []search.HistogramResult{}, results)
	assert.Equal(t, 0, metadata["total"])
}

func TestTimeFilterGetExtendedBoundsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	// Two values at 4000 and 6000. Session filter range [0, 10000] is wider than data,
	// so the histogram should extend to cover the full session range.
	t4000 := int64(4000)
	t6000 := int64(6000)

	for _, tc := range []struct {
		id    string
		value *int64
	}{
		{"extTimeDoc1", &t4000},
		{"extTimeDoc2", &t6000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID: identifier.From(tc.id),
			Claims: internalSearch.ClaimTypes{
				Identifier: nil, String: nil, HTML: nil, Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeInt{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Session filter with wider range [0, 10000] than data [4000, 6000].
	gte := int64(0)
	lte := int64(10000)
	session := &search.Session{
		ID: nil, Version: 0, View: "", Query: "",
		Filters: &search.Filters{
			And: nil, Or: nil, Not: nil, Rel: nil, Amount: nil,
			Time: &search.TimeFilter{Prop: timeProp, Gte: &gte, Lte: &lte, None: false},
		},
	}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)

	// Histogram uses session bounds [0, 10000]. interval = 101, 100 buckets.
	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "10000", metadata["to"])
	assert.Equal(t, "101", metadata["interval"])
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify all 100 buckets: exact integer From values at i*101.
	// Value 4000 -> bucket[39] (From=3939), value 6000 -> bucket[59] (From=5959).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, float64(i)*101.0, r.From, 1e-10, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 39:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 4000)", i)
		case 59:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 6000)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(2), totalCount)
}

func TestTimeFilterGetHardBoundsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	// Two time intervals: [0, 2000] and [8000, 10000].
	t0 := int64(0)
	t2000 := int64(2000)
	t8000 := int64(8000)
	t10000 := int64(10000)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("hardTimeDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t0, LessThan: nil, LessThanOrEqual: &t2000,
				},
				From: &t0, FromDisplay: "", To: &t2000, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("hardTimeDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t8000, LessThan: nil, LessThanOrEqual: &t10000,
				},
				From: &t8000, FromDisplay: "", To: &t10000, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Search session filters time between 1000 and 9000.
	// Both documents match because their ranges overlap [1000, 9000].
	gte := int64(1000)
	lte := int64(9000)
	session := &search.Session{
		ID: nil, Version: 0, View: "", Query: "",
		Filters: &search.Filters{
			And: nil, Or: nil, Not: nil, Rel: nil, Amount: nil,
			Time: &search.TimeFilter{Prop: timeProp, Gte: &gte, Lte: &lte, None: false},
		},
	}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)

	// The session filter provides bounds [1000, 9000], so the histogram uses those.
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9000", metadata["to"])

	// With hard_bounds, the histogram is clipped to [1000, 9000].
	assert.Equal(t, "80", metadata["interval"])
	assert.Equal(t, "101", metadata["total"])
	require.Len(t, results, 101)

	// Doc [0,2000] overlaps bins 0-12 (From 1000 to 1960), doc [8000,10000] overlaps bins 87-100 (From 7960 to 9000).
	// Total count = 13 + 14 = 27.
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 1000.0+float64(i)*80.0, r.From, 1e-10, "bucket %d From", i)
		totalCount += r.Count
		switch {
		case i <= 12:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (from doc [0,2000])", i)
		case i >= 87:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (from doc [8000,10000])", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(27), totalCount)
}

func TestTimeFilterGetWideRangeIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	// Doc1: point value at 500.
	t500 := int64(500)
	// Doc2: wide range [2000, 8000] — spans many histogram bins.
	t2000 := int64(2000)
	t8000 := int64(8000)
	// Doc3: point value at 9500.
	t9500 := int64(9500)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("wideTimeDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t500, LessThan: nil, LessThanOrEqual: &t500,
				},
				From: &t500, FromDisplay: "", To: &t500, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("wideTimeDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t2000, LessThan: nil, LessThanOrEqual: &t8000,
				},
				From: &t2000, FromDisplay: "", To: &t8000, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("wideTimeDoc3"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeInt{
					GreaterThan: nil, GreaterThanOrEqual: &t9500, LessThan: nil, LessThanOrEqual: &t9500,
				},
				From: &t9500, FromDisplay: "", To: &t9500, ToDisplay: "",
			}},
			Reference: nil, Relation: nil, Has: nil, None: nil, Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.TimeFilterGet(ctx, getSearchService, *session.ID, timeProp)
	require.NoError(t, errE)

	assert.Equal(t, "500", metadata["from"])
	assert.Equal(t, "9500", metadata["to"])
	assert.Equal(t, "90", metadata["interval"])

	// The wide-range document [2000, 8000] is counted in every bucket it overlaps.
	// Point value 500 -> bucket[0] (count 1), point value 9500 -> last bucket (count 1).
	// Wide range [2000, 8000] overlaps many buckets in the middle.
	// Total count = 70 (1 + 68 range buckets + 1).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 500.0+float64(i)*90.0, r.From, 1e-10, "bucket %d From", i)
		totalCount += r.Count
	}
	assert.Equal(t, int64(70), totalCount)

	// First and last buckets have the point values.
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
}
