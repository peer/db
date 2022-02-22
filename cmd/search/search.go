package main

import (
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func listen(config *Config) errors.E {
	esClient, err := search.GetClient(cleanhttp.DefaultPooledClient(), config.Log)
	if err != nil {
		return err
	}

	s := &search.Service{
		ESClient: esClient,
		Log:      config.Log,
	}

	router := httprouter.New()
	router.RedirectTrailingSlash = true
	router.RedirectFixedPath = true
	router.HandleMethodNotAllowed = true
	router.NotFound = http.HandlerFunc(search.NotFound)

	s.RouteWith(router)

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	return errors.WithStack(server.ListenAndServeTLS(config.CertFile, config.KeyFile))
}
