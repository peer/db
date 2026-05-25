package peerdb

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

func (s *Service) setRoutes() { //nolint:maintidx
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
		"SearchFilterGet": {
			Path: "/s/filters/:id/:filter",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchFilterGetAPI,
				},
			},
		},
		"SearchSubRefFilter": {
			Path: "/s/filters/:id/ref/:parentProp/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchSubRefFilterGetAPI,
				},
			},
		},
		"SearchRefFilter": {
			Path: "/s/filters/:id/ref/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchRefFilterGetAPI,
				},
			},
		},
		"SearchAmountFilterWithUnit": {
			Path: "/s/filters/:id/amount/:prop/:unit",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchAmountFilterGetAPI,
				},
			},
		},
		"SearchAmountFilter": {
			Path: "/s/filters/:id/amount/:prop",
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
		"SearchSubAmountFilterWithUnit": {
			Path: "/s/filters/:id/amount/:parentProp/:prop/:unit",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchSubAmountFilterGetAPI,
				},
			},
		},
		"SearchSubAmountFilter": {
			Path: "/s/filters/:id/amount/:parentProp/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchSubAmountFilterGetAPI,
				},
			},
		},
		"SearchSubTimeFilter": {
			Path: "/s/filters/:id/time/:parentProp/:prop",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchSubTimeFilterGetAPI,
				},
			},
		},
		"SearchHasFilter": {
			Path: "/s/filters/:id/has",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchHasFilterGetAPI,
				},
			},
		},
		"SearchSubHasFilter": {
			Path: "/s/filters/:id/has/:parentProp",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchSubHasFilterGetAPI,
				},
			},
		},
		"SearchShortcut": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.SearchShortcutGet,
				},
			},
			Path: "/s",
		},
		"SearchCreate": {
			Path: "/s/create",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.SearchCreatePostAPI,
				},
			},
		},
		"SearchJustResults": {
			Path: "/s/results",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet:  s.SearchJustResultsGetAPI,
					http.MethodPost: s.SearchJustResultsPostAPI,
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
		"DocumentChanges": {
			Path: "/d/changes/:changeset",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentChangesGetAPI,
				},
			},
		},
		"DocumentChangesGet": {
			Path: "/d/changes/:changeset/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DocumentChangesGetGetAPI,
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
		"StorageUpload": {
			Path: "/f/upload/:session",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageUploadGetAPI,
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
		"StorageChanges": {
			Path: "/f/changes/:changeset",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageChangesGetAPI,
				},
			},
		},
		"StorageChangesGet": {
			Path: "/f/changes/:changeset/:id",
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageChangesGetGet,
				},
			},
		},
		"StorageGet": {
			Path: "/f/:id",
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.StorageGetGet,
				},
			},
		},
		"AuthSignIn": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.AuthSignInGet,
				},
			},
			Path: "/auth/signIn",
		},
		"AuthCallback": {
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.AuthCallbackGet,
				},
			},
			Path: "/auth/callback",
		},
		"AuthSignOut": {
			Path: "/auth/signOut",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodPost: s.AuthSignOutPostAPI,
				},
			},
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
		s.Routes["DebugIndexed"] = waf.Route{
			Path: "/debug/indexed/:id",
			API: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet: s.DebugIndexedGetAPI,
				},
			},
		}
		s.Routes["DebugRiver"] = waf.Route{
			Path: debugRiverPrefix + "/:path*",
			RouteOptions: waf.RouteOptions{
				Handlers: map[string]waf.Handler{
					http.MethodGet:   s.DebugRiver,
					http.MethodPost:  s.DebugRiver,
					http.MethodPut:   s.DebugRiver,
					http.MethodPatch: s.DebugRiver,
				},
			},
		}
	}
}
