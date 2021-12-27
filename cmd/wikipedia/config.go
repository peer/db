package main

import (
	"github.com/alecthomas/kong"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Version  kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	CacheDir string           `name:"cache" placeholder:"DIR" default:".cache" type:"path" help:"Where to cache files to."`
}
