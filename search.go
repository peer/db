package peerdb

import (
	"context"
	"io"
	"net/http"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
)

// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?

// UpdatePropertiesTotal updates internal count of number of all properties for each site.
func (s *Service) UpdatePropertiesTotal(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.rel.prop.id", "CAfaL1ZZs6L4uyFdrJZ2wN"), // TYPE.
		elastic.NewTermQuery("claims.rel.to.id", "HohteEmv2o7gPRnJ5wukVe"),   // PROPERTY.
	)
	query := elastic.NewNestedQuery("claims.rel", boolQuery)

	for _, site := range s.Sites {
		total, err := site.ESClient.Count(site.Index).Query(query).Do(ctx)
		if err != nil {
			return errors.Errorf(`site "%s": %w`, site.Index, err)
		}
		site.propertiesTotal = total
	}

	return nil
}

func (s *Service) getSearchService(req *http.Request) (*elastic.SearchService, int64) {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return site.ESClient.Search(site.Index).FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).TrackTotalHits(true).AllowPartialSearchResults(false), site.propertiesTotal
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() (*elastic.SearchService, int64) {
	return func() (*elastic.SearchService, int64) {
		return s.getSearchService(req)
	}
}

// SearchAmountFilterGet handles GET requests for amount filter search endpoints.
func (s *Service) SearchAmountFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.AmountFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop, params["unit"])
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

// SearchRelFilterGet handles GET requests for relation filter search endpoints.
//
//nolint:dupl
func (s *Service) SearchRelFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

// SearchStringFilterGet handles GET requests for string filter search endpoints.
//
//nolint:dupl
func (s *Service) SearchStringFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.StringFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
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

// SearchTimeFilterGet handles GET requests for time filter search endpoints.
//
//nolint:dupl
func (s *Service) SearchTimeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

// SearchGet is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
func (s *Service) SearchGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchSession).Start()
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

	s.Home(w, req, nil)
}

// SearchGetGet is a GET/HEAD HTTP request API handler which returns a search session.
func (s *Service) SearchGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchSession).Start()
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

// SearchFiltersGet is a GET/HEAD HTTP request API handler which returns filters available for the search session.
func (s *Service) SearchFiltersGet(w http.ResponseWriter, req *http.Request, params waf.Params) { //nolint:dupl
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchSession).Start()
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

// SearchResultsGet is a GET/HEAD HTTP request API handler and it searches ElasticSearch index for the provided
// search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchResultsGet(w http.ResponseWriter, req *http.Request, params waf.Params) { //nolint:dupl
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchSession).Start()
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

// SearchJustResultsPost is a POST HTTP request API handler and it searches ElasticSearch index without
// creating a search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchJustResultsPost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
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

// SearchCreatePost is a POST HTTP API request handler which creates a new search session.
func (s *Service) SearchCreatePost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
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

	m := metrics.Duration(internal.MetricSearchSession).Start()
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

// SearchUpdatePost is a POST HTTP API request handler which updates the search session.
func (s *Service) SearchUpdatePost(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	m := metrics.Duration(internal.MetricSearchSession).Start()
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
