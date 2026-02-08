// Package peerdb provides a collaboration platform (database and application framework).
package peerdb

import (
	"context"
	_ "embed"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"
)

//go:embed routes.json
var routesConfiguration []byte

// Service is the main HTTP service for PeerDB.
type Service struct {
	waf.Service[*Site]
}

// Init initializes the HTTP service and is used primarily in tests. Use Run otherwise.
func (c *ServeCommand) Init(ctx context.Context, globals *Globals, files fs.FS) (http.Handler, *Service, errors.E) {
	// Routes come from a single source of truth, e.g., a file.
	var routesConfig struct {
		Routes []waf.Route `json:"routes"`
	}
	errE := x.UnmarshalWithoutUnknownFields(routesConfiguration, &routesConfig)
	if errE != nil {
		return nil, nil, errE
	}

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
			Build:           nil,
			Index:           globals.Elastic.Index,
			Schema:          globals.Postgres.Schema,
			Title:           c.Title,
			Store:           nil,
			Coordinator:     nil,
			Storage:         nil,
			ESProcessor:     nil,
			ESClient:        nil,
			DBPool:          nil,
			propertiesTotal: 0,
		}}
		sites[c.Domain] = &globals.Sites[0]
	}

	// If sites are not provided (and no default domain), sites
	// are automatically constructed based on the certificate.
	sitesProvided := len(sites) > 0
	sites, errE = c.Server.Init(sites)
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

	errE = Init(ctx, globals)
	if errE != nil {
		return nil, nil, errE
	}

	var middleware []func(http.Handler) http.Handler

	if c.Username != "" && c.Password != nil {
		middleware = append(middleware, basicAuthHandler(c.Username, strings.TrimSpace(string(c.Password))))
		globals.Logger.Info().Str("username", c.Username).Msg("authentication enabled for all sites")
	}

	service := &Service{ //nolint:forcetypeassert
		Service: waf.Service[*Site]{
			Logger:          globals.Logger,
			CanonicalLogger: globals.Logger,
			WithContext:     globals.WithContext,
			StaticFiles:     files.(fs.ReadFileFS), //nolint:errcheck
			Routes:          routesConfig.Routes,
			Sites:           sites,
			Middleware:      middleware,
			SiteContextPath: "/context.json",
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
	}

	// Construct the main handler for the service using the router.
	router := new(waf.Router)
	handler, errE := service.RouteWith(service, router)
	if errE != nil {
		return nil, nil, errE
	}

	return handler, service, nil
}

// Run starts the HTTP server and serves the PeerDB application.
func (c *ServeCommand) Run(globals *Globals, files fs.FS) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler, service, errE := c.Init(ctx, globals, files)
	if errE != nil {
		return errE
	}

	errE = service.UpdatePropertiesTotal(ctx)
	if errE != nil {
		return errE
	}

	// It returns only on error or if the server is gracefully shut down using ctrl-c.
	return c.Server.Run(ctx, handler)
}
