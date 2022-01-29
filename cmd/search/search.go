package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func listen(config *Config) errors.E {
	client, err := elastic.NewClient()
	if err != nil {
		return errors.WithStack(err)
	}

	router := httprouter.New()
	router.GET("/d/:id", search.Get(client))
	router.HEAD("/d/:id", search.Get(client))

	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true
	router.NotFound = search.NotFound

	return errors.WithStack(http.ListenAndServe(":8080", router))
}
