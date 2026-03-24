package peerdb

import (
	"io"
	"net/http"

	essearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
)

func (s *Service) getSearchService(req *http.Request) (*essearch.Search, int64, int64) {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return site.ESClient.Search().Index(site.Index).
		Source_(esdsl.NewSourceConfig().Bool(false)).
		Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).
		TrackTotalHits(esdsl.NewTrackHits().Bool(true)).
		AllowPartialSearchResults(false), site.propertiesTotal, site.unitsTotal
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() (*essearch.Search, int64, int64) {
	return func() (*essearch.Search, int64, int64) {
		return s.getSearchService(req)
	}
}

// SearchAmountFilterGetAPI handles GET requests for amount filter search endpoints.
func (s *Service) SearchAmountFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	var unit *identifier.Identifier
	if unitStr, ok := params["unit"]; ok {
		u, errE := identifier.MaybeString(unitStr)
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, `"unit" is not a valid identifier`))
			return
		}
		unit = &u
	}

	data, metadata, errE := search.AmountFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop, unit)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchRelFilterGetAPI handles GET requests for relation filter search endpoints.
//
//nolint:dupl
func (s *Service) SearchRelFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.RelFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchTimeFilterGetAPI handles GET requests for time filter search endpoints.
//
//nolint:dupl
func (s *Service) SearchTimeFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.TimeFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchGetGet is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
func (s *Service) SearchGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	_, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		// TODO: We should show some nice 404 error page here.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		// TODO: We should show some nice 500 error page here.
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// SearchGetGetAPI is a GET/HEAD HTTP request API handler which returns a search session.
func (s *Service) SearchGetGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, searchSession, nil)
}

// SearchFiltersGetAPI is a GET/HEAD HTTP request API handler which returns filters available for the search session.
func (s *Service) SearchFiltersGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) { //nolint:dupl
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.FiltersGet(ctx, s.getSearchServiceClosure(req), searchSession)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchResultsGetAPI is a GET/HEAD HTTP request API handler and it searches ElasticSearch index for the provided
// search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchResultsGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) { //nolint:dupl
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), searchSession)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchJustResultsPostAPI is a POST HTTP request API handler and it searches ElasticSearch index without
// creating a search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchJustResultsPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	var searchSession search.Session
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &searchSession)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	if searchSession.ID != nil {
		s.BadRequestWithError(w, req, errors.New("payload contains ID"))
		return
	}

	errE = searchSession.Validate(ctx, nil)
	if errE != nil {
		errE = errors.WrapWith(errE, search.ErrValidationFailed)
		s.BadRequestWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchSession)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchCreatePostAPI is a POST HTTP API request handler which creates a new search session.
func (s *Service) SearchCreatePostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	var searchSession search.Session
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &searchSession)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	if searchSession.ID != nil {
		s.BadRequestWithError(w, req, errors.New("payload contains ID"))
		return
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.CreateSession(ctx, &searchSession)
	m.Stop()
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, searchSession.Ref(), nil)
}

// SearchUpdatePostAPI is a POST HTTP API request handler which updates the search session.
func (s *Service) SearchUpdatePostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	var searchSession search.Session
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &searchSession)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	// If searchSession.ID == nil, UpdateSession returns an error.
	if searchSession.ID != nil && params["id"] != searchSession.ID.String() {
		errE = errors.New("params ID does not match payload ID")
		errors.Details(errE)["params"] = params["id"]
		errors.Details(errE)["payload"] = *searchSession.ID
		s.BadRequestWithError(w, req, errE)
		return
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.UpdateSession(ctx, &searchSession)
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, searchSession.Ref(), nil)
}
