package peerdb

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/store"

	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

//go:embed routes.json
var routesConfiguration []byte

//go:embed dist
var files embed.FS

// TODO: Determine reasonable size for the buffer.
const bridgeBufferSize = 100

type Service struct {
	waf.Service[*Site]

	esClient *elastic.Client
}

// Init is used primarily in tests. Use Run otherwise.
func (c *ServeCommand) Init(ctx context.Context, globals *Globals, files fs.ReadFileFS) (http.Handler, *Service, errors.E) {
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
	for _, site := range globals.Sites {
		site := site
		sites[site.Domain] = &site
	}

	if len(sites) == 0 && c.Domain != "" {
		sites[c.Domain] = &Site{
			Site: waf.Site{
				Domain:   c.Domain,
				CertFile: "",
				KeyFile:  "",
			},
			Build:           nil,
			Index:           globals.Index,
			Schema:          globals.Schema,
			Title:           c.Title,
			SizeField:       globals.SizeField,
			store:           nil,
			esProcessor:     nil,
			propertiesTotal: 0,
		}
	}

	// If sites are not provided, sites are automatically constructed based on the certificate.
	sitesProvided := len(sites) > 0
	sites, errE = c.Server.Init(sites)
	if errE != nil {
		return nil, nil, errE
	}

	if !sitesProvided {
		// We set fields not set when sites are automatically constructed.
		for _, site := range sites {
			site.Index = globals.Index
			site.Schema = globals.Schema
			site.Title = c.Title
			site.SizeField = globals.SizeField
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

	// We remove "dist" prefix.
	f, err := fs.Sub(files, "dist")
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	dbpool, errE := internal.InitPostgres(ctx, string(globals.Database), globals.Logger, func(ctx context.Context) (string, string) {
		requestID := waf.MustRequestID(ctx)
		site := waf.MustGetSite[*Site](ctx)
		return site.Schema, requestID.String()
	})
	if errE != nil {
		return nil, nil, errE
	}

	esClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic)
	if errE != nil {
		return nil, nil, errE
	}

	for _, site := range sites {
		// TODO: Add some monitoring of the channel contention.
		channel := make(chan store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage], bridgeBufferSize)
		context.AfterFunc(ctx, func() { close(channel) })
		esProcessor, errE := es.Init(ctx, globals.Logger, esClient, site.Index, site.SizeField) //nolint:govet
		if errE != nil {
			return nil, nil, errE
		}
		store := &store.Store[json.RawMessage, json.RawMessage, json.RawMessage]{
			Schema:    site.Schema,
			Committed: channel,
		}
		errE = store.Init(ctx, dbpool)
		if errE != nil {
			return nil, nil, errE
		}
		site.store = store
		site.esProcessor = esProcessor
		go es.Bridge(
			ctx,
			globals.Logger.With().Str("schema", site.Schema).Str("index", site.Index).Logger(),
			store,
			esProcessor,
			site.Index,
			channel,
		)
	}

	service := &Service{ //nolint:forcetypeassert
		Service: waf.Service[*Site]{
			Logger:          globals.Logger,
			CanonicalLogger: globals.Logger,
			WithContext:     globals.WithContext,
			StaticFiles:     f.(fs.ReadFileFS),
			Routes:          routesConfig.Routes,
			Sites:           sites,
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
		esClient: esClient,
	}

	errE = service.populatePropertiesTotal(ctx)
	if errE != nil {
		return nil, nil, errE
	}

	// Construct the main handler for the service using the router.
	router := new(waf.Router)
	handler, errE := service.RouteWith(service, router)
	if errE != nil {
		return nil, nil, errE
	}

	return handler, service, nil
}

func (c *ServeCommand) Run(globals *Globals) errors.E {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler, _, errE := c.Init(ctx, globals, files)
	if errE != nil {
		return errE
	}

	// It returns only on error or if the server is gracefully shut down using ctrl-c.
	return c.Server.Run(ctx, handler)
}
