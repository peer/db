package search

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/tozd/identifier"
)

// TODO: Support slug per document.
// TODO: JSON response should include _id field.

// DocumentGet is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGet(w http.ResponseWriter, req *http.Request, params Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.badRequestWithError(w, req, errors.WithMessage(errE, `"id" parameter is not a valid identifier`))
		return
	}

	// We validate "s" and "q" parameters.
	if req.Form.Has("s") || req.Form.Has("q") {
		m := timing.NewMetric("s").Start()
		sh := getSearch(req.Form)
		m.Stop()
		if sh == nil {
			// Something was not OK, so we redirect to the URL without both "s" and "q".
			path, err := s.Router.Path("DocumentGet", Params{"id": id.String()}, url.Values{"tab": req.Form["tab"]}.Encode())
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
			path, err := s.Router.Path("DocumentGet", Params{"id": id.String()}, url.Values{"s": {sh.ID.String()}, "tab": req.Form["tab"]}.Encode())
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
	searchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}
	searchService = searchService.From(0).Size(0).Query(elastic.NewTermQuery("_id", id))

	m := timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	if res.Hits.TotalHits.Value == 0 {
		s.NotFound(w, req, nil)
		return
	}

	s.HomeGet(w, req, nil)
}

// DocumentGetAPIGet is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetAPIGet(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, []string{compressionGzip, compressionDeflate, compressionIdentity})
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.badRequestWithError(w, req, errors.WithMessage(errE, `"id" parameter is not a valid identifier`))
		return
	}

	// We do not check "s" and "q" parameters because the expectation is that
	// they are not provided with JSON request (because they are not used).

	// We do a search query and not _doc or _source request to get the document
	// so that it works also on aliases.
	// See: https://github.com/elastic/elasticsearch/issues/69649
	searchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}
	searchService = searchService.From(0).Size(maxResultsCount).FetchSource(true).Query(elastic.NewTermQuery("_id", id))

	m := timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	if len(res.Hits.Hits) == 0 {
		s.NotFound(w, req, nil)
		return
	} else if len(res.Hits.Hits) > 1 {
		s.Log.Warn().Str("id", id.String()).Msg("found more than one document for ID")
	}

	// TODO: We should return a version of the document with the response and requesting same version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "max-age=604800")

	// ID is not stored in the document, so we set it here ourselves.
	source := bytes.NewBuffer(res.Hits.Hits[0].Source)
	source.Truncate(source.Len() - 1)
	source.WriteString(`,"_id":"`)
	source.WriteString(id.String())
	source.WriteString(`"}`)

	s.writeJSON(w, req, contentEncoding, json.RawMessage(source.Bytes()), nil)
}
