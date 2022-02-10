package main

import (
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/julienschmidt/httprouter"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func listen(config *Config) errors.E {
	esClient, err := elastic.NewClient(
		elastic.SetHttpClient(cleanhttp.DefaultPooledClient()),
	)
	if err != nil {
		return errors.WithStack(err)
	}

	router := httprouter.New()
	router.GET("/d", search.ListGet(esClient))
	router.HEAD("/d", search.ListGet(esClient))
	router.POST("/d", search.ListPost(esClient))
	router.GET("/d/:id", search.Get(esClient))
	router.HEAD("/d/:id", search.Get(esClient))

	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true
	router.NotFound = http.HandlerFunc(search.NotFound)

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	return errors.WithStack(server.ListenAndServeTLS(config.CertFile, config.KeyFile))
}
