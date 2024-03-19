package search

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
	DefaultTitle    = "PeerDB Search"
)

// Globals describes top-level (global) flags.
//
//nolint:lll
type Globals struct {
	zerolog.LoggingConfig `yaml:",inline"`

	Version kong.VersionFlag `help:"Show program's version and exit."                                              short:"V" yaml:"-"`
	Config  cli.ConfigFlag   `help:"Load configuration from a JSON or YAML file." name:"config" placeholder:"PATH" short:"c" yaml:"-"`

	Elastic   string `default:"${defaultElastic}"                help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."                                                                                                     placeholder:"URL"             short:"e" yaml:"elastic"`
	Index     string `default:"${defaultIndex}"   group:"Sites:" help:"Name of ElasticSearch index to use when sites are not configured. Default: ${defaultIndex}."                                                                        placeholder:"NAME"            short:"i" yaml:"index"`
	SizeField bool   `                            group:"Sites:" help:"Enable size field on documents when sites are not configured. Requires mapper-size ElasticSearch plugin installed."                                                                                         yaml:"sizeField"`
	Sites     []Site `                            group:"Sites:" help:"Site configuration as JSON or YAML with fields \"domain\", \"index\", \"title\", \"cert\", \"key\", and \"sizeField\". Can be provided multiple times." name:"site" placeholder:"SITE" sep:"none" short:"s" yaml:"sites"`
}

func (g *Globals) Validate() error {
	domains := mapset.NewThreadUnsafeSet[string]()
	for i, site := range g.Sites {
		// This is not validated when Site is not populated by Kong.
		if site.Domain == "" {
			return errors.Errorf(`domain is required for site at index %d`, i)
		}

		// To make sure validation is called.
		if err := site.Validate(); err != nil {
			return err
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
	}

	return nil
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals `yaml:"globals"`

	Serve    ServeCommand    `cmd:"" default:"withargs" help:"Run PeerDB Search server. Default command."      yaml:"serve"`
	Populate PopulateCommand `cmd:""                    help:"Populate index or indices with core properties." yaml:"populate"`
}

//nolint:lll
type ServeCommand struct {
	Server waf.Server[*Site] `embed:"" yaml:",inline"`

	Domain string `                          group:"Let's Encrypt:" help:"Domain name to request for Let's Encrypt's certificate when sites are not configured."   name:"tls.domain" placeholder:"STRING" short:"D" yaml:"domain"`
	Title  string `default:"${defaultTitle}" group:"Sites:"         help:"Title to be shown to the users when sites are not configured. Default: ${defaultTitle}."                   placeholder:"NAME"   short:"T" yaml:"title"`
}

func (c *ServeCommand) Validate() error {
	// We have to call Validate on kong-embedded structs ourselves.
	// See: https://github.com/alecthomas/kong/issues/90
	if err := c.Server.TLS.Validate(); err != nil {
		return err
	}

	if c.Domain != "" && c.Server.TLS.Email == "" {
		return errors.New("contact e-mail is required for Let's Encrypt's certificate")
	}

	return nil
}

type PopulateCommand struct{}
