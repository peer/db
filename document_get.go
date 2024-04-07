package peerdb

import (
	"net/http"
	"net/url"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Support slug per document.

// TODO: Support "version" query string to fetch an exact version.

// DocumentGet is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We validate "s" and "q" parameters.
	if req.Form.Has("s") || req.Form.Has("q") {
		var q *string
		if req.Form.Has("q") {
			qq := req.Form.Get("q")
			q = &qq
		}

		m := metrics.Duration(internal.MetricSearchState).Start()
		sh := search.GetState(req.Form.Get("s"), q)
		m.Stop()
		if sh == nil {
			// Something was not OK, so we redirect to the URL without both "s" and "q".
			path, err := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, url.Values{"tab": req.Form["tab"]})
			if err != nil {
				s.InternalServerErrorWithError(w, req, err)
				return
			}
			// TODO: Should we already do the query, to warm up ES cache?
			//       Maybe we should cache response ourselves so that we do not hit ES twice?
			w.Header().Set("Location", path)
			w.WriteHeader(http.StatusSeeOther)
			return
		} else if req.Form.Has("q") {
			// We redirect to the URL without "q".
			path, err := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, url.Values{"s": {sh.ID.String()}, "tab": req.Form["tab"]})
			if err != nil {
				s.InternalServerErrorWithError(w, req, err)
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
	st := s.getStore(req)

	m := metrics.Duration(internal.MetricDatabase).Start()
	// TODO: Add API to store to just check if the value exists.
	// TODO: To support "omni" instances, allow getting across multiple schemas.
	_, _, _, errE = st.GetCurrent(ctx, id) //nolint:dogsled
	m.Stop()

	if errE != nil {
		if errors.Is(errE, store.ErrValueNotFound) {
			s.NotFound(w, req)
			return
		}
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.Home(w, req, nil)
}

// DocumentGetGet is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We do not check "s" and "q" parameters because the expectation is that
	// they are not provided with JSON request (because they are not used).

	st := s.getStore(req)

	m := metrics.Duration(internal.MetricDatabase).Start()
	// TODO: To support "omni" instances, allow getting across multiple schemas.
	data, metadataJSON, version, errE := st.GetCurrent(ctx, id)
	m.Stop()

	if errE != nil {
		if errors.Is(errE, store.ErrValueNotFound) {
			s.NotFound(w, req)
			return
		}
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	var metadata map[string]interface{}
	errE = x.Unmarshal(metadataJSON, &metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	// TODO: We should return a version of the document with the response and requesting same version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "max-age=604800")

	s.WriteJSON(w, req, data, metadata)
}
