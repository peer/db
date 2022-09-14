package main

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/hashicorp/go-cleanhttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

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

	s, err := search.NewService(esClient, config.Log, config.Index, development)
	if err != nil {
		return err
	}

	router := search.NewRouter()
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
		TLSConfig: &tls.Config{
			MinVersion:       tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			},
		},
	}

	if config.TLS.Static.CertFile != "" && config.TLS.Static.KeyFile != "" {
		manager := CertificateManager{
			CertFile: config.TLS.Static.CertFile,
			KeyFile:  config.TLS.Static.KeyFile,
			Log:      config.Log,
		}

		err = manager.Start()
		if err != nil {
			return err
		}
		defer manager.Stop()

		server.TLSConfig.GetCertificate = manager.GetCertificate
	} else if len(config.TLS.LetsEncrypt.Domain) > 0 && config.TLS.LetsEncrypt.Email != "" && config.TLS.LetsEncrypt.Cache != "" {
		manager := autocert.Manager{
			Cache:      autocert.DirCache(config.TLS.LetsEncrypt.Cache),
			Prompt:     autocert.AcceptTOS,
			Email:      config.TLS.LetsEncrypt.Email,
			HostPolicy: autocert.HostWhitelist(config.TLS.LetsEncrypt.Domain...),
		}

		server.TLSConfig.GetCertificate = manager.GetCertificate
		server.TLSConfig.NextProtos = []string{"h2", "http/1.1", acme.ALPNProto}
	} else {
		return errors.New("missing static certificate or Let's Encrypt's certificate configuration")
	}

	config.Log.Info().Msgf("starting on %s", listenAddr)

	return errors.WithStack(server.ListenAndServeTLS("", ""))
}
