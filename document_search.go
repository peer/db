package search

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

// search represents current search state.
// Search states form a tree with a link to the previous (parent) state.
type search struct {
	ID       string `json:"s"`
	Text     string `json:"q"`
	ParentID string `json:"-"`
}

// Encode returns search state as a query string.
// We want the order of parameters to be "s" and then "q" so that
// if "q" is cut, URL still works.
func (q *search) Encode() string {
	var buf strings.Builder
	buf.WriteString(url.QueryEscape("s"))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(q.ID))
	buf.WriteByte('&')
	buf.WriteString(url.QueryEscape("q"))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(q.Text))
	return buf.String()
}

// Encode returns search state as a query string, with additional "at" parameter.
// We want the order of parameters to be "s", "at", and then "q" so that
// if "q" is cut, URL still works.
func (q *search) EncodeWithAt(at string) string {
	if at == "" {
		return q.Encode()
	}
	var buf strings.Builder
	buf.WriteString(url.QueryEscape("s"))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(q.ID))
	buf.WriteByte('&')
	buf.WriteString(url.QueryEscape("at"))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(at))
	buf.WriteByte('&')
	buf.WriteString(url.QueryEscape("q"))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(q.Text))
	return buf.String()
}

// TODO: Use a database instead.
var searches = sync.Map{}

// field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// makeSearch creates a new search state given optional existing state and new queries.
func makeSearch(form url.Values) *search {
	parentSearchID := form.Get("s")
	if !identifier.Valid(parentSearchID) {
		parentSearchID = ""
	}
	textQuery := form.Get("q")
	if parentSearchID != "" {
		ps, ok := searches.Load(parentSearchID)
		if ok {
			parentSearch := ps.(*search) //nolint:errcheck
			// There was no change.
			if parentSearch.Text == textQuery {
				return parentSearch
			}
		} else {
			// Unknown ID.
			parentSearchID = ""
		}
	}
	sh := &search{
		ID:       identifier.NewRandom(),
		ParentID: parentSearchID,
		Text:     textQuery,
	}
	searches.Store(sh.ID, sh)
	return sh
}

// getOrMakeSearch resolves an existing search state if possible.
// If not, it creates a new search state.
func getOrMakeSearch(form url.Values) (*search, bool) {
	searchID := form.Get("s")
	if !identifier.Valid(searchID) {
		return makeSearch(form), false
	}
	sh, ok := searches.Load(searchID)
	if !ok {
		return makeSearch(form), false
	}
	textQuery := form.Get("q")
	ss := sh.(*search) //nolint:errcheck
	// There was a change, we make current search a parent search to a new search.
	// We allow there to not be "q" so that it is easier to use as an API.
	if form.Has("q") && ss.Text != textQuery {
		ss = &search{
			ID:       identifier.NewRandom(),
			ParentID: searchID,
			Text:     textQuery,
		}
		searches.Store(ss.ID, ss)
		return ss, false
	}
	return ss, true
}

// getSearch resolves an existing search state if possible.
func getSearch(form url.Values) *search {
	searchID := form.Get("s")
	if !identifier.Valid(searchID) {
		return nil
	}
	sh, ok := searches.Load(searchID)
	if !ok {
		return nil
	}
	textQuery := form.Get("q")
	ss := sh.(*search) //nolint:errcheck
	// We allow there to not be "q" so that it is easier to use as an API.
	if form.Has("q") && ss.Text != textQuery {
		return nil
	}
	return ss
}

// searchResult is returned from the searchGet API endpoint.
type searchResult struct {
	ID string `json:"_id"`
}

// DocumentSearchGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
// If search state is invalid, it redirects to a valid one.
func (s *Service) DocumentSearchGetHTML(w http.ResponseWriter, req *http.Request, _ Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh, ok := getOrMakeSearch(req.Form)
	m.Stop()
	if !ok {
		// Something was not OK, so we redirect to the correct URL.
		path, err := s.Router.Path("DocumentSearch", nil, sh.Encode())
		if err != nil {
			s.internalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	} else if !req.Form.Has("q") {
		// "q" is missing, so we redirect to the correct URL.
		path, err := s.Router.Path("DocumentSearch", nil, sh.EncodeWithAt(req.Form.Get("at")))
		if err != nil {
			s.internalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	if s.Development != "" {
		s.Proxy(w, req, nil)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}

// DocumentSearchGetJSON is a GET/HEAD HTTP request handler and it searches ElasticSearch index using provided
// search state and returns to the client a JSON with an array of IDs of found documents. If search state is
// invalid, it returns correct query parameters as JSON. It supports compression based on accepted content
// encoding and range requests. It returns search metadata (e.g., total results) as PeerDB HTTP response headers.
func (s *Service) DocumentSearchGetJSON(w http.ResponseWriter, req *http.Request, _ Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh, ok := getOrMakeSearch(req.Form)
	m.Stop()
	if !ok {
		// Something was not OK, so we return new query parameters.
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		s.writeJSON(w, req, contentEncoding, sh, nil)
		return
	}

	// TODO: Determine which operator should be the default?
	// TODO: Make sure right analyzers are used for all fields.
	// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
	searchService := s.ESClient.Search("docs").FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", idFromRequest(req)).From(0).Size(1000).TrackTotalHits(true) //nolint:gomnd
	if sh.Text == "" {
		matchQuery := elastic.NewMatchAllQuery()
		searchService = searchService.Query(matchQuery)
	} else {
		boolQuery := elastic.NewBoolQuery()
		// TODO: Check which analyzer is used.
		boolQuery = boolQuery.Should(elastic.NewSimpleQueryStringQuery(sh.Text).Field("name.en").DefaultOperator("AND"))
		for _, field := range []field{
			{"active.id", "id"},
			{"active.ref", "iri"},
			{"active.text", "html.en"},
			{"active.string", "string"},
		} {
			// TODO: Can we use simple query for keyword fields? Which analyzer is used?
			q := elastic.NewSimpleQueryStringQuery(sh.Text).Field(field.Prefix + "." + field.Field).DefaultOperator("AND")
			boolQuery = boolQuery.Should(elastic.NewNestedQuery(field.Prefix, q))
		}
		searchService = searchService.Query(boolQuery)
	}
	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	results := make([]searchResult, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		results[i] = searchResult{ID: hit.Id}
	}

	total := strconv.FormatInt(res.Hits.TotalHits.Value, 10) //nolint:gomnd
	if res.Hits.TotalHits.Relation == "gte" {
		total += "+"
	}

	metadata := http.Header{
		"Total": {total},
	}

	// A special case. If reqest had only "s" parameter, we expose the query in the response.
	if !req.Form.Has("q") {
		metadata.Set("Query", url.PathEscape(sh.Text))
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}

// DocumentSearchPostHTML is a POST HTTP request handler which stores the search state and redirect to
// the GET endpoint based on search ID. The handler follows the Post/Redirect/Get pattern.
func (s *Service) DocumentSearchPostHTML(w http.ResponseWriter, req *http.Request, _ Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh := makeSearch(req.Form)
	m.Stop()
	path, err := s.Router.Path("DocumentSearch", nil, sh.Encode())
	if err != nil {
		s.internalServerErrorWithError(w, req, err)
		return
	}

	// TODO: Should we push the location to the client, too?
	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit ES twice?
	w.Header().Set("Location", path)
	w.WriteHeader(http.StatusSeeOther)
}

// DocumentSearchPostJSON is a POST HTTP request handler which stores the search state and returns
// query parameters for the GET endpoint as JSON.
func (s *Service) DocumentSearchPostJSON(w http.ResponseWriter, req *http.Request, _ Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh := makeSearch(req.Form)
	m.Stop()

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit ES twice?
	s.writeJSON(w, req, contentEncoding, sh, nil)
}
