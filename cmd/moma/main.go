package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
)

func main() {
	var config Config
	cli.Run(&config, kong.Vars{
		"defaultCacheDir":    DefaultCacheDir,
		"defaultElastic":     DefaultElastic,
		"defaultIndex":       DefaultIndex,
		"defaultArtistsURL":  DefaultArtistsURL,
		"defaultArtworksURL": DefaultArtworksURL,
	}, func(_ *kong.Context) errors.E {
		return index(&config)
	})
}
