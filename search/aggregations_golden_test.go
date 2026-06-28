package search_test

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// updateGolden, when set, makes the aggregation golden tests (re)write their golden files under testdata/
// instead of comparing against them. Regenerate with "go test ./search/ -run Golden -update-golden".
var updateGolden = flag.Bool("update-golden", false, "update aggregation golden files") //nolint:gochecknoglobals

// emptySearchResponse is a minimal valid Elasticsearch search response with empty aggregations, returned by
// the recording transport. The X-Elastic-Product header is the product check the typed client requires. The
// functions under test parse these empty aggregations and return an error, which the capture helper ignores:
// only the recorded outgoing request is of interest.
const emptySearchResponse = `{"took":0,"timed_out":false,"_shards":{"total":1,"successful":1,"skipped":0,"failed":0},` +
	`"hits":{"total":{"value":0,"relation":"eq"},"hits":[]},"aggregations":{}}`

// recordingRoundTripper is an http.RoundTripper that records the outgoing request body and replies with a
// canned response, so an Elasticsearch request a filter function builds can be captured without a real
// Elasticsearch.
type recordingRoundTripper func(req *http.Request) (*http.Response, error)

func (f recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// captureAggregationRequest runs call against a getSearchService backed by a recording transport and returns
// the body of the single Elasticsearch request it sends. The function under test then parses the canned empty
// response and returns an error; that error is ignored on purpose, since only the recorded request matters.
func captureAggregationRequest(t *testing.T, call func(getSearchService func() *esSearch.Search)) []byte {
	t.Helper()

	var captured []byte
	transport := recordingRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			captured = body
		}
		return &http.Response{ //nolint:exhaustruct
			StatusCode: http.StatusOK,
			Header: http.Header{
				"X-Elastic-Product": []string{"Elasticsearch"},
				"Content-Type":      []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(emptySearchResponse)),
		}, nil
	})

	esClient, errE := internalSearch.GetClient(&http.Client{Transport: transport}, zerolog.Nop(), "http://localhost:9200") //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)

	getSearchService := func() *esSearch.Search {
		return esClient.Search().Index("test")
	}

	call(getSearchService)
	require.NotEmpty(t, captured, "no request body was captured")

	return captured
}

// assertAggregationsGolden extracts the top-level "aggregations" object from a captured Elasticsearch request
// body, re-marshals it indented (so map keys are sorted and the diff is readable), and compares it with the
// golden file testdata/<name>.json. With -update-golden it writes the golden file instead. Only the
// aggregation structure is snapshotted here; the document-matching query is covered by the ToQuery goldens.
func assertAggregationsGolden(t *testing.T, name string, requestBody []byte) {
	t.Helper()

	var body struct {
		Aggregations json.RawMessage `json:"aggregations"`
	}
	errE := x.Unmarshal(requestBody, &body)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, body.Aggregations, "captured request has no aggregations")

	var aggs any
	errE = x.Unmarshal(body.Aggregations, &aggs)
	require.NoError(t, errE, "% -+#.1v", errE)
	got, err := json.MarshalIndent(aggs, "", "  ")
	require.NoError(t, err)

	path := filepath.Join("testdata", name+".json")

	if *updateGolden {
		errMk := os.MkdirAll("testdata", 0o755) //nolint:gosec
		require.NoError(t, errMk)
		errW := os.WriteFile(path, append(got, '\n'), 0o644) //nolint:gosec
		require.NoError(t, errW)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec
	require.NoError(t, err, "missing golden file %s, run with -update-golden", path)
	assert.JSONEq(t, string(want), string(got))
}

// aggregationsGoldenSession builds, without touching the database, a session carrying one active reference
// filter, so FiltersGet emits both the discovery aggregations and one per-active-filter active_N aggregation.
// All ids are deterministic identifier.From hashes, matching the existing ToQuery goldens.
func aggregationsGoldenSession() *search.Session {
	prop := identifier.From("prop")
	value := identifier.From("value")
	filterBase := []string{"test", "FILTER", "filter1"}
	filterID := identifier.From(filterBase...)
	sessionBase := []string{"test", "SEARCH", "session1"}
	return &search.Session{
		SessionData: search.SessionData{ //nolint:exhaustruct
			Filters: []search.Filter{{ //nolint:exhaustruct
				ID:   &filterID,
				Base: filterBase,
				Prop: []identifier.Identifier{prop},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false},
			}},
		},
		ID:      identifier.From(sessionBase...),
		Base:    sessionBase,
		Version: 0,
	}
}

