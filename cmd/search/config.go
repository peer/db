package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/go/zerolog"
)

type site struct {
	Domain    string `json:"domain"              yaml:"domain"`
	Index     string `json:"index"               yaml:"index"`
	Title     string `json:"title"               yaml:"title"`
	CertFile  string `json:"cert,omitempty"      yaml:"cert,omitempty"`
	KeyFile   string `json:"key,omitempty"       yaml:"key,omitempty"`
	SizeField bool   `json:"sizeField,omitempty" yaml:"sizeField,omitempty"`
}

func (s *site) Decode(ctx *kong.DecodeContext) error {
	var value string
	err := ctx.Scan.PopValueInto("value", &value)
	if err != nil {
		return err
	}
	return x.UnmarshalWithoutUnknownFields([]byte(value), s)
}

const (
	DefaultElastic  = "http://127.0.0.1:9200"
	DefaultIndex    = "docs"
	DefaultProxyTo  = "http://localhost:5173"
	DefaultTLSCache = "letsencrypt"
	DefaultTitle    = "PeerDB Search"
)

// Globals describes top-level (global) flags.
//
//nolint:lll
type Globals struct {
	zerolog.LoggingConfig `yaml:",inline"`

	Version   kong.VersionFlag `                                           help:"Show program's version and exit."                                                                                                                                                           short:"V" yaml:"-"`
	Config    cli.ConfigFlag   `                                           help:"Load configuration from a JSON or YAML file."                                                                                                   name:"config" placeholder:"PATH"            short:"c" yaml:"-"`
	Elastic   string           `default:"${defaultElastic}"                help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."                                                                                               placeholder:"URL"             short:"e" yaml:"elastic"`
	Index     string           `default:"${defaultIndex}"   group:"Sites:" help:"Name of ElasticSearch index to use when sites are not configured. Default: ${defaultIndex}."                                                                  placeholder:"NAME"            short:"i" yaml:"index"`
	SizeField bool             `                            group:"Sites:" help:"Enable size field on documents when sites are not configured. Requires mapper-size ElasticSearch plugin installed."                                                                                   yaml:"sizeField"`
	Sites     []site           `                            group:"Sites:" help:"Site configuration as JSON with fields \"domain\", \"index\", \"title\", \"cert\", \"key\", and \"sizeField\". Can be provided multiple times." name:"site"   placeholder:"SITE" sep:"none" short:"s" yaml:"sites"`
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
	Development bool   `                            help:"Run in development mode and proxy unknown requests."                                     short:"d" yaml:"development"`
	ProxyTo     string `default:"${defaultProxyTo}" help:"Base URL to proxy to in development mode. Default: ${defaultProxyTo}." placeholder:"URL" short:"P" yaml:"proxyTo"`
	TLS         struct {
		CertFile string `                             group:"File certificate:" help:"Default  certificate for TLS, when not using Let's Encrypt."                           name:"cert" placeholder:"PATH"   short:"k" type:"existingfile" yaml:"cert"`
		KeyFile  string `                             group:"File certificate:" help:"Default certificate's private key, when not using Let's Encrypt."                      name:"key"  placeholder:"PATH"   short:"K" type:"existingfile" yaml:"key"`
		Domain   string `                             group:"Let's Encrypt:"    help:"Domain name to request for Let's Encrypt's certificate when sites are not configured."             placeholder:"STRING" short:"D"                     yaml:"domain"`
		Email    string `                             group:"Let's Encrypt:"    help:"Contact e-mail to use with Let's Encrypt."                                                                              short:"E"                     yaml:"email"`
		Cache    string `default:"${defaultTLSCache}" group:"Let's Encrypt:"    help:"Let's Encrypt's cache directory. Default: ${defaultTLSCache}."                                     placeholder:"PATH"   short:"C" type:"path"         yaml:"cache"`
	} `embed:"" prefix:"tls." yaml:"tls"`
	Title string `default:"${defaultTitle}" group:"Sites:" help:"Title to be shown to the users when sites are not configured. Default: ${defaultTitle}." placeholder:"NAME" short:"T" yaml:"title"`
}

type PopulateCommand struct{}

func (c Config) Validate() error {
	if c.Serve.TLS.CertFile != "" || c.Serve.TLS.KeyFile != "" {
		if c.Serve.TLS.CertFile == "" {
			return errors.New("missing file certificate for provided private key")
		}
		if c.Serve.TLS.KeyFile == "" {
			return errors.New("missing file certificate's matching private key")
		}
	}

	if c.Serve.TLS.Domain != "" && c.Serve.TLS.Email == "" {
		return errors.New("contact e-mail is required for Let's Encrypt's certificate")
	}
	if c.Serve.TLS.Email != "" && c.Serve.TLS.Cache == "" {
		return errors.New("cache directory is required for Let's Encrypt's certificate")
	}

	domains := map[string]bool{}
	for i, site := range c.Sites {
		if site.Domain == "" {
			return errors.Errorf(`domain is required for site at index %d`, i)
		}
		if site.Index == "" {
			site.Index = DefaultIndex
		}
		if site.Title == "" {
			site.Title = DefaultTitle
		}

		if domains[site.Domain] {
			return errors.Errorf(`duplicate site for domain "%s"`, site.Domain)
		}
		domains[site.Domain] = true

		if site.CertFile != "" || site.KeyFile != "" {
			if site.CertFile == "" {
				return errors.Errorf(`missing file certificate for provided private key for site "%s"`, site.Domain)
			}
			if site.KeyFile == "" {
				return errors.Errorf(`missing file certificate's matching private key for site "%s"`, site.Domain)
			}
		}
	}

	return nil
}
