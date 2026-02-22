// Command peerdb is the command-line interface for PeerDB.
package main

import (
	"io/fs"

	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/dist"
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
		"developmentModeHelp": peerdb.DevelopmentModeHelp,
	}, func(ctx *kong.Context) errors.E {
		return errors.WithStack(ctx.Run(&config.Globals))
		// We have to use BindTo instead of passing it directly to Run because we are using an interface.
		// See: https://github.com/alecthomas/kong/issues/48
	}, kong.BindTo(dist.Files, (*fs.FS)(nil)))
}
