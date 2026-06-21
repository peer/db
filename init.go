package peerdb

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// Init initializes PeerDB for all sites defined in globals.
//
// It establishes connections to PostgreSQL database and ElasticSearch.
// It configures PostgreSQL schemas and ElasticSearch indices.
//
// It can be called multiple times. In that case it will initialize only
// sites which have not been initialized yet.
//
// You have to run site.Start for each site after this call to start the
// base for each site.
func Init(ctx context.Context, globals *Globals) (func(), errors.E) {
	var dbpool *pgxpool.Pool
	var esClient *elasticsearch.TypedClient

	// First we check if any site have them initialized already.
	for _, site := range globals.Sites {
		if dbpool == nil && site.DBPool != nil {
			dbpool = site.DBPool
		}

		if esClient == nil && site.ESClient != nil {
			esClient = site.ESClient
		}

		if dbpool != nil && esClient != nil {
			break
		}
	}

	onShutdown := []func(){}
	onShutdownF := func() {
		for _, f := range onShutdown {
			if f == nil {
				continue
			}
			f()
		}
	}

	// Initialize for the first time.
	if dbpool == nil {
		var errE errors.E
		var dbpoolCleanup func()
		// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
		// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
		dbpool, dbpoolCleanup, errE = internalStore.InitPostgres(
			context.WithoutCancel(ctx),
			string(globals.Postgres.URL),
			globals.Logger,
			getRequestWithFallback(),
		)
		if errE != nil {
			return nil, errE
		}
		// We want dbpoolCleanup to be last.
		onShutdown = append(onShutdown, dbpoolCleanup)
	}

	// Initialize for the first time.
	if esClient == nil {
		var errE errors.E
		esClient, errE = internalSearch.GetClient(cleanhttp.DefaultPooledClient(), globals.Logger, globals.Elastic.URL)
		if errE != nil {
			return onShutdownF, errE
		}
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]

		// Init can be called multiple times and Prepare initializes a site only once. ConfigureBase must
		// run exactly once per site (hooks must not be registered twice), so it runs only when Prepare
		// populated the base now.
		firstInit := site.Base == nil

		errE := site.Prepare(ctx, globals.Logger, globals.WithContext, dbpool, esClient, globals.Elastic.Shards, globals.Storage.Dir)
		if errE != nil {
			return onShutdownF, errE
		}

		if firstInit && globals.Customize.ConfigureBase != nil {
			errE = globals.Customize.ConfigureBase(site)
			if errE != nil {
				return onShutdownF, errE
			}
		}
	}

	return onShutdownF, nil
}

// InitSites sets up default site configuration and build information if needed. It also applies consumer
// site defaults (through Customize.SiteDefaults) to every site: sites from the configuration already received
// them during configuration validation, but the default site synthesized here did not, and callers which populate
// Globals programmatically (without command-line parsing) get them here as well. SiteDefaults is idempotent,
// so the repeated application is safe.
func InitSites(globals *Globals) errors.E {
	if len(globals.Sites) == 0 {
		globals.Sites = []internalSite.Site{{
			Site: waf.Site{
				Domain:   "",
				CertFile: "",
				KeyFile:  "",
			},
			Build:                nil,
			IndexPrefix:          globals.Elastic.IndexPrefix,
			Schema:               globals.Postgres.Schema,
			Title:                "",
			Logo:                 "",
			LogoCompact:          "",
			LanguagePriority:     nil,
			DefaultLanguage:      "",
			LanguageCodes:        nil,
			Features:             internalSite.SiteFeatures{},
			Roles:                nil,
			Visibility:           nil,
			Auth:                 internalSite.SiteAuthConfig{},
			MetadataHeaderPrefix: "",
			Authenticator:        nil,
			Base:                 nil,
			DBPool:               nil,
			ESClient:             nil,
			RiverClient:          nil,
			DebugRiverHandler:    nil,
		}}
	}

	if globals.Customize.SiteDefaults != nil {
		for i := range globals.Sites {
			errE := globals.Customize.SiteDefaults(&globals.Sites[i])
			if errE != nil {
				return errE
			}
		}
	}

	// Sites from the configuration were validated during configuration validation already, but the
	// default site synthesized above was not, so we validate all sites here. Validation is idempotent.
	for i := range globals.Sites {
		err := globals.Sites[i].Validate()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// We set build information on sites.
	if cli.Version != "" || cli.BuildTimestamp != "" || cli.Revision != "" {
		for i := range globals.Sites {
			site := &globals.Sites[i]
			site.Build = &internalSite.Build{
				Version:        cli.Version,
				BuildTimestamp: cli.BuildTimestamp,
				Revision:       cli.Revision,
			}
		}
	}

	return nil
}
