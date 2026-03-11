package peerdb

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

func (s *Service) setRoutes() {
	s.Routes = map[string]waf.Route{
		"Home": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.HomeGet,
				},
			},
			Path: "/",
		},
		"License": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.LicenseGet,
				},
			},
			Path: "/LICENSE",
		},
		"Notice": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.NoticeGet,
				},
			},
			Path: "/NOTICE",
		},
		"SearchFilters": {
			Path: "/s/filters/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchFiltersGetAPI,
				},
			},
		},
		"SearchRelFilter": {
			Path: "/s/filters/:id/rel/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchRelFilterGetAPI,
				},
			},
		},
		"SearchAmountFilter": {
			Path: "/s/filters/:id/amount/:prop/:unit",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchAmountFilterGetAPI,
				},
			},
		},
		"SearchTimeFilter": {
			Path: "/s/filters/:id/time/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchTimeFilterGetAPI,
				},
			},
		},
		"SearchStringFilter": {
			Path: "/s/filters/:id/string/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchStringFilterGetAPI,
				},
			},
		},
		"SearchCreate": {
			Path: "/s/create",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.SearchCreatePostAPI,
				},
			},
		},
		"SearchGet": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchGetGet,
				},
			},
			Path: "/s/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchGetGetAPI,
				},
			},
		},
		"SearchJustResults": {
			Path: "/s/results",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.SearchJustResultsPostAPI,
				},
			},
		},
		"SearchResults": {
			Path: "/s/results/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchResultsGetAPI,
				},
			},
		},
		"SearchUpdate": {
			Path: "/s/update/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.SearchUpdatePostAPI,
				},
			},
		},
		"DocumentCreate": {
			Path: "/d/create",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.DocumentCreatePostAPI,
				},
			},
		},
		"DocumentBeginEdit": {
			Path: "/d/beginEdit/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.DocumentBeginEditPostAPI,
				},
			},
		},
		"DocumentSaveChange": {
			Path: "/d/saveChange/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.DocumentSaveChangePostAPI,
				},
			},
		},
		"DocumentListChanges": {
			Path: "/d/listChanges/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentListChangesGetAPI,
				},
			},
		},
		"DocumentGetChange": {
			Path: "/d/getChange/:session/:change",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentGetChangeGetAPI,
				},
			},
		},
		"DocumentEndEdit": {
			Path: "/d/endEdit/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.DocumentEndEditPostAPI,
				},
			},
		},
		"DocumentDiscardEdit": {
			Path: "/d/discardEdit/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.DocumentDiscardEditPostAPI,
				},
			},
		},
		"DocumentEdit": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentEditGet,
				},
			},
			Path: "/d/edit/:id/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentEditGetAPI,
				},
			},
		},
		"DocumentGet": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentGetGet,
				},
			},
			Path: "/d/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentGetGetAPI,
				},
			},
		},
		"StorageBeginUpload": {
			Path: "/f/beginUpload",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.StorageBeginUploadPostAPI,
				},
			},
		},
		"StorageUploadChunk": {
			Path: "/f/uploadChunk/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.StorageUploadChunkPostAPI,
				},
			},
		},
		"StorageListChunks": {
			Path: "/f/listChunks/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageListChunksGetAPI,
				},
			},
		},
		"StorageGetChunk": {
			Path: "/f/getChunk/:session/:chunk",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageGetChunkGetAPI,
				},
			},
		},
		"StorageEndUpload": {
			Path: "/f/endUpload/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.StorageEndUploadPostAPI,
				},
			},
		},
		"StorageDiscardUpload": {
			Path: "/f/discardUpload/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.StorageDiscardUploadPostAPI,
				},
			},
		},
		"StorageGet": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageGet,
				},
			},
			Path: "/f/:id",
		},
	}

	// We add debugging routes only in development mode.
	if s.Development {
		s.Routes["DebugMapping"] = waf.Route{
			Path: "/debug/mapping",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DebugMappingGetAPI,
				},
			},
		}
	}
}
