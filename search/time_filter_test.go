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

	seedTimeFilterDocs(t, ctx, esClient, index, timeProp)

	// Create a session with a time filter.
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// The histogram spans the known endpoint window edges [1000, 9001).
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9001", metadata["to"])
	assertIntervalPrefix(t, "80.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Window [1000, 1001) -> bucket[0], window [9000, 9001) -> bucket[99], while window
	// [5000, 5001) straddles the boundary between buckets [49] and [50] and is counted
	// in both.
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 1000.0+float64(i)*80.01000000000002, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 0:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 1000)", i)
		case 49, 50:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 5000)", i)
		case 99:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 9000)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(4), totalCount)
}

func TestTimeFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	t1000 := float64(1000)

	// Doc with the time prop.
	indexTimePointDoc(t, ctx, esClient, index, "timeDoc1", timeProp, &t1000)
	// Doc without the time prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("timeDoc2"),
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
	indexTimePointDoc(t, ctx, esClient, index, "timeDoc1", timeProp, &t1000)
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

	seedTimeFilterDocs(t, ctx, esClient, index, timeProp)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for time histogram using the session's full query and prop from outside the session.
	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "9001", metadata["to"])
	assertIntervalPrefix(t, "80.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Verify total count across all histogram bins equals 4 (the window [5000, 5001)
	// straddles a bucket boundary and is counted in two buckets).
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(4), totalCount)
}

func TestTimeFilterGetSameValuesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t5000 := float64(5000)

	// Two open-ended claims with the same known endpoint: the only known endpoint value is
	// 5000, so the histogram collapses to a single bucket.
	indexTimeIntervalDoc(t, ctx, esClient, index, "sameTimeDoc0", timeProp, &t5000, nil)
	indexTimeIntervalDoc(t, ctx, esClient, index, "sameTimeDoc1", timeProp, &t5000, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
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

	indexTimePointDoc(t, ctx, esClient, index, "negTimeDoc1", timeProp, &tNeg500)
	indexTimePointDoc(t, ctx, esClient, index, "negTimeDoc2", timeProp, &t500)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "-500", metadata["from"])
	assert.Equal(t, "501", metadata["to"])
	assertIntervalPrefix(t, "10.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Value -500 in bucket[0], value 500 in bucket[99].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, -500.0+float64(i)*10.010000000000002, r.From, 1e-6, "bucket %d From", i)
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
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HistogramResult{}, results)
	assert.Equal(t, "0", metadata["total"])
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

	indexTimePointDoc(t, ctx, esClient, index, "extTimeDoc1", timeProp, &t4000)
	indexTimePointDoc(t, ctx, esClient, index, "extTimeDoc2", timeProp, &t6000)
	refreshIndex(t, ctx, esClient, index)

	// Session filter with wider range [0, 10000] than data [4000, 6000].
	gte := float64(0)
	lte := float64(10000)
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "0", metadata["from"])
	assert.Equal(t, "10000", metadata["to"])
	assertIntervalPrefix(t, "100.0", metadata)
	assert.Equal(t, "100", metadata["total"])
	require.Len(t, results, 100)

	// Window [4000, 4001) straddles buckets [39] and [40], window [6000, 6001) straddles
	// buckets [59] and [60].
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, float64(i)*100.00000000000001, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
		switch i {
		case 39, 40:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 4000)", i)
		case 59, 60:
			assert.Equal(t, int64(1), r.Count, "bucket %d Count (value 6000)", i)
		default:
			assert.Equal(t, int64(0), r.Count, "bucket %d Count", i)
		}
	}
	assert.Equal(t, int64(4), totalCount)
}

func TestTimeFilterGetHardBoundsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	// Two time intervals: [0, 2000) and [8000, 10000), with the to values being window ends.
	t0 := float64(0)
	t2000 := float64(2000)
	t8000 := float64(8000)
	t10000 := float64(10000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "hardTimeDoc1", timeProp, &t0, &t2000)
	indexTimeIntervalDoc(t, ctx, esClient, index, "hardTimeDoc2", timeProp, &t8000, &t10000)
	refreshIndex(t, ctx, esClient, index)

	// Search session filters time between 1000 and 9000.
	// Both documents match because their ranges overlap [1000, 9000].
	gte := float64(1000)
	lte := float64(9000)
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
		}},
		Prefilters: nil,
		Reverse:    nil,
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

