// Package peerdb provides the command-line interface for PeerDB.
package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb"
)

func main() {
	var config peerdb.Config
	cli.Run(&config, kong.Vars{
		"defaultProxyTo":      peerdb.DefaultProxyTo,
		"defaultTLSCache":     peerdb.DefaultTLSCache,
		"defaultElastic":      peerdb.DefaultElastic,
		"defaultSchema":       peerdb.DefaultSchema,
		"defaultIndex":        peerdb.DefaultIndex,
		"defaultTitle":        peerdb.DefaultTitle,
		"developmentModeHelp": " Proxy unknown requests.",
	}, func(ctx *kong.Context) errors.E {
		return errors.WithStack(ctx.Run(&config.Globals))
	})
}
