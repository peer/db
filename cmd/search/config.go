package main

import (
	"github.com/alecthomas/kong"

	"gitlab.com/peerdb/search/internal/cli"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	cli.LoggingConfig
	CertFile string `short:"c" placeholder:"PATH" required:"" type:"existingfile" help:"A certificate for TLS."`
	KeyFile  string `short:"k" placeholder:"PATH" required:"" type:"existingfile" help:"A certificate's matching private key."`
}