func TestTimeFilterGetWideRangeIntegration(t *testing.T) {
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

	indexTimePointDoc(t, ctx, esClient, index, "wideTimeDoc1", timeProp, &t500)
	indexTimeIntervalDoc(t, ctx, esClient, index, "wideTimeDoc2", timeProp, &t2000, &t8000)
	indexTimePointDoc(t, ctx, esClient, index, "wideTimeDoc3", timeProp, &t9500)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "500", metadata["from"])
	assert.Equal(t, "9501", metadata["to"])
	assertIntervalPrefix(t, "90.0", metadata)

	// The wide-range document [2000, 8000) is counted in every bucket it overlaps.
	// Point window [500, 501) -> bucket[0] (count 1), point window [9500, 9501) -> last
	// bucket (count 1). Wide range [2000, 8000) overlaps 68 buckets in the middle.
	// Total count = 70 (1 + 68 range buckets + 1).
	var totalCount int64
	for i, r := range results {
		assert.InDelta(t, 500.0+float64(i)*90.01000000000002, r.From, 1e-6, "bucket %d From", i)
		totalCount += r.Count
	}
	assert.Equal(t, int64(70), totalCount)

	// First and last buckets have the point values.
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
}

// A single matching claim with an open (none) end indexes no to field at all, so the max
// aggregation over it has no value.
func TestTimeFilterGetOpenEndIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "openEndTimeDoc1", timeProp, &t1000, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The only known endpoint value is the from, so the histogram collapses to a single bucket.
	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "1000", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 1000.0, Count: 1}}, results)
}

// Claims open-ended in opposite directions: aggregating just min over from and max over to
// would produce an inverted range (min 5000, max 1000). The combined aggregation over both
// fields spans all known endpoint values instead.
func TestTimeFilterGetOppositeOpenEndsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)
	t5000 := float64(5000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "oppositeTimeDoc1", timeProp, nil, &t1000)
	indexTimeIntervalDoc(t, ctx, esClient, index, "oppositeTimeDoc2", timeProp, &t5000, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "100", metadata["total"])
	// The min known endpoint value 1000 is a to value, so the histogram start is lowered by
	// one second step to catch the open-start document whose range ends exclusively at 1000.
	assert.Equal(t, "999", metadata["from"])
	assert.Equal(t, "5000", metadata["to"])
	assertIntervalPrefix(t, "40.0", metadata)
	require.Len(t, results, 100)

	// The document with the open start overlaps just the first bucket (its range ends right
	// below 1000), while the document with the open end overlaps every bucket from 5000 on,
	// which within hard bounds is just the last one.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(2), totalCount)
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
}

// A single open-start claim: its known endpoint is its to value 1000, so the histogram
// collapses to a single bucket displayed at 1000, but the from metadata bound is lowered
// by one precision step because the claim does not contain 1000 itself (its range upper
// bound is exclusive) and a filter using [1000, 1000] bounds would not match it.
func TestTimeFilterGetOpenStartIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "openStartOnlyTimeDoc1", timeProp, nil, &t1000)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "999", metadata["from"])
	assert.Equal(t, "1000", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 1000.0, Count: 1}}, results)
}

// A single-value selection round-trips: an active filter using the from and to bounds of a
// single-bucket response gets the same single-bucket response back (the data still has a
// single known endpoint value), not a degenerate histogram over the selected bounds.
func TestTimeFilterGetSingleValueActiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "singleValueTimeDoc1", timeProp, nil, &t1000)
	refreshIndex(t, ctx, esClient, index)

	gte := float64(999)
	lte := float64(1000)
	session := createSession(t, ctx, search.SessionData{
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
		}},
		Prefilters: nil,
		Reverse:    nil,
	})

	results, metadata, errE := session.Filters[0].Time.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "999", metadata["from"])
	assert.Equal(t, "1000", metadata["to"])
	assert.Equal(t, []search.HistogramResult{{From: 1000.0, Count: 1}}, results)
}

