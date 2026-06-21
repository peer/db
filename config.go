package peerdb

import (
	"context"

	"github.com/alecthomas/kong"
	mapset "github.com/deckarep/golang-set/v2"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/zerolog"
	"gitlab.com/tozd/waf"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

const (
	// DefaultListen is the default TCP listen address.
	DefaultListen = ":8080"
	// DefaultProxyTo is the default URL to proxy to during development.
	DefaultProxyTo = "http://localhost:5173"
	// DefaultElastic is the default Elasticsearch URL.
	DefaultElastic = "http://127.0.0.1:9200"
	// DefaultSchema is the default database schema name.
	DefaultSchema = "peerdb"
	// DefaultIndexPrefix is the default Elasticsearch index prefix. The visibility level name is appended to it to form each per-level index name.
	DefaultIndexPrefix = "peerdb"
	// DefaultShards is the default number of Elasticsearch shards.
	DefaultShards = "10"
	// DefaultTitle is the default application title.
	DefaultTitle = "PeerDB"
)

// PostgresConfig contains configuration for PostgreSQL database connection.
//
//nolint:lll
type PostgresConfig struct {
	URL    kong.FileContentFlag `                           env:"URL_PATH" help:"File with PostgreSQL database URL."                              placeholder:"PATH" required:"" short:"d" yaml:"database"`
	Schema string               `default:"${defaultSchema}"                help:"Name of PostgreSQL schema to use when sites are not configured." placeholder:"NAME"                       yaml:"schema"`
}

// ElasticConfig contains configuration for ElasticSearch connection.
//
//nolint:lll
type ElasticConfig struct {
	URL         string `default:"${defaultElastic}"     help:"URL of the ElasticSearch instance."                                        placeholder:"URL"    short:"e" yaml:"elastic"`
	IndexPrefix string `default:"${defaultIndexPrefix}" help:"Prefix of ElasticSearch index names to use when sites are not configured." placeholder:"PREFIX"           yaml:"indexPrefix"`
	Shards      int    `default:"${defaultShards}"      help:"Number of ElasticSearch shards when initializing indices."                 placeholder:"NUM"              yaml:"shards"`
}

// StorageConfig contains configuration for file storage.
type StorageConfig struct {
	Dir string `help:"Directory under which to store files." placeholder:"PATH" required:"" type:"path" yaml:"dir"`
}

// Customizer allows a consumer using PeerDB as a library to attach code at well-defined points of the
// lifecycle, uniformly across all commands, so that commands cannot diverge in how sites are configured.
// All hooks are optional. Set it on Globals in code before command-line parsing.
type Customizer struct {
	// SiteDefaults is called for every site after the configuration file and command-line flags have been
	// applied and before the site is validated. It can fill defaults into fields the configuration left
	// unset (the configuration can then override such fields) or overwrite fields unconditionally (such
	// fields are fixed by code and the configuration cannot change them). It is also called for sites
	// synthesized when none are configured (the default site and the domain/certificate-based sites),
	// and can run more than once for the same site, so it must be idempotent and accept values it
	// has set itself.
	SiteDefaults func(site *Site) errors.E

	// ConfigureBase is called for every site right after Init has populated site.Base and before the base
	// is started, in every command which initializes the base. It is the place to register document, file,
	// and indexing hooks on the base. It is called exactly once per site.
	ConfigureBase func(site *Site) errors.E
}

// Globals describes top-level (global) flags.
type Globals struct {
	zerolog.LoggingConfig `yaml:",inline"`

	Version kong.VersionFlag `help:"Show program's version and exit."                                              short:"V" yaml:"-"`
	Config  cli.ConfigFlag   `help:"Load configuration from a JSON or YAML file." name:"config" placeholder:"PATH" short:"c" yaml:"-"`

	Postgres PostgresConfig `embed:"" envprefix:"POSTGRES_" prefix:"postgres." yaml:"postgres"`
	Elastic  ElasticConfig  `embed:"" envprefix:"ELASTIC_"  prefix:"elastic."  yaml:"elastic"`
	Storage  StorageConfig  `embed:"" envprefix:"STORAGE_"  prefix:"storage."  yaml:"storage"`

	Sites []internalSite.Site `help:"Site configuration as JSON or YAML. Can be provided multiple times." name:"site" placeholder:"SITE" sep:"none" short:"s" yaml:"sites"`

	// Customize is set in code by a consumer using PeerDB as a library. It is not part of the configuration.
	Customize Customizer `json:"-" kong:"-" yaml:"-"`
}

// Validate validates the global configuration.
func (g *Globals) Validate() error {
	domains := mapset.NewThreadUnsafeSet[string]()
	for i, site := range g.Sites {
		// This is not validated when Site is not populated by Kong.
		if site.Domain == "" {
			errE := errors.New("domain is required for site")
			errors.Details(errE)["site"] = i
			return errE
		}

		// Consumer defaults run before validation, so that they see the raw configured state (e.g. an
		// empty Visibility is still empty, not yet defaulted by validation) and so that the values they
		// set are validated.
		if g.Customize.SiteDefaults != nil {
			errE := g.Customize.SiteDefaults(&site)
			if errE != nil {
				return errE
			}
		}

		// To make sure validation is called.
		err := site.Validate()
		if err != nil {
			return errors.WithStack(err)
		}

		// We cannot use kong to set these defaults, so we do it here.
		if site.IndexPrefix == "" {
			site.IndexPrefix = DefaultIndexPrefix
		}
		if site.Title == "" {
			site.Title = DefaultTitle
		}

		if !domains.Add(site.Domain) {
			errE := errors.New("duplicate site for domain")
			errors.Details(errE)["domain"] = site.Domain
			return errE
		}

		// Site might have been changed, so we assign it back.
		g.Sites[i] = site
	}

	return nil
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals `yaml:"globals"`

	Serve    ServeCommand    `cmd:"" default:"withargs" help:"Run HTTP server. Default command." yaml:"serve"`
	Populate PopulateCommand `cmd:""                    help:"Populate database with documents." yaml:"populate"`
	DB       DBCommand       `cmd:""                    help:"Manage database."                  yaml:"db"`
}

// ServeCommand contains configuration for the serve command.
//
//nolint:lll
type ServeCommand struct {
	Server waf.Server[*internalSite.Site] `embed:"" yaml:",inline"`

	Username string               `                    help:"Require authentication to access all sites. Its username."                    yaml:"username"`
	Password kong.FileContentFlag `env:"PASSWORD_PATH" help:"Require authentication to access all sites. Its password." placeholder:"PATH" yaml:"password"`

	Domain string `                          group:"Let's Encrypt:" help:"Domain name to request for Let's Encrypt's certificate when sites are not configured." name:"tls.domain" placeholder:"STRING"           yaml:"domain"`
	Title  string `default:"${defaultTitle}"                        help:"Title to be shown to the users when sites are not configured."                                           placeholder:"STRING" short:"T" yaml:"title"`

	// AfterInit is set in code by a consumer using PeerDB as a library. It is called at the end of Init,
	// after the service has been created and the sites' bases initialized, and before Prepare starts
	// them, so it can wrap the service, add routes, and register service-scoped river workers (the river
	// clients are not yet started). It is not part of the configuration.
	AfterInit func(ctx context.Context, service *Service) errors.E `json:"-" kong:"-" yaml:"-"`

	// AfterPrepare is set in code by a consumer using PeerDB as a library. It is called at the end of
	// Prepare, after the sites' bases have been started (the river clients are running) and before the
	// request handler is constructed, so it can e.g. register periodic river jobs. It is not part of the
	// configuration.
	AfterPrepare func(ctx context.Context, service *Service) errors.E `json:"-" kong:"-" yaml:"-"`
}

// Validate validates the serve command configuration.
func (c *ServeCommand) Validate() error {
	// We have to call Validate on kong-embedded structs ourselves.
	// See: https://github.com/alecthomas/kong/issues/90
	err := c.Server.HTTPS.Validate()
	if err != nil {
		return errors.WithStack(err)
	}

	if c.Domain != "" && c.Server.HTTPS.LetsEncryptCache == "" {
		return errors.New("Let's Encrypt's cache directory is required for Let's Encrypt's certificate")
	}

	if (c.Username != "" && c.Password == nil) || (c.Username == "" && c.Password != nil) {
		return errors.New("both username and password have to be set to require authentication, or neither")
	}

	return nil
}

// PopulateCommand contains configuration for the populate command.
type PopulateCommand struct {
	SaveDir   string `help:"Save intermediate structs as files into a directory."            name:"save"   placeholder:"DIR" short:"S" type:"path" yaml:"saveDir"`
	OutputDir string `help:"Save documents as files into a directory."                       name:"output" placeholder:"DIR" short:"O" type:"path" yaml:"outputDir"`
	DryRun    bool   `help:"Dry run. Do everything, but insert documents into the database."                                                       yaml:"dryRun"`

	// PopulateSite is set in code by a consumer using PeerDB as a library to replace how a site is
	// populated. When nil, core documents are generated and inserted. It is called with a per-site
	// cancellable context whose logger carries the site's indexPrefix and schema fields and whose
	// fallback database context is set. It returns the shutdown function from PopulateAndStart (or nil
	// when the base was not started). Run cancels the context and then runs the shutdown. It is not
	// part of the configuration.
	PopulateSite func(ctx context.Context, site Site) (func(), errors.E) `json:"-" kong:"-" yaml:"-"`
}

// DBWaitCommand waits for pending indexing to complete.
type DBWaitCommand struct{}

// DBReindexCommand forces a full reindex of all documents.
//
//nolint:lll
type DBReindexCommand struct {
	RecreateIndex bool `help:"Delete and recreate the ElasticSearch indices and clear inverse-relation metadata in the store before reindexing, so the current mapping and inverse relations are rebuilt from scratch." name:"recreate-index" yaml:"recreateIndex"`
}

// DBVacuumCommand reclaims dead tuples in PostgreSQL and expunges deleted documents from ElasticSearch for all sites.
type DBVacuumCommand struct{}

// DBWipeCommand drops PostgreSQL schemas and deletes ElasticSearch indices for all sites.
type DBWipeCommand struct{}

// DBExportCommand exports documents to CSV, JSON, or struct.
//
//nolint:lll
type DBExportCommand struct {
	Output     string   `default:"-"                          help:"Output file path. Use - for stdout."                                                                  placeholder:"PATH"   short:"o" type:"path" yaml:"output"`
	Format     string   `default:"csv" enum:"csv,json,struct" help:"Output format."                                                                                       placeholder:"FORMAT" short:"f"             yaml:"format"`
	InstanceOf []string `                                     help:"Limit to instances of class (mnemonic or ID)."                                     name:"instance-of" placeholder:"STRING" short:"i"             yaml:"instanceOf"`
	Property   []string `                                     help:"Properties to export (a.b.c path, * for single-level wildcard, ** for recursive)." name:"property"    placeholder:"STRING" short:"p"             yaml:"property"`
}

// DBDiagramCommand outputs a Mermaid ER diagram of classes and fields.
type DBDiagramCommand struct {
	Output   string `default:"-" help:"Output file path. Use - for stdout."                                        placeholder:"PATH" short:"o" type:"path" yaml:"output"`
	SkipCore bool   `            help:"Exclude core entities and INSTANCE_OF references to them." name:"skip-core"                                          yaml:"skipCore"`
}

// DBCommand contains sub-commands for managing database.
type DBCommand struct {
	Wait    DBWaitCommand    `cmd:"" help:"Wait for pending indexing to complete and exit."      yaml:"wait"`
	Reindex DBReindexCommand `cmd:"" help:"Force full reindex of all documents."                 yaml:"reindex"`
	Vacuum  DBVacuumCommand  `cmd:"" help:"Vacuum PostgreSQL and expunge ElasticSearch deletes." yaml:"vacuum"`
	Wipe    DBWipeCommand    `cmd:"" help:"Wipe PostgreSQL schemas and ElasticSearch indices."   yaml:"wipe"`
	Export  DBExportCommand  `cmd:"" help:"Export documents to CSV, JSON, or struct."            yaml:"export"`
	Diagram DBDiagramCommand `cmd:"" help:"Output Mermaid ER diagram of classes and fields."     yaml:"diagram"`
}
