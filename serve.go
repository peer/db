// Package peerdb provides a collaboration platform (database and application framework).
package peerdb

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// authCleanupInterval is how often the per-site auth cleanup job runs.
// Expired rows do not affect correctness. The cleanup job exists purely
// to bound table and index growth, so once a day is more than enough.
// Skipping a run only leaves dead rows lying around for an extra day.
const authCleanupInterval = 24 * time.Hour

// Service is the main HTTP service for PeerDB.
type Service struct {
	waf.Service[*internalSite.Site]

	// Is service running in development mode.
	Development bool

	// scoreFactorMu guards the scoreFactorCache map structure (which entries
	// exist), not the entries themselves. Each entry carries its own mutex.
	scoreFactorMu sync.Mutex

	// scoreFactorCache memoizes, per ElasticSearch index, the counts.score ranking
	// boost factor. Entries are recomputed lazily once older than scoreFactorTTL.
	scoreFactorCache map[string]*scoreFactorEntry
}

// lookupSiteAuthenticator resolves the per-request Authenticator and role
// allowlist for the auth middleware.
//
//nolint:ireturn
func (s *Service) lookupSiteAuthenticator(w http.ResponseWriter, req *http.Request) (auth.Authenticator, map[string][]string, bool) {
	site, ok := waf.GetSite[*internalSite.Site](req.Context())
	if !ok {
		s.InternalServerErrorWithError(w, req, errors.New("no site in request context"))
		return nil, nil, true
	}
	if site.Authenticator == nil {
		errE := errors.New("site has no authenticator configured")
		errors.Details(errE)["domain"] = site.Domain
		s.InternalServerErrorWithError(w, req, errE)
		return nil, nil, true
	}
	return site.Authenticator, site.Roles, false
}

// HasPermission reports whether the caller currently holds the given
// permission on the site this request targets. A permission is granted
// when any role bound to the request (auth.Roles) maps via Site.Roles
// to a permission list that contains it. Returns nil on success and a
// "permission denied" error otherwise (including when no site is in
// ctx). In sync with src/auth/index.ts.
func (s *Service) HasPermission(ctx context.Context, permission string) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return errors.New("permission denied")
	}
	for _, role := range auth.Roles(ctx) {
		if slices.Contains(site.Roles[role], permission) {
			return nil
		}
	}
	return errors.New("permission denied")
}

