package main

import (
	"github.com/alecthomas/kong"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/cli"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	cli.LoggingConfig
	TLS struct {
		Static struct {
			CertFile string `short:"k" name:"cert" placeholder:"PATH" type:"existingfile" help:"A static certificate for TLS, when not using Let's Encrypt."`
			KeyFile  string `short:"K" name:"key" placeholder:"PATH" type:"existingfile" help:"A static certificate's matching private key, when not using Let's Encrypt."`
		} `embed:"" prefix:"static."`
		LetsEncrypt struct {
			Domain []string `short:"D" sep:"none" placeholder:"STRING" help:"Domain name to request for Let's Encrypt's certificate. Can be provided multiple times."`
			Email  string   `short:"E" help:"Contact e-mail to use with Let's Encrypt."`
			Cache  string   `short:"C" type:"path" placeholder:"PATH" default:"letsencrypt" help:"Let's Encrypt's cache directory. Default: ${default}."`
		} `embed:"" prefix:"letsencrypt."`
	} `embed:"" prefix:"tls."`
	Elastic     string `short:"e" placeholder:"URL" default:"http://127.0.0.1:9200" help:"URL of the ElasticSearch instance. Default: ${default}"`
	Index       string `short:"i" placeholder:"NAME" default:"docs" help:"Name of ElasticSearch index to use. Default: ${default}."`
	Development bool   `short:"d" help:"Run in development mode and proxy unknown requests."`
	ProxyTo     string `short:"P" placeholder:"URL" default:"http://localhost:3000" help:"Base URL to proxy to in development mode. Default: ${default}"`
}

func (c Config) Validate() error {
	if c.TLS.Static.CertFile != "" || c.TLS.Static.KeyFile != "" {
		if len(c.TLS.LetsEncrypt.Domain) > 0 || c.TLS.LetsEncrypt.Email != "" {
			return errors.New("static certificate cannot be used together with Let's Encrypt")
		}
		if c.TLS.Static.CertFile == "" {
			return errors.New("missing static certificate for provided private key")
		}
		if c.TLS.Static.KeyFile == "" {
			return errors.New("missing static certificate's matching private key")
		}
		return nil
	}

	if len(c.TLS.LetsEncrypt.Domain) > 0 || c.TLS.LetsEncrypt.Email != "" {
		if len(c.TLS.LetsEncrypt.Domain) == 0 {
			return errors.New("at least one domain is required for Let's Encrypt's certificate")
		}
		if c.TLS.LetsEncrypt.Email == "" {
			return errors.New("contact e-mail is required for Let's Encrypt's certificate")
		}
		return nil
	}

	return errors.New("static certificate or Let's Encrypt's certificate is required")
}
