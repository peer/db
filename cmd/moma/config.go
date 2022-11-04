package main

import (
	"github.com/alecthomas/kong"

	"gitlab.com/peerdb/search/internal/cli"
)

const (
	DefaultCacheDir    = ".cache"
	DefaultElastic     = "http://127.0.0.1:9200"
	DefaultIndex       = "docs"
	DefaultArtistsURL  = "https://github.com/MuseumofModernArt/collection/raw/master/Artists.json"
	DefaultArtworksURL = "https://github.com/MuseumofModernArt/collection/raw/master/Artworks.json"
)

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
//
//nolint:lll
type Config struct {
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	cli.LoggingConfig
	CacheDir    string `short:"C" name:"cache" placeholder:"DIR" default:"${defaultCacheDir}" type:"path" help:"Where to cache files to. Default: ${defaultCacheDir}."`
	Elastic     string `short:"e" placeholder:"URL" default:"${defaultElastic}" help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."`
	Index       string `short:"i" placeholder:"NAME" default:"${defaultIndex}" help:"Name of ElasticSearch index to use. Default: ${defaultIndex}."`
	SizeField   bool   `help:"Enable size field on documents.. Requires mapper-size ElasticSearch plugin installed."`
	ArtistsURL  string `placeholder:"URL" name:"artists" default:"${defaultArtistsURL}" help:"URL of artists JSON to use. It can be a local file path, too. Default: ${defaultArtistsURL}."`
	ArtworksURL string `placeholder:"URL" name:"artworks" default:"${defaultArtworksURL}" help:"URL of artworks JSON to use. It can be a local file path, too. Default: ${defaultArtworksURL}."`
	WebsiteData bool   `help:"Fetch images and descriptions from MoMA website."`
}
