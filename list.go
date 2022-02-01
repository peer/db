package search

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/golang/gddo/httputil"
	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

type Search struct {
	ID       string
	ParentID string
	Text     string
}

func (q *Search) Encode() string {
	v := url.Values{}
	v.Set("q", q.Text)
	v.Set("s", q.ID)
	return v.Encode()
}

// TODO: Use a database instead.
var searches = sync.Map{}

type Field struct {
	Prefix string
	Field  string
}

func makeSearch(form url.Values) *Search {
	parentSearchID := form.Get("s")
	if !identifier.Valid(parentSearchID) {
		parentSearchID = ""
	}
	textQuery := form.Get("q")
	if parentSearchID != "" {
		ps, ok := searches.Load(parentSearchID)
		if ok {
			parentSearch := ps.(*Search)
			// There was no change.
			if parentSearch.Text == textQuery {
				return parentSearch
			}
		} else {
			// Unknown ID.
			parentSearchID = ""
		}
	}
	search := &Search{
		ID:       identifier.NewRandom(),
		ParentID: parentSearchID,
		Text:     textQuery,
	}
	searches.Store(search.ID, search)
	return search
}

type ListResult struct {
	ID string `json:"_id"`
}

func getSearch(form url.Values) (*Search, bool) {
	searchID := form.Get("s")
	if !identifier.Valid(searchID) {
		return makeSearch(form), false
	}
	s, ok := searches.Load(searchID)
	if !ok {
		return makeSearch(form), false
	}
	textQuery := form.Get("q")
	search := s.(*Search)
	// There was a change, we make current search
	// a parent search to a new search.
	if search.Text != textQuery {
		search = &Search{
			ID:       identifier.NewRandom(),
			ParentID: searchID,
			Text:     textQuery,
		}
		searches.Store(search.ID, search)
		return search, false
	}
	return search, true
}

func ListGet(client *elastic.Client) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		err := req.ParseForm()
		if err != nil {
			BadRequestError(w, req, errors.WithStack(err))
			return
		}
		search, ok := getSearch(req.Form)
		if !ok {
			// Something was not OK, so we redirect to the correct URL.
			w.Header().Set("Location", "/d?"+search.Encode())
			w.WriteHeader(http.StatusSeeOther)
			return
		}

		contentEncoding := httputil.NegotiateContentEncoding(req, []string{"br", "gzip", "deflate", "identity"})
		if contentEncoding == "" {
			http.Error(w, "406 not acceptable", http.StatusNotAcceptable)
			return
		}

		ctx := req.Context()
		searchService := client.Search("docs").From(0).Size(1000).FetchSource(false).Routing(req.RemoteAddr)
		if search.Text == "" {
			matchQuery := elastic.NewMatchAllQuery()
			searchService = searchService.Query(matchQuery)
		} else {
			boolQuery := elastic.NewBoolQuery()
			boolQuery = boolQuery.Should(elastic.NewSimpleQueryStringQuery(search.Text).Field("name.en").Field("otherNames.en").DefaultOperator("AND"))
			for _, field := range []Field{
				{"active.id", "id"},
				{"active.ref", "iri"},
				{"active.text", "html.en"},
				{"active.string", "string"},
			} {
				// TODO: Can we use simple query for keyword fields?
				q := elastic.NewSimpleQueryStringQuery(search.Text).Field(field.Prefix + "." + field.Field).DefaultOperator("AND")
				boolQuery = boolQuery.Should(elastic.NewNestedQuery(field.Prefix, q))
			}
			searchService = searchService.Query(boolQuery)
		}
		searchResult, err := searchService.Do(ctx)
		if err != nil {
			InternalError(w, req, errors.WithStack(err))
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

		results := make([]ListResult, len(searchResult.Hits.Hits))
		for i, hit := range searchResult.Hits.Hits {
			results[i] = ListResult{ID: hit.Id}
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

		encoded, err := json.Marshal(results)
		if err != nil {
			InternalError(w, req, errors.WithStack(err))
			return
		}

		// Compress only if content is larger than 1 KB.
		if len(encoded) <= 1024 {
			contentEncoding = "identity"
		}

		// TODO: Use a pool of compression workers?
		switch contentEncoding {
		case "br":
			var buf bytes.Buffer
			writer := brotli.NewWriter(&buf)
			_, err := writer.Write(encoded)
			if closeErr := writer.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				InternalError(w, req, errors.WithStack(err))
				return
			}
			encoded = buf.Bytes()
		case "gzip":
			var buf bytes.Buffer
			writer := gzip.NewWriter(&buf)
			_, err := writer.Write(encoded)
			if closeErr := writer.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				InternalError(w, req, errors.WithStack(err))
				return
			}
			encoded = buf.Bytes()
		case "deflate":
			var buf bytes.Buffer
			writer, err := flate.NewWriter(&buf, -1)
			if err != nil {
				InternalError(w, req, errors.WithStack(err))
				return
			}
			_, err = writer.Write(encoded)
			if closeErr := writer.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				InternalError(w, req, errors.WithStack(err))
				return
			}
			encoded = buf.Bytes()
		case "identity":
			// Nothing.
		}

		hash := sha256.Sum256(encoded)
		etag := `"` + base64.RawURLEncoding.EncodeToString(hash[:]) + `"`

		w.Header().Set("Content-Type", "application/json")
		if contentEncoding != "identity" {
			w.Header().Set("Content-Encoding", contentEncoding)
		} else {
			// TODO: Always set Content-Length.
			//       See: https://github.com/golang/go/pull/50904
			w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Etag", etag)

		// See: https://github.com/golang/go/issues/50905
		// See: https://github.com/golang/go/pull/50903
		http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(encoded))
	}
}

func ListPost(client *elastic.Client) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		err := req.ParseForm()
		if err != nil {
			BadRequestError(w, req, errors.WithStack(err))
			return
		}
		search := makeSearch(req.Form)
		w.Header().Set("Location", "/d?"+search.Encode())
		w.WriteHeader(http.StatusSeeOther)
	}
}
