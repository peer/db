package main

import (
	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/zerolog"
)

const (
	DefaultCacheDir = ".cache"
)

//nolint:lll
type PostgresConfig struct {
	URL    kong.FileContentFlag `                           env:"URL_PATH" help:"File with PostgreSQL database URL. Environment variable: ${env}." placeholder:"PATH" required:"" short:"d"`
	Schema string               `default:"${defaultSchema}"                help:"Name of PostgreSQL schema to use. Default: ${defaultSchema}."     placeholder:"NAME"             short:"s"`
}

type ElasticConfig struct {
	URL       string `default:"${defaultElastic}" help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."                       placeholder:"URL"  short:"e"`
	Index     string `default:"${defaultIndex}"   help:"Name of ElasticSearch index to use. Default: ${defaultIndex}."                        placeholder:"NAME" short:"i"`
	SizeField bool   `                            help:"Enable size field on documents. Requires mapper-size ElasticSearch plugin installed."`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
//
//nolint:lll
type Config struct {
	zerolog.LoggingConfig

	Version  kong.VersionFlag `                                                            help:"Show program's version and exit."                                                                        short:"V"`
	CacheDir string           `default:"${defaultCacheDir}"                                help:"Where to cache files to. Default: ${defaultCacheDir}." name:"cache" placeholder:"DIR"                    short:"C" type:"path"`
	Postgres PostgresConfig   `                             embed:"" envprefix:"POSTGRES_"                                                                                             prefix:"postgres."`
	Elastic  ElasticConfig    `                             embed:"" envprefix:"ELASTIC_"                                                                                              prefix:"elastic."`

	FoodDataCentral     FoodDataCentral     `embed:"" prefix:"fooddatacentral."`
	Natureta            Natureta            `embed:"" prefix:"natureta."`
	LjubljanskeMlekarne LjubljanskeMlekarne `embed:"" prefix:"ljubljanskemlekarne."`
	Zito                Zito                `embed:"" prefix:"zito."`
	FURSDEJ             FURSDEJ             `embed:"" prefix:"fursdej."`
	Datakick            Datakick            `embed:"" prefix:"datakick."`
}
