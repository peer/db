package peerdb

import (
	"github.com/alecthomas/kong"
	mapset "github.com/deckarep/golang-set/v2"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/zerolog"
	"gitlab.com/tozd/waf"
)

const (
	DefaultProxyTo  = "http://localhost:5173"
	DefaultTLSCache = "letsencrypt"
	DefaultElastic  = "http://127.0.0.1:9200"
	DefaultIndex    = "docs"
	DefaultSchema   = "docs"
	DefaultTitle    = "PeerDB"
)

//nolint:lll
type PostgresConfig struct {
	URL    kong.FileContentFlag `                           env:"URL_PATH" help:"File with PostgreSQL database URL. Environment variable: ${env}."                     placeholder:"PATH" required:"" short:"d" yaml:"database"`
	Schema string               `default:"${defaultSchema}"                help:"Name of PostgreSQL schema to use when sites are not configured. Default: ${default}." placeholder:"NAME"                       yaml:"schema"`
}

//nolint:lll
type ElasticConfig struct {
	URL   string `default:"${defaultElastic}" help:"URL of the ElasticSearch instance. Default: ${default}."                                                            placeholder:"URL"  short:"e" yaml:"elastic"`
	Index string `default:"${defaultIndex}"   help:"Name of ElasticSearch index to use when sites are not configured. Default: ${default}."                             placeholder:"NAME"           yaml:"index"`
}

// Globals describes top-level (global) flags.
//
//nolint:lll
type Globals struct {
	zerolog.LoggingConfig `yaml:",inline"`

	Version kong.VersionFlag `help:"Show program's version and exit."                                              short:"V" yaml:"-"`
	Config  cli.ConfigFlag   `help:"Load configuration from a JSON or YAML file." name:"config" placeholder:"PATH" short:"c" yaml:"-"`

	Postgres PostgresConfig `embed:"" envprefix:"POSTGRES_" prefix:"postgres." yaml:"postgres"`
	Elastic  ElasticConfig  `embed:"" envprefix:"ELASTIC_"  prefix:"elastic."  yaml:"elastic"`

	Sites []Site `help:"Site configuration as JSON or YAML with fields \"domain\", \"index\", \"schema\", \"title\", \"cert\", and \"key\". Can be provided multiple times." name:"site" placeholder:"SITE" sep:"none" short:"s" yaml:"sites"`
}

func (g *Globals) Validate() error {
	domains := mapset.NewThreadUnsafeSet[string]()
	for i, site := range g.Sites {
		// This is not validated when Site is not populated by Kong.
		if site.Domain == "" {
			return errors.Errorf(`domain is required for site at index %d`, i)
		}

		// To make sure validation is called.
		err := site.Validate()
		if err != nil {
			return errors.WithStack(err)
		}

		// We cannot use kong to set these defaults, so we do it here.
		if site.Index == "" {
			site.Index = DefaultIndex
		}
		if site.Title == "" {
			site.Title = DefaultTitle
		}

		if !domains.Add(site.Domain) {
			return errors.Errorf(`duplicate site for domain "%s"`, site.Domain)
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

	Serve    ServeCommand    `cmd:"" default:"withargs" help:"Run PeerDB server. Default command."                    yaml:"serve"`
	Populate PopulateCommand `cmd:""                    help:"Populate search index or indices with core properties." yaml:"populate"`
}

//nolint:lll
type ServeCommand struct {
	Server waf.Server[*Site] `embed:"" yaml:",inline"`

	Username string               `                    help:"Require authentication to access all sites. Its username."                    yaml:"username"`
	Password kong.FileContentFlag `env:"PASSWORD_PATH" help:"Require authentication to access all sites. Its password." placeholder:"PATH" yaml:"password"`

	Domain string `                          group:"Let's Encrypt:" help:"Domain name to request for Let's Encrypt's certificate when sites are not configured." name:"tls.domain" placeholder:"STRING"           yaml:"domain"`
	Title  string `default:"${defaultTitle}"                        help:"Title to be shown to the users when sites are not configured. Default: ${default}."                      placeholder:"NAME"   short:"T" yaml:"title"`
}

func (c *ServeCommand) Validate() error {
	// We have to call Validate on kong-embedded structs ourselves.
	// See: https://github.com/alecthomas/kong/issues/90
	err := c.Server.TLS.Validate()
	if err != nil {
		return errors.WithStack(err)
	}

	if c.Domain != "" && c.Server.TLS.Email == "" {
		return errors.New("contact e-mail is required for Let's Encrypt's certificate")
	}

	if (c.Username != "" && c.Password == nil) || (c.Username == "" && c.Password != nil) {
		return errors.New("both username and password have to be set to require authentication, or neither")
	}

	return nil
}

type PopulateCommand struct{}
