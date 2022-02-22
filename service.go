package search

import (
	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
)

type Service struct {
	ESClient *elastic.Client
	Log      zerolog.Logger
}

func (s *Service) RouteWith(router *httprouter.Router) {
	router.GET("/d", s.ListGet)
	router.HEAD("/d", s.ListGet)
	router.POST("/d", s.ListPost)
	router.GET("/d/:id", s.Get)
	router.HEAD("/d/:id", s.Get)
}
