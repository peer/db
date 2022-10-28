package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/cli"
)

func main() {
	var config Config
	cli.Run(&config, "", kong.Vars{
		"defaultElastic":  DefaultElastic,
		"defaultIndex":    DefaultIndex,
		"defaultProxyTo":  DefaultProxyTo,
		"defaultTLSCache": DefaultTLSCache,
		"defaultTitle":    DefaultTitle,
	}, func(ctx *kong.Context) errors.E {
		return errors.WithStack(ctx.Run(&config.Globals))
	})
}