func TestFiltersGetAggregationsGolden(t *testing.T) {
	t.Parallel()

	session := aggregationsGoldenSession()
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	t.Run("NoQuery", func(t *testing.T) {
		t.Parallel()

		ctx := siteContext(t.Context())
		body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
			_, _, _ = search.FiltersGet(ctx, getSearchService, session, enabledLanguages, "", search.PrefilterExcludes{})
		})
		assertAggregationsGolden(t, "filters_get_no_query", body)
	})

	t.Run("ValueQuery", func(t *testing.T) {
		t.Parallel()

		ctx := siteContext(t.Context())
		body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
			_, _, _ = search.FiltersGet(ctx, getSearchService, session, enabledLanguages, "col*", search.PrefilterExcludes{})
		})
		assertAggregationsGolden(t, "filters_get_value_query", body)
	})
}

func TestRefFilterGetAggregationsGolden(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")
	hierProp := identifier.From("hierProp")
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	t.Run("NoQuery", func(t *testing.T) {
		t.Parallel()

		ctx := siteContext(t.Context())
		f := &search.RefFilter{To: nil, Direct: nil, Missing: false}
		body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
			_, _, _ = f.Get(ctx, getSearchService, esdsl.NewMatchAllQuery(), prop, nil, "", enabledLanguages, nil)
		})
		assertAggregationsGolden(t, "ref_filter_get_no_query", body)
	})

	t.Run("ValueQuery", func(t *testing.T) {
		t.Parallel()

		ctx := siteContext(t.Context())
		// An active filter with a selected value plus a resolver that surfaces it, so the selectedMatch and
		// propMatch augment aggregations appear. The resolver returns a single-segment hierarchy path, so the
		// augment is exactly the selected value (one id) and the captured terms query is deterministic.
		f := &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false}
		resolver := newPathResolver(map[identifier.Identifier][]string{
			value: {hierProp.String() + ":" + value.String()},
		})
		body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
			_, _, _ = f.Get(ctx, getSearchService, esdsl.NewMatchAllQuery(), prop, nil, "col*", enabledLanguages, resolver)
		})
		assertAggregationsGolden(t, "ref_filter_get_value_query", body)
	})
}

func TestRefFilterGetSubRefAggregationsGolden(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	prop := identifier.From("prop")
	value := identifier.From("value")
	hierProp := identifier.From("hierProp")
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	ctx := siteContext(t.Context())
	f := &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false}
	resolver := newPathResolver(map[identifier.Identifier][]string{
		value: {hierProp.String() + ":" + value.String()},
	})
	body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
		_, _, _ = f.GetSubRef(ctx, getSearchService, esdsl.NewMatchAllQuery(), parentProp, prop, nil, nil, "col*", enabledLanguages, resolver)
	})
	assertAggregationsGolden(t, "ref_filter_get_subref_value_query", body)
}

func TestHasFilterGetAggregationsGolden(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	ctx := siteContext(t.Context())
	f := &search.HasFilter{Props: []search.HasValue{{ID: prop}}}
	body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
		_, _, _ = f.Get(ctx, getSearchService, esdsl.NewMatchAllQuery(), "col*", enabledLanguages)
	})
	assertAggregationsGolden(t, "has_filter_get_value_query", body)
}

func TestHasFilterGetSubHasAggregationsGolden(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	prop := identifier.From("prop")
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	ctx := siteContext(t.Context())
	f := &search.HasFilter{Props: []search.HasValue{{ID: prop}}}
	body := captureAggregationRequest(t, func(getSearchService func() *esSearch.Search) {
		_, _, _ = f.GetSubHas(ctx, getSearchService, esdsl.NewMatchAllQuery(), parentProp, nil, "col*", enabledLanguages)
	})
	assertAggregationsGolden(t, "has_filter_get_subhas_value_query", body)
}
