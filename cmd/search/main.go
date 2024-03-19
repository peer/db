package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func main() {
	var config search.Config
	cli.Run(&config, kong.Vars{
		"defaultProxyTo":      search.DefaultProxyTo,
		"defaultTLSCache":     search.DefaultTLSCache,
		"defaultElastic":      search.DefaultElastic,
		"defaultIndex":        search.DefaultIndex,
		"defaultTitle":        search.DefaultTitle,
		"developmentModeHelp": " Proxy unknown requests.",
	}, func(ctx *kong.Context) errors.E {
		return errors.WithStack(ctx.Run(&config.Globals))
	})
}
