package main

import (
	"github.com/alecthomas/kong"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Version   kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	OutputDir string           `name:"output" placeholder:"DIR" default:"output" type:"path" help:"Where to output files to."`
}
