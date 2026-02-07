// Command moma is the MoMA (Museum of Modern Art) data import tool.
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
		"defaultCacheDir":    DefaultCacheDir,
		"defaultElastic":     peerdb.DefaultElastic,
		"defaultIndex":       peerdb.DefaultIndex,
		"defaultSchema":      peerdb.DefaultSchema,
		"defaultArtistsURL":  DefaultArtistsURL,
		"defaultArtworksURL": DefaultArtworksURL,
	}, func(_ *kong.Context) errors.E {
		return index(&config)
	})
}
