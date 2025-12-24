package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/zerolog"
)

const (
	DefaultOutput = "internal/es/index.json"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	zerolog.LoggingConfig

	Version kong.VersionFlag `                           help:"Show program's version and exit."                                           short:"V"`
	Output  string           `default:"${defaultOutput}" help:"Where to output generated mapping." placeholder:"PATH" short:"o" type:"path"`
}
