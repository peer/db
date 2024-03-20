package peerdb

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/search"
)

//go:embed routes.json
var routesConfiguration []byte

//go:embed dist
var files embed.FS

type Service struct {
	waf.Service[*Site]

	ESClient *elastic.Client
}

// Init is used primarily in tests. Use Run otherwise.
func (c *ServeCommand) Init(globals *Globals, files fs.ReadFileFS) (http.Handler, *Service, errors.E) {
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
			Title:           c.Title,
			SizeField:       globals.SizeField,
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

	esClient, errE := search.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic)
	if errE != nil {
		return nil, nil, errE
	}

	service := &Service{
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
		ESClient: esClient,
	}

	errE = service.populateProperties(context.Background())
	if errE != nil {
		return nil, nil, errE
	}

	router := new(waf.Router)
	// EncodeQuery should match implementation on the frontend.
	router.EncodeQuery = func(query url.Values) string {
		if len(query) == 0 {
			return ""
		}

		// We want keys in an alphabetical order (default in Go).
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// We want the order of parameters to be "s", "at", and then "q" so that if "q" is cut,
		// URL still works. So we just bring "s" to the front.
		i := slices.Index(keys, "s")
		if i >= 0 {
			keys = slices.Delete(keys, i, i+1)
			keys = slices.Insert(keys, 0, "s")
		}

		var buf strings.Builder
		for _, k := range keys {
			vs := query[k]
			keyEscaped := url.QueryEscape(k)
			for _, v := range vs {
				if buf.Len() > 0 {
					buf.WriteByte('&')
				}
				buf.WriteString(keyEscaped)
				buf.WriteByte('=')
				buf.WriteString(url.QueryEscape(v))
			}
		}

		return buf.String()
	}

	// Construct the main handler for the service using the router.
	handler, errE := service.RouteWith(service, router)
	if errE != nil {
		return nil, nil, errE
	}

	return handler, service, nil
}

func (c *ServeCommand) Run(globals *Globals) errors.E {
	handler, _, errE := c.Init(globals, files)
	if errE != nil {
		return errE
	}

	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// It returns only on error or if the server is gracefully shut down using ctrl-c.
	return c.Server.Run(ctx, handler)
}
