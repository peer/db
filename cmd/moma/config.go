package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/zerolog"
)

const (
	DefaultCacheDir    = ".cache"
	DefaultArtistsURL  = "https://github.com/MuseumofModernArt/collection/raw/main/Artists.json"
	DefaultArtworksURL = "https://github.com/MuseumofModernArt/collection/raw/main/Artworks.json"
)

//nolint:lll
type PostgresConfig struct {
	URL    kong.FileContentFlag `                           env:"URL_PATH" help:"File with PostgreSQL database URL. Environment variable: ${env}." placeholder:"PATH" required:"" short:"d"`
	Schema string               `default:"${defaultSchema}"                help:"Name of PostgreSQL schema to use. Default: ${default}."           placeholder:"NAME"             short:"s"`
}

type ElasticConfig struct {
	URL   string `default:"${defaultElastic}" help:"URL of the ElasticSearch instance. Default: ${default}."                              placeholder:"URL"  short:"e"`
	Index string `default:"${defaultIndex}"   help:"Name of ElasticSearch index to use. Default: ${default}."                             placeholder:"NAME" short:"i"`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
//
//nolint:lll
type Config struct {
	zerolog.LoggingConfig

	Version     kong.VersionFlag `                                                               help:"Show program's version and exit."                                                                                                         short:"V"`
	CacheDir    string           `default:"${defaultCacheDir}"                                   help:"Where to cache files to. Default: ${default}."                                       name:"cache"    placeholder:"DIR"                    short:"C" type:"path"`
	Postgres    PostgresConfig   `                                embed:"" envprefix:"POSTGRES_"                                                                                                                              prefix:"postgres."`
	Elastic     ElasticConfig    `                                embed:"" envprefix:"ELASTIC_"                                                                                                                               prefix:"elastic."`
	ArtistsURL  string           `default:"${defaultArtistsURL}"                                 help:"URL of artists JSON to use. It can be a local file path, too. Default: ${default}."  name:"artists"  placeholder:"URL"`
	ArtworksURL string           `default:"${defaultArtworksURL}"                                help:"URL of artworks JSON to use. It can be a local file path, too. Default: ${default}." name:"artworks" placeholder:"URL"`
	WebsiteData bool             `                                                               help:"Fetch images and descriptions from MoMA website."`
}
