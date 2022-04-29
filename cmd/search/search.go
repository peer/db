package main

import (
	"log"
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/cli"
)

const (
	listenAddr = ":8080"
)

func listen(config *Config) errors.E {
	esClient, err := search.GetClient(cleanhttp.DefaultPooledClient(), config.Log, config.Elastic)
	if err != nil {
		return err
	}

	development := config.ProxyTo
	if !config.Development {
		development = ""
	}

	s := &search.Service{
		ESClient:    esClient,
		Log:         config.Log,
		Development: development,
	}

	router := httprouter.New()
	handler, err := s.RouteWith(router, cli.Version)
	if err != nil {
		return err
	}

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
