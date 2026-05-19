// Package peerdb provides a collaboration platform (database and application framework).
package peerdb

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// Service is the main HTTP service for PeerDB.
type Service struct {
	waf.Service[*Site]

	// Is service running in development mode.
	Development bool

	// Auth verifies OIDC bearer tokens on API requests. It is nil when OIDC
	// authentication is not configured; handlers should treat that as "no
	// authentication available" rather than always-allow or always-deny.
	Auth *auth.Verifier

	// DocumentHooks are called in order to allow for modification of documents
	// before they are send to the client.
	DocumentHooks []func(doc *document.D) (*document.D, errors.E)
}

// Init initializes the HTTP service and is used together with Prepare to implement Run.
func (c *ServeCommand) Init(ctx context.Context, globals *Globals, files fs.FS) (*Service, func(), errors.E) {
	c.Server.Logger = globals.Logger

	sites := map[string]*Site{}
	for i := range globals.Sites {
		site := &globals.Sites[i]

		sites[site.Domain] = site
	}

	if len(sites) == 0 && c.Domain != "" {
		// If sites are not provided, but default domain is,
		// we create a site based on the default domain.
		globals.Sites = []Site{{
			Site: waf.Site{
				Domain:   c.Domain,
				CertFile: "",
				KeyFile:  "",
			},
			Build:             nil,
			Index:             globals.Elastic.Index,
			Schema:            globals.Postgres.Schema,
			Title:             c.Title,
			Logo:              "",
			LanguagePriority:  nil,
			DefaultLanguage:   "",
			LanguageCodes:     nil,
			Features:          SiteFeatures{},
			OIDC:              nil,
			Base:              nil,
			DBPool:            nil,
			ESClient:          nil,
			RiverClient:       nil,
			debugRiverHandler: nil,
			initialized:       false,
			propertiesTotal:   0,
			unitsTotal:        0,
		}}
		sites[c.Domain] = &globals.Sites[0]
	}

	// If sites are not provided (and no default domain), sites
	// are automatically constructed based on the certificate.
	sitesProvided := len(sites) > 0
	sites, errE := c.Server.Init(sites)
	if errE != nil {
		return nil, nil, errE
	}

	if !sitesProvided {
		// We set fields not set when sites are automatically constructed.
		for domain, site := range sites {
			site.Index = globals.Elastic.Index
			site.Schema = globals.Postgres.Schema
			site.Title = c.Title
			// We copy the site to globals.Sites.
			globals.Sites = append(globals.Sites, *site)
			// And then we update the reference to this copy.
			sites[domain] = &globals.Sites[len(globals.Sites)-1]
		}
	}

	// We set build information on sites.
	if cli.Version != "" || cli.BuildTimestamp != "" || cli.Revision != "" {
		for _, site := range sites {
			site.Build = &Build{
				Version:        cli.Version,
				BuildTimestamp: cli.BuildTimestamp,
				Revision:       cli.Revision,
			}
		}
	}

	onShutdown, errE := Init(ctx, globals)
	if errE != nil {
		return nil, onShutdown, errE
	}

	var middleware []func(http.Handler) http.Handler

	if c.Username != "" && c.Password != nil {
		globals.Logger.Info().Str("username", c.Username).Msg("authentication enabled for all sites")

		middleware = append(middleware, basicAuthHandler(c.Username, strings.TrimSpace(string(c.Password))))
	}

	var verifier *auth.Verifier
	if c.Auth.Issuer != "" {
		verifier, errE = auth.New(ctx, c.Auth.Issuer, c.Auth.ClientID)
		if errE != nil {
			return nil, onShutdown, errE
		}
		globals.Logger.Info().Str("issuer", c.Auth.Issuer).Str("clientId", c.Auth.ClientID).Msg("OIDC authentication enabled")

		// We attach the OIDC middleware last so the bearer token is verified
		// after any preceding gate (e.g. basicAuth) has already let the request
		// through. The middleware is permissive - it never rejects on its own -
		// so handlers that require a signed-in caller still have to call
		// Verifier.RequireAuthenticated explicitly; handlers that adapt to who
		// is signed in can just read auth.Subject / auth.Roles from ctx.
		middleware = append(middleware, verifier.Middleware())
	}

	// We populate per-site OIDC context so the frontend knows how to start a
	// sign-in flow. Each site reuses the global OIDC config but gets its own
	// redirect URI rooted in its domain.
	if verifier != nil {
		for _, site := range sites {
			site.OIDC = &SiteOIDC{
				Issuer:   verifier.Issuer(),
				ClientID: verifier.ClientID(),
				// TODO: Allow setting external port.
				RedirectURI: "https://" + site.Domain + "/",
			}
		}
	}

	service := &Service{ //nolint:forcetypeassert
		Service: waf.Service[*Site]{
			Logger:          globals.Logger,
			CanonicalLogger: globals.Logger,
			WithContext:     globals.WithContext,
			StaticFiles:     files.(fs.ReadFileFS), //nolint:errcheck
			Routes:          nil,
			Sites:           sites,
			Middleware:      middleware,
			SiteContextPath: "/context.json",
			RoutesPath:      "/routes.json",
			ProxyStaticTo:   c.Server.ProxyToInDevelopment(),
			SkipServingFile: func(path string) bool {
				switch path {
				case "/index.html":
					// We want the file to be served by Home route at / and not be
					// available at index.html (as well).
					return true
				case "/LICENSE.txt":
					// We want the file to be served by License route at /LICENSE and not be
					// available at LICENSE.txt (as well).
					return true
				case "/NOTICE.txt":
					// We want the file to be served by Notice route at /NOTICE and not be
					// available at NOTICE.txt (as well).
					return true
				default:
					return false
				}
			},
		},
		Development: c.Server.Development,
		Auth:        verifier,
	}

	service.setRoutes()

	return service, onShutdown, nil
}

