package search

import (
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search/identifier"
)

const (
	maxResultsCount = 1000
)

type filters struct {
	And   []filters `json:"and,omitempty"`
	Or    []filters `json:"or,omitempty"`
	Not   *filters  `json:"not,omitempty"`
	Prop  string    `json:"prop,omitempty"`
	Value string    `json:"value,omitempty"`
	None  bool      `json:"none,omitempty"`
}

func (f filters) Valid() errors.E {
	nonEmpty := 0
	if len(f.And) > 0 {
		nonEmpty++
	}
	if len(f.Or) > 0 {
		nonEmpty++
	}
	if f.Not != nil {
		nonEmpty++
	}
	if f.Prop != "" {
		nonEmpty++
		if f.Value == "" && !f.None {
			return errors.New("value or none has to be set if prop is set")
		}
		if f.Value != "" && f.None {
			return errors.New("value and none cannot be both set")
		}
		if !identifier.Valid(f.Prop) {
			return errors.New("invalid prop")
		}
		if f.Value != "" && !identifier.Valid(f.Value) {
			return errors.New("invalid value")
		}
	} else {
		if f.Value != "" {
			return errors.New("value can be set only if prop is set as well")
		}
		if f.None {
			return errors.New("none can be set only if prop is set as well")
		}
	}
	if nonEmpty > 1 {
		return errors.New("only one clause can be set")
	} else if nonEmpty == 0 {
		return errors.New("no clause is set")
	}
	for _, c := range f.And {
		err := c.Valid()
		if err != nil {
			return err
		}
	}
	for _, c := range f.Or {
		err := c.Valid()
		if err != nil {
			return err
		}
	}
	return nil
}

func (f filters) ToQuery() elastic.Query { //nolint:ireturn
	if len(f.And) > 0 {
		boolQuery := elastic.NewBoolQuery()
		for _, filter := range f.And {
			boolQuery.Must(filter.ToQuery())
		}
		return boolQuery
	}
	if len(f.Or) > 0 {
		boolQuery := elastic.NewBoolQuery()
		for _, filter := range f.Or {
			boolQuery.Should(filter.ToQuery())
		}
		return boolQuery
	}
	if f.Not != nil {
		boolQuery := elastic.NewBoolQuery()
		boolQuery.MustNot(f.Not.ToQuery())
		return boolQuery
	}
	if f.Prop != "" && f.None {
		boolQuery := elastic.NewBoolQuery()
		boolQuery.MustNot(elastic.NewTermQuery("active.rel.prop._id", f.Prop))
		return elastic.NewNestedQuery("active.rel", boolQuery)
	}
	if f.Prop != "" && f.Value != "" {
		boolQuery := elastic.NewBoolQuery()
		boolQuery.Must(elastic.NewTermQuery("active.rel.prop._id", f.Prop))
		boolQuery.Must(elastic.NewTermQuery("active.rel.to._id", f.Value))
		return elastic.NewNestedQuery("active.rel", boolQuery)
	}
	panic(errors.New("invalid filters"))
}

// search represents current search state.
// Search states form a tree with a link to the previous (parent) state.
type search struct {
	ID       string   `json:"s"`
	Text     string   `json:"q"`
	Filters  *filters `json:"filters"`
	ParentID string   `json:"-"`
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

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// makeSearch creates a new search state given optional existing state and new queries.
func makeSearch(form url.Values) *search {
	parentSearchID := form.Get("s")
	if !identifier.Valid(parentSearchID) {
		parentSearchID = ""
	}

	textQuery := form.Get("q")

	var fs *filters
	fsJSON := form.Get("filters")
	if fsJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(fsJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		}
	}

	if parentSearchID != "" {
		ps, ok := searches.Load(parentSearchID)
		if ok {
			parentSearch := ps.(*search) //nolint:errcheck
			// There was no change.
			if parentSearch.Text == textQuery && reflect.DeepEqual(parentSearch.Filters, fs) {
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
		Filters:  fs,
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

	var fs *filters
	fsJSON := form.Get("filters")
	if fsJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(fsJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		}
	}

	ss := sh.(*search) //nolint:errcheck
	// There was a change, we make current search a parent search to a new search.
	// We allow there to not be "q" or "filters" so that it is easier to use as an API.
	if (form.Has("q") && ss.Text != textQuery) || (form.Has("filters") && !reflect.DeepEqual(ss.Filters, fs)) {
		ss = &search{
			ID:       identifier.NewRandom(),
			ParentID: searchID,
			Text:     textQuery,
			Filters:  fs,
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
	ID    string `json:"_id"`
	Count int64  `json:"_count,omitempty"`
	Type  string `json:"_type,omitempty"`
	Unit  string `json:"_unit,omitempty"`
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

func (s *Service) getSearchService(req *http.Request) *elastic.SearchService {
	return s.ESClient.Search(s.Index).FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", idFromRequest(req)).TrackTotalHits(true)
}

// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *Service) getSearchQuery(sh *search) elastic.Query { //nolint:ireturn
	boolQuery := elastic.NewBoolQuery()

	if sh.Text != "" {
		bq := elastic.NewBoolQuery()
		bq.Should(elastic.NewTermQuery("_id", sh.Text))
		// TODO: Check which analyzer is used.
		bq.Should(elastic.NewSimpleQueryStringQuery(sh.Text).Field("name.en").DefaultOperator("AND"))
		for _, field := range []field{
			{"active.id", "id"},
			{"active.ref", "iri"},
			{"active.text", "html.en"},
			{"active.string", "string"},
		} {
			// TODO: Can we use simple query for keyword fields? Which analyzer is used?
			q := elastic.NewSimpleQueryStringQuery(sh.Text).Field(field.Prefix + "." + field.Field).DefaultOperator("AND")
			bq.Should(elastic.NewNestedQuery(field.Prefix, q))
		}
		boolQuery.Must(bq)
	}

	if sh.Filters != nil {
		boolQuery.Must(sh.Filters.ToQuery())
	}

	return boolQuery
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

	searchService := s.getSearchService(req).From(0).Size(maxResultsCount).Query(s.getSearchQuery(sh))

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

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

	// TODO: Move this to a separate API endpoint.
	filters, err := x.MarshalWithoutEscapeHTML(sh.Filters)
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	metadata.Set("Query", url.PathEscape(sh.Text))
	metadata.Set("Filters", url.PathEscape(string(filters)))

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
