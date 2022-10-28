package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/cli"
)

const (
	listenAddr = ":8080"
)

func (c *ServeCommand) Run(globals *Globals) errors.E {
	esClient, err := search.GetClient(cleanhttp.DefaultPooledClient(), globals.Log, globals.Elastic)
	if err != nil {
		return err
	}

	development := c.ProxyTo
	if !c.Development {
		development = ""
	}

	var fileGetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	var letsEncryptGetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	letsEncryptDomainsList := []string{}
	sites := map[string]search.Site{}

	if len(globals.Sites) > 0 {
		fileGetCertificateFunctions := map[string]func(*tls.ClientHelloInfo) (*tls.Certificate, error){}

		for i, site := range globals.Sites {
			if site.Domain == "" {
				return errors.Errorf(`domain is required for site at index %d`, i)
			}

			if site.CertFile != "" && site.KeyFile != "" {
				manager := CertificateManager{
					CertFile: site.CertFile,
					KeyFile:  site.KeyFile,
					Log:      globals.Log,
				}

				err = manager.Start()
				if err != nil {
					return err
				}
				defer manager.Stop()

				fileGetCertificateFunctions[site.Domain] = manager.GetCertificate
			} else if c.TLS.Email != "" && c.TLS.Cache != "" {
				letsEncryptDomainsList = append(letsEncryptDomainsList, site.Domain)
			} else if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
				manager := CertificateManager{
					CertFile: c.TLS.CertFile,
					KeyFile:  c.TLS.KeyFile,
					Log:      globals.Log,
				}

				err = manager.Start()
				if err != nil {
					return err
				}
				defer manager.Stop()

				fileGetCertificateFunctions[site.Domain] = manager.GetCertificate
			} else {
				return errors.Errorf(`missing file or Let's Encrypt's certificate configuration for site "%s"`, site.Domain)
			}

			if _, ok := sites[site.Domain]; ok {
				return errors.Errorf(`duplicate site for domain "%s"`, site.Domain)
			}
			sites[site.Domain] = search.Site{
				Domain: site.Domain,
				Index:  site.Index,
				Title:  site.Title,
			}
		}

		if len(fileGetCertificateFunctions) > 0 {
			fileGetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				// Note that this conversion is necessary because some server names in the handshakes
				// started by some clients (such as cURL) are not converted to Punycode, which will
				// prevent us from obtaining certificates for them. In addition, we should also treat
				// example.com and EXAMPLE.COM as equivalent and return the same certificate for them.
				// Fortunately, this conversion also helped us deal with this kind of mixedcase problems.
				//
				// Due to the "σςΣ" problem (see https://unicode.org/faq/idn.html#22), we can't use
				// idna.Punycode.ToASCII (or just idna.ToASCII) here.
				name, err := idna.Lookup.ToASCII(hello.ServerName)
				if err != nil {
					return nil, errors.Errorf(`server name contains invalid character: %s`, hello.ServerName)
				}
				f, ok := fileGetCertificateFunctions[name]
				if ok {
					return f(hello)
				}
				return nil, nil //nolint:nilnil
			}
		}
	} else {
		if c.TLS.Domain != "" && c.TLS.Email != "" && c.TLS.Cache != "" {
			letsEncryptDomainsList = append(letsEncryptDomainsList, c.TLS.Domain)
		} else if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
			manager := CertificateManager{
				CertFile: c.TLS.CertFile,
				KeyFile:  c.TLS.KeyFile,
				Log:      globals.Log,
			}

			err = manager.Start()
			if err != nil {
				return err
			}
			defer manager.Stop()

			fileGetCertificate = manager.GetCertificate
		} else {
			return errors.New("missing file or Let's Encrypt's certificate configuration")
		}

		sites[""] = search.Site{
			Domain: "",
			Index:  globals.Index,
			Title:  c.Title,
		}
	}

	if len(letsEncryptDomainsList) > 0 {
		manager := autocert.Manager{
			Cache:      autocert.DirCache(c.TLS.Cache),
			Prompt:     autocert.AcceptTOS,
			Email:      c.TLS.Email,
			HostPolicy: autocert.HostWhitelist(letsEncryptDomainsList...),
		}

		letsEncryptGetCertificate = manager.GetCertificate
	}

	s, err := search.NewService(esClient, globals.Log, cli.Version, cli.BuildTimestamp, cli.Revision, sites, development)
	if err != nil {
		return err
	}

	router := search.NewRouter()
	handler, err := s.RouteWith(router, cli.Version)
	if err != nil {
		return err
	}

	// TODO: Implement graceful shutdown.
	// TODO: Add request timeouts so that malicious client cannot make too slow requests or read too slowly the response.
	//       Currently this is not possible, because ReadTimeout and WriteTimeout count in handler processing time as well.
	//       Moreover, when they timeout, they do not cancel the handler itself. See: https://github.com/golang/go/issues/16100
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ErrorLog:          log.New(globals.Log, "", 0),
		ConnContext:       s.ConnContext,
		ReadHeaderTimeout: time.Minute,
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

	if fileGetCertificate != nil && letsEncryptGetCertificate != nil {
		server.TLSConfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			c, err := fileGetCertificate(hello)
			if err != nil {
				return c, err
			} else if c != nil {
				return c, nil
			}

			return letsEncryptGetCertificate(hello)
		}
		server.TLSConfig.NextProtos = []string{"h2", "http/1.1", acme.ALPNProto}
	} else if fileGetCertificate != nil {
		server.TLSConfig.GetCertificate = fileGetCertificate
	} else if letsEncryptGetCertificate != nil {
		server.TLSConfig.GetCertificate = letsEncryptGetCertificate
		server.TLSConfig.NextProtos = []string{"h2", "http/1.1", acme.ALPNProto}
	} else {
		panic(errors.New("no GetCertificate"))
	}

	globals.Log.Info().Msgf("starting on %s", listenAddr)

	return errors.WithStack(server.ListenAndServeTLS("", ""))
}
