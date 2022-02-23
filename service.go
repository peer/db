package search

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
)

type Service struct {
	ESClient *elastic.Client
	Log      zerolog.Logger
}

func (s *Service) RouteWith(router *httprouter.Router) {
	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true

	router.GET("/d", s.ListGet)
	router.HEAD("/d", s.ListGet)
	router.POST("/d", s.ListPost)
	router.GET("/d/:id", s.Get)
	router.HEAD("/d/:id", s.Get)

	router.NotFound = http.HandlerFunc(s.NotFound)
}