// An open-start claim combined with a bounded claim: aggregating just min over from would
// put the histogram start at 3000 and hide the open-start document entirely. The combined
// min comes from its to value 1000 and the histogram start is lowered by one second step
// below it so the document is counted in the first bucket.
func TestTimeFilterGetOpenStartWithBoundedIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)
	t3000 := float64(3000)
	t5000 := float64(5000)

	indexTimeIntervalDoc(t, ctx, esClient, index, "openStartTimeDoc1", timeProp, nil, &t1000)
	indexTimeIntervalDoc(t, ctx, esClient, index, "openStartTimeDoc2", timeProp, &t3000, &t5000)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "100", metadata["total"])
	assert.Equal(t, "999", metadata["from"])
	assert.Equal(t, "5000", metadata["to"])
	assertIntervalPrefix(t, "40.0", metadata)
	require.Len(t, results, 100)

	// The open-start document overlaps just the first bucket (its range ends right below
	// 1000) and the bounded document overlaps the 50 buckets spanning [3000, 5000).
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(0), results[1].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
	assert.Equal(t, int64(51), totalCount)
}

// A claim with both endpoints open (none) indexes neither the from nor the to field, only
// sentinel range bounds, so there are no known endpoint values to span a histogram with.
func TestTimeFilterGetUnboundedIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")

	indexTimeIntervalDoc(t, ctx, esClient, index, "unboundedTimeDoc1", timeProp, nil, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "0", metadata["total"])
	assert.Equal(t, int64(0), metadata["missing"])
	assert.Empty(t, results)
}

// A claim with one unknown endpoint never reaches the index with a missing from or to
// field: the converter collapses it to a point claim at the known endpoint (see
// TestConvertTimeIntervalToUnknownWithFrom), so it supplies histogram bounds like any
// point value, here combined with an open-ended claim supplying the other bound.
func TestTimeFilterGetUnknownEndIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)
	t5000 := float64(5000)

	// The indexed shape of an interval claim [1000, unknown) after conversion.
	indexTimePointDoc(t, ctx, esClient, index, "unknownEndTimeDoc1", timeProp, &t1000)
	indexTimeIntervalDoc(t, ctx, esClient, index, "unknownEndTimeDoc2", timeProp, &t5000, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "100", metadata["total"])
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "5000", metadata["to"])
	assertIntervalPrefix(t, "40.0", metadata)
	require.Len(t, results, 100)

	// The collapsed point lands in the first bucket and the open-ended claim overlaps
	// every bucket from 5000 on, which within hard bounds is just the last one.
	var totalCount int64
	for _, r := range results {
		totalCount += r.Count
	}
	assert.Equal(t, int64(2), totalCount)
	assert.Equal(t, int64(1), results[0].Count)
	assert.Equal(t, int64(1), results[len(results)-1].Count)
}

// A claim with both endpoints unknown is converted to an unknown claim (see
// TestConvertTimeIntervalBothUnknown) and is not indexed under claims.time at all, so
// its document counts as missing for the filter and does not affect the histogram.
func TestTimeFilterGetFullyUnknownIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	t1000 := float64(1000)

	// The indexed shape of an interval claim with both endpoints unknown after conversion.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("fullyUnknownTimeDoc1"),
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
			Unknown: internalSearch.UnknownClaims{{
				Prop: timeProp, PropDisplay: nil, PropNaming: nil,
			}},
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexTimeIntervalDoc(t, ctx, esClient, index, "fullyUnknownTimeDoc2", timeProp, &t1000, nil)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.TimeFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), timeProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "1000", metadata["from"])
	assert.Equal(t, "1000", metadata["to"])
	assert.Equal(t, int64(1), metadata["missing"])
	assert.Equal(t, []search.HistogramResult{{From: 1000.0, Count: 1}}, results)
}
