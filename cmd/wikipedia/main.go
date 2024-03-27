package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb"
)

func main() {
	var config Config
	cli.Run(&config, kong.Vars{
		"defaultAPILimit": DefaultAPILimit,
		"defaultCacheDir": DefaultCacheDir,
		"defaultElastic":  peerdb.DefaultElastic,
		"defaultIndex":    peerdb.DefaultIndex,
	}, func(ctx *kong.Context) errors.E {
		return errors.WithStack(ctx.Run(&config.Globals))
	})
}
