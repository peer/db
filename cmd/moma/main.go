package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/cli"
)

func main() {
	var config Config
	cli.Run(&config, kong.Vars{
		"defaultCacheDir":    DefaultCacheDir,
		"defaultElastic":     DefaultElastic,
		"defaultIndex":       DefaultIndex,
		"defaultArtistsURL":  DefaultArtistsURL,
		"defaultArtworksURL": DefaultArtworksURL,
	}, func(ctx *kong.Context) errors.E {
		return index(&config)
	})
}
