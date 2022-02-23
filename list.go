package search

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/golang/gddo/httputil"
	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

// Search represents current search state.
// Search states form a tree with a link to the previous (parent) state.
type search struct {
	ID       string
	ParentID string
	Text     string
}

// Encode encodes search state into a string suitable for use in a query string.
func (q *search) Encode() string {
	v := url.Values{}
	v.Set("q", q.Text)
	v.Set("s", q.ID)
	return v.Encode()
}

// TODO: Use a database instead.
var searches = sync.Map{}

// Field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// MakeSearch creates a new search state given optional existing state and new queries.
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

// GetSearch resolves an existing search state if possible.
// If not, it creates a new search state.
func getSearch(form url.Values) (*search, bool) {
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
	// There was a change, we make current search
	// a parent search to a new search.
	if ss.Text != textQuery {
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

// ListResult is returned from the ListGet API endpoint.
type listResult struct {
	ID string `json:"_id"`
}

// ListGet is a GET/HEAD HTTP request handler and it searches ElasticSearch index using provided search
// state and returns to the client a JSON with an array of IDs of found documents. If called using
// HTTP2, it also pushes all found documents to the client. If search state is invalid, it redirects to
// a valid one. It supports compression based on accepted content encoding and range requests.
// It returns search metadata (e.g., total results) as PeerDB HTTP response headers.
func (s *Service) ListGet(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := req.ParseForm()
	if err != nil {
		s.badRequest(w, req, errors.WithStack(err))
		return
	}
	sh, ok := getSearch(req.Form)
	if !ok {
		// Something was not OK, so we redirect to the correct URL.
		w.Header().Set("Location", "/d?"+sh.Encode())
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	contentEncoding := httputil.NegotiateContentEncoding(req, []string{compressionBrotli, compressionGzip, compressionDeflate, compressionIdentity})
	if contentEncoding == "" {
		http.Error(w, "406 not acceptable", http.StatusNotAcceptable)
		return
	}

	// TODO: Determine which operator should be the default?
	// TODO: Make sure right analyzers are used for all fields.
	// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
	ctx := req.Context()
	searchService := s.ESClient.Search("docs").From(0).Size(1000).FetchSource(false).Routing(getHost(req.RemoteAddr)) //nolint:gomnd
	if sh.Text == "" {
		matchQuery := elastic.NewMatchAllQuery()
		searchService = searchService.Query(matchQuery)
	} else {
		boolQuery := elastic.NewBoolQuery()
		// TODO: Check which analyzer is used.
		boolQuery = boolQuery.Should(elastic.NewSimpleQueryStringQuery(sh.Text).Field("name.en").Field("otherNames.en").DefaultOperator("AND"))
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
	searchResult, err := searchService.Do(ctx)
	if err != nil {
		s.internalServerError(w, req, errors.WithStack(err))
		return
	}

	pusher, ok := w.(http.Pusher)
	if !ok {
		pusher = nil
	}
	options := &http.PushOptions{
		Header: http.Header{
			"Accept-Encoding": req.Header["Accept-Encoding"],
		},
	}

	results := make([]listResult, len(searchResult.Hits.Hits))
	for i, hit := range searchResult.Hits.Hits {
		results[i] = listResult{ID: hit.Id}
		if pusher != nil {
			err := pusher.Push("/d/"+hit.Id, options)
			if errors.Is(err, http.ErrNotSupported) {
				// Nothing.
			} else if err != nil {
				// TODO: Use logger.
				fmt.Fprintf(os.Stderr, "failed to push: %+v\n", err)
			}
		}
	}

	total := strconv.FormatInt(searchResult.Hits.TotalHits.Value, 10) //nolint:gomnd
	if searchResult.Hits.TotalHits.Relation == "gte" {
		total += "+"
	}

	s.writeJSON(w, req, contentEncoding, results, http.Header{
		"Total": {total},
	})
}

// ListPost is a POST HTTP request handler which stores the search state and redirect to
// the GET endpoint based on search ID. The handler follows the Post/Redirect/Get pattern.
func (s *Service) ListPost(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := req.ParseForm()
	if err != nil {
		s.badRequest(w, req, errors.WithStack(err))
		return
	}
	sh := makeSearch(req.Form)
	// TODO: Should we push the location to the client, too?
	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit ES twice?
	w.Header().Set("Location", "/d?"+sh.Encode())
	w.WriteHeader(http.StatusSeeOther)
}
