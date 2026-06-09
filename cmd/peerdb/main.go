// Command peerdb is the command-line interface for PeerDB.
package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/dist"
)

func main() {
	var config peerdb.Config
	cli.Run(&config, kong.Vars{
		"defaultListen":       peerdb.DefaultListen,
		"defaultProxyTo":      peerdb.DefaultProxyTo,
		"defaultElastic":      peerdb.DefaultElastic,
		"defaultSchema":       peerdb.DefaultSchema,
		"defaultIndexPrefix":  peerdb.DefaultIndexPrefix,
		"defaultShards":       peerdb.DefaultShards,
		"defaultTitle":        peerdb.DefaultTitle,
		"developmentModeHelp": peerdb.DevelopmentModeHelp,
	}, func(ctx *cli.Context) errors.E {
		return ctx.Run(&config.Globals)
	},
		// We have to use BindFor instead of passing it directly to Run because we are using an interface.
		// See: https://github.com/alecthomas/kong/issues/48
		kong.BindFor(dist.Files),
	)
}
