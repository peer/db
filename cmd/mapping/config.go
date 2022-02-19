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
}
