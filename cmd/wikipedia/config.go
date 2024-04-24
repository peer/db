package main

import (
	"reflect"

	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/zerolog"
)

const (
	DefaultAPILimit = "50"
	DefaultCacheDir = ".cache"
)

// Globals describes top-level (global) flags.
//
//nolint:lll
type Globals struct {
	zerolog.LoggingConfig

	Version                kong.VersionFlag     `                                                 help:"Show program's version and exit."                                                                                                        short:"V"`
	CacheDir               string               `default:"${defaultCacheDir}"                     help:"Where to cache files to. Default: ${defaultCacheDir}."                                       name:"cache" placeholder:"DIR"              short:"C" type:"path"`
	Database               kong.FileContentFlag `                             env:"DATABASE_PATH" help:"File with PostgreSQL database URL. Environment variable: ${env}."                                         placeholder:"PATH" required:"" short:"d"`
	Elastic                string               `default:"${defaultElastic}"                      help:"URL of the ElasticSearch instance. Default: ${defaultElastic}."                                           placeholder:"URL"              short:"e"`
	Index                  string               `default:"${defaultIndex}"                        help:"Name of ElasticSearch index to use. Default: ${defaultIndex}."                                            placeholder:"NAME"             short:"i"`
	Schema                 string               `default:"${defaultSchema}"                       help:"Name of PostgreSQL schema to use Default: ${defaultSchema}."                                              placeholder:"NAME"             short:"s"`
	SizeField              bool                 `                                                 help:"Enable size field on documents.. Requires mapper-size ElasticSearch plugin installed."`
	DecompressionThreads   int                  `default:"0"                                      help:"The number of threads used for decompression. Defaults to the number of available cores."                 placeholder:"INT"`
	DecodingThreads        int                  `default:"0"                                      help:"The number of threads used for decoding. Defaults to the number of available cores."                      placeholder:"INT"`
	ItemsProcessingThreads int                  `default:"0"                                      help:"The number of threads used for items processing. Defaults to the number of available cores."              placeholder:"INT"`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals

	// First we create all documents we search for: Wikidata entities and Wikimedia Commons and Wikipedia files.
	Wikidata       WikidataCommand       `cmd:"" help:"Populate search with Wikidata entities dump."`
	CommonsFiles   CommonsFilesCommand   `cmd:"" help:"Populate search with Wikimedia Commons files from image table SQL dump." name:"commons-files"`
	WikipediaFiles WikipediaFilesCommand `cmd:"" help:"Populate search with Wikipedia files from image table SQL dump."         name:"wikipedia-files"`

	// Then we add claims from entities of Wikimedia Commons files.
	Commons CommonsCommand `cmd:"" help:"Populate search with Wikimedia Commons entities dump."`

	// We add descriptions from HTML dumps.
	WikipediaArticles         WikipediaArticlesCommand         `cmd:"" help:"Populate search with Wikipedia articles HTML dump."          name:"wikipedia-articles"`
	WikipediaFileDescriptions WikipediaFileDescriptionsCommand `cmd:"" help:"Populate search with Wikipedia file descriptions HTML dump." name:"wikipedia-file-descriptions"`
	WikipediaCategories       WikipediaCategoriesCommand       `cmd:"" help:"Populate search with Wikipedia categories HTML dump."        name:"wikipedia-categories"`

	// Not everything is available as dumps, so we fetch using API.
	WikipediaTemplates      WikipediaTemplatesCommand      `cmd:"" help:"Populate search with Wikipedia templates using API."                 name:"wikipedia-templates"`
	CommonsFileDescriptions CommonsFileDescriptionsCommand `cmd:"" help:"Populate search with Wikimedia Commons file descriptions using API." name:"commons-file-descriptions"` //nolint:lll
	CommonsCategories       CommonsCategoriesCommand       `cmd:"" help:"Populate search with Wikimedia Commons categories using API."        name:"commons-categories"`
	CommonsTemplates        CommonsTemplatesCommand        `cmd:"" help:"Populate search with Wikimedia Commons templates using API."         name:"commons-templates"`

	Prepare  PrepareCommand  `cmd:"" help:"Prepare populated data for search."`
	Optimize OptimizeCommand `cmd:"" help:"Optimize search data."`

	All AllCommand `cmd:"" default:"" help:"Run all passes in order using latest dumps. Default command."`
}

type runner interface {
	Run(*Globals) errors.E
}

//nolint:lll
type AllCommand struct {
	WikidataSaveSkipped          string `help:"Save IDs of skipped Wikidata entities."                                                                                                          placeholder:"PATH" type:"path"`
	CommonsSaveSkipped           string `help:"Save filenames of skipped Wikimedia Commons files."                                                                                              placeholder:"PATH" type:"path"`
	WikipediaSaveSkipped         string `help:"Save filenames of skipped Wikipedia files."                                                                                                      placeholder:"PATH" type:"path"`
	WikidataURL                  string `help:"URL of Wikidata entities JSON dump to use. It can be a local file path, too. Default: the latest."            name:"wikidata"                    placeholder:"URL"`
	CommonsFilesURL              string `help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest." name:"commons-files"               placeholder:"URL"`
	WikipediaFilesURL            string `help:"URL of Wikipedia image table SQL dump to use. It can be a local file path, too. Default: the latest."         name:"wikipedia-files"             placeholder:"URL"`
	CommonsURL                   string `help:"URL of Wikimedia Commons entities JSON dump to use. It can be a local file path, too. Default: the latest."   name:"commons"                     placeholder:"URL"`
	WikipediaArticlesURL         string `help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."           name:"wikipedia-articles"          placeholder:"URL"`
	WikipediaFileDescriptionsURL string `help:"URL of Wikipedia file descriptions HTML dump to use. It can be a local file path, too. Default: the latest."  name:"wikipedia-file-descriptions" placeholder:"URL"`
	WikipediaCategoriesURL       string `help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."           name:"wikipedia-categories"        placeholder:"URL"`
}

func (c *AllCommand) Run(globals *Globals) errors.E {
	//nolint:exhaustruct
	allCommands := []runner{
		&WikidataCommand{
			SaveSkipped: c.WikidataSaveSkipped,
			URL:         c.WikidataURL,
		},
		&CommonsFilesCommand{
			SaveSkipped: c.CommonsSaveSkipped,
			URL:         c.CommonsFilesURL,
		},
		&WikipediaFilesCommand{
			SaveSkipped: c.WikidataSaveSkipped,
			URL:         c.WikipediaFilesURL,
		},
		&CommonsCommand{
			URL: c.CommonsURL,
		},
		&WikipediaArticlesCommand{
			URL: c.WikipediaArticlesURL,
		},
		&WikipediaFileDescriptionsCommand{
			URL: c.WikipediaFileDescriptionsURL,
		},
		&WikipediaCategoriesCommand{
			URL: c.WikipediaCategoriesURL,
		},
		&WikipediaTemplatesCommand{},
		&CommonsFileDescriptionsCommand{},
		&CommonsCategoriesCommand{},
		&CommonsTemplatesCommand{},
		&PrepareCommand{},
		&OptimizeCommand{},
	}

	for _, command := range allCommands {
		globals.Logger.Info().Msgf("running command %s", reflect.TypeOf(command).Elem().Name())
		err := command.Run(globals)
		if err != nil {
			return err
		}
	}

	return nil
}