// siteRoleNames returns the sorted names of the roles the site declares.
// It is the set a mock sign-in grants, resolved lazily at sign-in time so
// roles configured on the site after Init are still picked up.
func siteRoleNames(site *internalSite.Site) []string {
	roles := make([]string, 0, len(site.Roles))
	for role := range site.Roles {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	return roles
}

// Init initializes the HTTP service and is used together with Prepare to implement Run.
func (c *ServeCommand) Init(ctx context.Context, globals *Globals, files fs.FS) (*Service, func(), errors.E) {
	c.Server.Logger = globals.Logger

	sites := map[string]*internalSite.Site{}
	for i := range globals.Sites {
		site := &globals.Sites[i]

		sites[site.Domain] = site
	}

	if len(sites) == 0 && c.Domain != "" {
		// If sites are not provided, but default domain is,
		// we create a site based on the default domain.
		globals.Sites = []internalSite.Site{{
			Site: waf.Site{
				Domain:   c.Domain,
				CertFile: "",
				KeyFile:  "",
			},
			Build:                nil,
			Index:                globals.Elastic.Index,
			Schema:               globals.Postgres.Schema,
			Title:                c.Title,
			Logo:                 "",
			LogoCompact:          "",
			LanguagePriority:     nil,
			DefaultLanguage:      "",
			LanguageCodes:        nil,
			Features:             internalSite.SiteFeatures{},
			Roles:                nil,
			Auth:                 internalSite.SiteAuthConfig{},
			MetadataHeaderPrefix: "",
			Authenticator:        nil,
			Base:                 nil,
			DBPool:               nil,
			ESClient:             nil,
			RiverClient:          nil,
			DebugRiverHandler:    nil,
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
			site.Build = &internalSite.Build{
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

	service := &Service{ //nolint:forcetypeassert
		Service: waf.Service[*internalSite.Site]{
			Logger:          globals.Logger,
			CanonicalLogger: globals.Logger,
			WithContext:     globals.WithContext,
			StaticFiles:     files.(fs.ReadFileFS), //nolint:errcheck
			Routes:          nil,
			Sites:           sites,
			Middleware:      nil,
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
		Development:      c.Server.Development,
		scoreFactorMu:    sync.Mutex{},
		scoreFactorCache: map[string]*scoreFactorEntry{},
	}

	// We expose the canonical metadata-header prefix on each site so the
	// frontend can compose the right header names without having to guess.
	// It is global to the service but the site context is the only
	// per-site JSON the frontend receives.
	for _, site := range sites {
		site.MetadataHeaderPrefix = service.MetadataHeaderPrefix
	}

	service.Middleware = []func(http.Handler) http.Handler{}
	if c.Username != "" && c.Password != nil {
		globals.Logger.Info().Str("username", c.Username).Msg("authentication enabled for all sites")

		// Basic auth is a strict outer gate that applies to every request,
		// regardless of whether the caller also presents OIDC credentials
		// via cookie.
		service.Middleware = append(service.Middleware, auth.BasicAuthMiddleware(
			c.Username,
			strings.TrimSpace(string(c.Password)),
			func(req *http.Request) string {
				return waf.MustGetSite[*internalSite.Site](req.Context()).Title
			},
		))
	}

	// We attach the auth middleware last so the access token is verified
	// after any preceding gate (e.g. basicAuth) has already let the request
	// through. The lookup hands the middleware the per-site Authenticator
	// and role allowlist on every request.
	service.Middleware = append(service.Middleware, auth.Middleware(
		service.MetadataHeaderPrefix,
		service.lookupSiteAuthenticator,
	))

	// Each site needs an Authenticator. If the site declares an OIDC
	// issuer we validate tokens against it. Otherwise we fall back to
	// MockAuthenticator which short-circuits the flow in-process. Mock
	// is intended for development. Production sites are expected to set
	// Auth.Issuer.
	for _, site := range sites {
		// We use a fallback DB context here because we are still in the init path and no request
		// is in flight yet, so search_path has to be driven from the site's schema.
		siteCtx := internalStore.WithFallbackDBContext(ctx, site.Schema, "init")

		redirectURI := sync.OnceValue(func() string {
			host, errE := c.Server.Host(site.Domain)
			if errE != nil {
				return ""
			}
			callbackPath, errE := service.Reverse("AuthCallback", nil, nil)
			if errE != nil {
				return ""
			}
			return "https://" + host + callbackPath
		})

		// Site.Validate makes sure that or all three settings are set or none.
		if site.Auth.Issuer != "" {
			site.Authenticator, errE = auth.NewOIDCAuthenticator(siteCtx, site.DBPool, site.Auth.Issuer, site.Auth.ClientID, site.Auth.ClientSecret, redirectURI)
			if errE != nil {
				return nil, onShutdown, errE
			}
			globals.Logger.Info().Str("domain", site.Domain).Str("issuer", site.Auth.Issuer).Str("clientId", site.Auth.ClientID).Msg("OIDC authentication enabled")
		} else {
			site.Authenticator, errE = auth.NewMockAuthenticator(siteCtx, site.DBPool, site.Domain, func() []string { return siteRoleNames(site) }, redirectURI)
			if errE != nil {
				return nil, onShutdown, errE
			}
			globals.Logger.Info().Str("domain", site.Domain).Msg("mock authentication enabled")
		}

		// Register the per-site auth cleanup worker. The actual periodic
		// scheduling happens in Run, after the river client has been
		// started by Prepare. Here we only make sure the worker type is
		// known to the client when it starts.
		site.Base.RegisterWorkers = append(site.Base.RegisterWorkers, func(_ context.Context, workers *river.Workers) errors.E {
			return errors.WithStack(river.AddWorkerSafely(workers, &authCleanupWorker{
				Site:           site,
				WorkerDefaults: river.WorkerDefaults[authCleanupJobArgs]{},
			}))
		})
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
		siteCtx := internalStore.WithFallbackDBContext(ctx, site.Schema, "prepare")

		documents, errE := site.FetchDocuments(siteCtx, internalCore.PropertyClassID)
		if errE != nil {
			return nil, onShutdownF, errE
		}
		languages, errE := site.FetchDocuments(siteCtx, internalCore.LanguageClassID)
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
	// It is safe to call cancel multiple times. We want it to be called before
	// any onShutdown waits so that anything blocked on ctx unblocks first.
	defer cancel()
	if errE != nil {
		return errE
	}

	handler, prepareShutdown, errE := c.Prepare(ctx, service)
	if prepareShutdown != nil {
		defer prepareShutdown()
	}
	// It is safe to call cancel multiple times. We want it to be called before
	// any onShutdown waits so that anything blocked on ctx unblocks first.
	defer cancel()
	if errE != nil {
		return errE
	}

	// Register the daily auth-cleanup periodic job on each site's river
	// client. Prepare has already started the client, so RunOnStart
	// fires the first cleanup shortly after startup.
	for _, site := range service.Sites {
		_, err := site.RiverClient.PeriodicJobs().AddSafely(river.NewPeriodicJob(
			river.PeriodicInterval(authCleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return authCleanupJobArgs{}, nil
			},
			&river.PeriodicJobOpts{ //nolint:exhaustruct
				RunOnStart: true,
			},
		))
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// It returns only on error or if the server is gracefully shut down using ctrl-c.
	return c.Server.Run(ctx, handler)
}
