package search

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

// TODO: Support slug per document.
// TODO: JSON response should include _id field.

// DocumentGetGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGetGetHTML(w http.ResponseWriter, req *http.Request, params Params) {
	site, errE := s.getSite(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["id"]
	if !identifier.Valid(id) {
		s.badRequestWithError(w, req, errors.New(`"id" parameter is not a valid identifier`))
		return
	}

	// We validate "s" and "q" parameters.
	if req.Form.Has("s") || req.Form.Has("q") {
		m := timing.NewMetric("s").Start()
		sh := getSearch(req.Form)
		m.Stop()
		if sh == nil {
			// Something was not OK, so we redirect to the URL without both "s" and "q".
			path, err := s.Router.Path("DocumentGet", Params{"id": id}, "")
			if err != nil {
				s.internalServerErrorWithError(w, req, err)
				return
			}
			// TODO: Should we already do the query, to warm up ES cache?
			//       Maybe we should cache response ourselves so that we do not hit ES twice?
			w.Header().Set("Location", path)
			w.WriteHeader(http.StatusSeeOther)
			return
		} else if req.Form.Has("q") {
			// We redirect to the URL without "q".
			path, err := s.Router.Path("DocumentGet", Params{"id": id}, url.Values{"s": {sh.ID}}.Encode())
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
	}

	// TODO: If "s" is provided, should we validate that id is really part of search? Currently we do on the frontend.

	// We check if document exists.
	headers := http.Header{}
	headers.Set("X-Opaque-ID", idFromRequest(req))
	m := timing.NewMetric("es").Start()
	_, err := s.ESClient.PerformRequest(ctx, elastic.PerformRequestOptions{
		Method:  "HEAD",
		Path:    fmt.Sprintf("/%s/_doc/%s", site.Index, id),
		Headers: headers,
	})
	m.Stop()
	if elastic.IsNotFound(err) {
		s.NotFound(w, req, nil)
		return
	} else if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	if s.Development != "" {
		s.Proxy(w, req, nil)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}

// DocumentGetGetJSON is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetGetJSON(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, []string{compressionGzip, compressionDeflate, compressionIdentity})
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	site, errE := s.getSite(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["id"]
	if !identifier.Valid(id) {
		s.badRequestWithError(w, req, errors.New(`"id" parameter is not a valid identifier`))
		return
	}

	// We do not check "s" and "q" parameters because the expectation is that
	// they are not provided with JSON request (because they are not used).

	headers := http.Header{}
	headers.Set("Accept-Encoding", contentEncoding)
	headers.Set("X-Opaque-ID", idFromRequest(req))
	m := timing.NewMetric("es").Start()
	resp, err := s.ESClient.PerformRequest(ctx, elastic.PerformRequestOptions{
		Method:  "GET",
		Path:    fmt.Sprintf("/%s/_doc/%s", site.Index, id),
		Headers: headers,
	})
	m.Stop()
	if elastic.IsNotFound(err) {
		s.NotFound(w, req, nil)
		return
	} else if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	hash := sha256.Sum256(resp.Body)
	etag := `"` + base64.RawURLEncoding.EncodeToString(hash[:]) + `"`

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if contentEncoding != compressionIdentity {
		w.Header().Set("Content-Encoding", contentEncoding)
	} else {
		// TODO: Always set Content-Length.
		//       See: https://github.com/golang/go/pull/50904
		w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	}
	// TODO: We should return a version of the document with the response and requesting same version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "max-age=604800")
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(resp.Body))
}
