package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/zerolog"
)

const (
	DefaultCacheDir    = ".cache"
	DefaultArtistsURL  = "https://github.com/MuseumofModernArt/collection/raw/master/Artists.json"
	DefaultArtworksURL = "https://github.com/MuseumofModernArt/collection/raw/master/Artworks.json"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
//
//nolint:lll
type Config struct {
	zerolog.LoggingConfig

	Version     kong.VersionFlag `                                help:"Show program's version and exit."                                                                                                  short:"V"`
	CacheDir    string           `default:"${defaultCacheDir}"    help:"Where to cache files to. Default: ${defaultCacheDir}."                                          name:"cache"    placeholder:"DIR"  short:"C" type:"path"`
	Elastic     string           `default:"${defaultElastic}"     help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."                                                 placeholder:"URL"  short:"e"`
	Index       string           `default:"${defaultIndex}"       help:"Name of ElasticSearch index to use. Default: ${defaultIndex}."                                                  placeholder:"NAME" short:"i"`
	SizeField   bool             `                                help:"Enable size field on documents.. Requires mapper-size ElasticSearch plugin installed."`
	ArtistsURL  string           `default:"${defaultArtistsURL}"  help:"URL of artists JSON to use. It can be a local file path, too. Default: ${defaultArtistsURL}."   name:"artists"  placeholder:"URL"`
	ArtworksURL string           `default:"${defaultArtworksURL}" help:"URL of artworks JSON to use. It can be a local file path, too. Default: ${defaultArtworksURL}." name:"artworks" placeholder:"URL"`
	WebsiteData bool             `                                help:"Fetch images and descriptions from MoMA website."`
}
