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

	t1000 := float64(1000)
	t5000 := float64(5000)
	t9000 := float64(9000)

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"timeDoc1", &t1000},
		{"timeDoc2", &t5000},
		{"timeDoc3", &t9000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Reference: nil, Has: nil, None: nil, Unknown: nil,
				SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session with a time filter.
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9000", metadata["to"])
	assertIntervalPrefix(t, "80.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Value 1000 -> bucket[0], value 5000 -> bucket[49], value 9000 -> bucket[99].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 1000.0+float64(i)*80.00000000000001, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 0:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 1000)", i)
		case 49:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 5000)", i)
		case 99:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 9000)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(3), totalCount)
}

func TestTimeFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	t1000 := float64(1000)

	// Doc with the time prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("timeDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t1000, LessThan: nil, LessThanOrEqual: &t1000,
				},
				From: &t1000, FromDisplay: "", To: &t1000, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	// Doc without the time prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("timeDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount:    nil,
			Time:      nil,
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	_, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Missing count should be 1 (one document without the time prop).
	assert.Equal(t, int64(1), metadata["missing"])
}

func TestTimeFilterGetNoMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	t1000 := float64(1000)

	// All docs have the time prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("timeDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t1000, LessThan: nil, LessThanOrEqual: &t1000,
				},
				From: &t1000, FromDisplay: "", To: &t1000, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	_, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing documents.
	assert.Equal(t, int64(0), metadata["missing"])
}

func TestTimeFilterGetInactiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	t1000 := float64(1000)
	t5000 := float64(5000)
	t9000 := float64(9000)

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"timeDoc1", &t1000},
		{"timeDoc2", &t5000},
		{"timeDoc3", &t9000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Reference: nil, Has: nil, None: nil, Unknown: nil,
				SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for time histogram using the session's full query and prop from outside the session.
	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9000", metadata["to"])
	assertIntervalPrefix(t, "80.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify total count across all histogram bins equals 3.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(3), totalCount)
}

func TestTimeFilterGetSameValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t5000 := float64(5000)

	for i := range 2 {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From("sameTimeDoc", string(rune('0'+i))),
			Display: nil,
			Text:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: &t5000, LessThan: nil, LessThanOrEqual: &t5000,
					},
					From: &t5000, FromDisplay: "", To: &t5000, ToDisplay: "",
				}},
				Reference: nil, Has: nil, None: nil, Unknown: nil,
				SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)
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
	tNeg500 := float64(-500)
	t500 := float64(500)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("negTimeDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &tNeg500, LessThan: nil, LessThanOrEqual: &tNeg500,
				},
				From: &tNeg500, FromDisplay: "", To: &tNeg500, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("negTimeDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t500, LessThan: nil, LessThanOrEqual: &t500,
				},
				From: &t500, FromDisplay: "", To: &t500, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "-500", metadata["from"])
	assert.Equal(t, "500", metadata["to"])
	assertIntervalPrefix(t, "10.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Value -500 in bucket[0], value 500 in bucket[99].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, -500.0+float64(i)*10.000000000000002, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 0:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value -500)", i)
		case 99:
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

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)
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
	t4000 := float64(4000)
	t6000 := float64(6000)

	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"extTimeDoc1", &t4000},
		{"extTimeDoc2", &t6000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID:      identifier.From(tc.id),
			Display: nil,
			Text:    nil,
			Claims: internalSearch.ClaimTypes{
				Amount: nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Reference: nil, Has: nil, None: nil, Unknown: nil,
				SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Session filter with wider range [0, 10000] than data [4000, 6000].
	gte := float64(0)
	lte := float64(10000)
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "10000", metadata["to"])
	assertIntervalPrefix(t, "100.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Value 4000 -> bucket[39], value 6000 -> bucket[59].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, float64(i)*100.00000000000001, r.From, 1e-6, "bucket %d From", i)
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
	t0 := float64(0)
	t2000 := float64(2000)
	t8000 := float64(8000)
	t10000 := float64(10000)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("hardTimeDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t0, LessThan: nil, LessThanOrEqual: &t2000,
				},
				From: &t0, FromDisplay: "", To: &t2000, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("hardTimeDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t8000, LessThan: nil, LessThanOrEqual: &t10000,
				},
				From: &t8000, FromDisplay: "", To: &t10000, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Search session filters time between 1000 and 9000.
	// Both documents match because their ranges overlap [1000, 9000].
	gte := float64(1000)
	lte := float64(9000)
	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The session filter provides bounds [1000, 9000], so the histogram uses those.
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9000", metadata["to"])
	assertIntervalPrefix(t, "80.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Doc [0,2000] overlaps bins 0-12 (13 bins), doc [8000,10000] overlaps bins 87-99 (13 bins).
	// Total count = 13 + 13 = 26.
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 1000.0+float64(i)*80.00000000000001, r.From, 1e-6, "bucket %d From", i)
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
	assert.Equal(t, int64(26), totalCount)
}

func TestTimeFilterGetWideRangeFloategration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	// Doc1: point value at 500.
	t500 := float64(500)
	// Doc2: wide range [2000, 8000] - spans many histogram bins.
	t2000 := float64(2000)
	t8000 := float64(8000)
	// Doc3: point value at 9500.
	t9500 := float64(9500)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideTimeDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t500, LessThan: nil, LessThanOrEqual: &t500,
				},
				From: &t500, FromDisplay: "", To: &t500, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideTimeDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t2000, LessThan: nil, LessThanOrEqual: &t8000,
				},
				From: &t2000, FromDisplay: "", To: &t8000, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("wideTimeDoc3"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil,
			Time: internalSearch.TimeClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: &t9500, LessThan: nil, LessThanOrEqual: &t9500,
				},
				From: &t9500, FromDisplay: "", To: &t9500, ToDisplay: "",
			}},
			Reference: nil, Has: nil, None: nil, Unknown: nil,
			SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		View: "", Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "500", metadata["from"])
	assert.Equal(t, "9500", metadata["to"])
	assertIntervalPrefix(t, "90.0", metadata)

	// The wide-range document [2000, 8000] is counted in every bucket it overlaps.
	// Point value 500 -> bucket[0] (count 1), point value 9500 -> last bucket (count 1).
	// Wide range [2000, 8000] overlaps 68 buckets in the middle.
	// Total count = 70 (1 + 68 range buckets + 1).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 500.0+float64(i)*90.00000000000001, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
	}
	assert.Equal(t, int64(70), totalCount)

	// First and last buckets have the point values.
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
}
