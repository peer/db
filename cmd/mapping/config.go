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
	Version kong.VersionFlag `help:"Show program's version and exit." short:"V"`
	cli.LoggingConfig
	Output string `default:"${defaultOutput}" help:"Where to output generated mapping. Default: ${defaultOutput}." placeholder:"PATH" short:"o" type:"path"`
}
