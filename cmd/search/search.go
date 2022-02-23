package main

import (
	"log"
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

const (
	listenAddr = ":8080"
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
	handler := s.RouteWith(router)

	// TODO: Implement graceful shutdown.
	server := &http.Server{
		Addr:        listenAddr,
		Handler:     handler,
		ErrorLog:    log.New(config.Log, "", 0),
		ConnContext: s.ConnContext,
	}

	config.Log.Info().Msgf("starting on %s", listenAddr)

	return errors.WithStack(server.ListenAndServeTLS(config.CertFile, config.KeyFile))
}
