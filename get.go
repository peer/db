package search

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	gddo "github.com/golang/gddo/httputil"
	"github.com/julienschmidt/httprouter"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

// TODO: Support slug per document.

// DocumentGetJSON is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetJSON(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := ps.ByName("id")
	if !identifier.Valid(id) {
		http.Error(w, "400 bad request", http.StatusBadRequest)
	}

	contentEncoding := gddo.NegotiateContentEncoding(req, []string{compressionGzip, compressionDeflate, compressionIdentity})
	if contentEncoding == "" {
		http.Error(w, "406 not acceptable", http.StatusNotAcceptable)
		return
	}

	headers := http.Header{}
	headers.Set("Accept-Encoding", contentEncoding)
	headers.Set("X-Opaque-ID", idFromRequest(req))
	m := timing.NewMetric("es").Start()
	resp, err := s.ESClient.PerformRequest(ctx, elastic.PerformRequestOptions{
		Method:  "GET",
		Path:    fmt.Sprintf("/docs/_source/%s", id),
		Headers: headers,
	})
	m.Stop()
	if elastic.IsNotFound(err) {
		s.NotFound(w, req)
		return
	} else if err != nil {
		s.internalServerError(w, req, errors.WithStack(err))
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
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Etag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// See: https://github.com/golang/go/issues/50905
	// See: https://github.com/golang/go/pull/50903
	http.ServeContent(w, req, "", time.Time{}, bytes.NewReader(resp.Body))
}

// DocumentGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGetHTML(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if s.Development != "" {
		s.Proxy(w, req)
	} else {
		// TODO
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
	}
}
