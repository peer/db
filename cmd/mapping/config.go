package main

import (
	"github.com/alecthomas/kong"

	"gitlab.com/peerdb/search/internal/cli"
)

const (
	DefaultOutput = "index.json"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	cli.LoggingConfig
	Output string `short:"o" placeholder:"PATH" type:"path" default:"${defaultOutput}" help:"Where to output generated mapping. Default: ${defaultOutput}."`
}