// Prepare prepares the HTTP service for serving.
func (c *ServeCommand) Prepare(ctx context.Context, service *Service) (http.Handler, func(), errors.E) {
	onShutdown := []func(){}
	onShutdownF := func() {
		for _, f := range onShutdown {
			if f == nil {
				continue
			}
			f()
		}
	}

	for _, site := range service.Sites {
		siteCtx := WithFallbackDBContext(ctx, site.Schema, "prepare")

		documents, errE := site.fetchDocuments(siteCtx, internalCore.PropertyClassID)
		if errE != nil {
			return nil, onShutdownF, errE
		}
		languages, errE := site.fetchDocuments(siteCtx, internalCore.LanguageClassID)
		if errE != nil {
			return nil, onShutdownF, errE
		}

		documents = append(documents, languages...)

		onS, errE := site.Start(siteCtx, documents)
		onShutdown = append(onShutdown, onS)
		if errE != nil {
			return nil, onShutdownF, errE
		}

		c.Server.Logger.Info().Str("domain", site.Domain).Str("index", site.Index).Str("schema", site.Schema).Msg("serving")
	}

	// Construct the main handler for the service using the router.
	router := new(waf.Router)
	handler, errE := service.RouteWith(router)
	return handler, onShutdownF, errE
}

// Run starts the HTTP server and serves the PeerDB application.
func (c *ServeCommand) Run(globals *Globals, files fs.FS) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = globals.Logger.WithContext(ctx)

	ctx, cancel := context.WithCancel(ctx)

	service, initShutdown, errE := c.Init(ctx, globals, files)
	if initShutdown != nil {
		defer initShutdown()
	}
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()
	if errE != nil {
		return errE
	}

	handler, prepareShutdown, errE := c.Prepare(ctx, service)
	if prepareShutdown != nil {
		defer prepareShutdown()
	}
	// It is safe to call cancel multiple times. We want it to be
	// called before any onShutdown waits.
	defer cancel()
	if errE != nil {
		return errE
	}

	// It returns only on error or if the server is gracefully shut down using ctrl-c.
	return c.Server.Run(ctx, handler)
}
