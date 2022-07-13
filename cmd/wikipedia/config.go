package main

import (
	"reflect"
	"time"

	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/cli"
)

const (
	bulkProcessorWorkers = 2
	clientRetryWaitMax   = 10 * 60 * time.Second
	clientRetryMax       = 9
)

// Globals describes top-level (global) flags.
type Globals struct {
	Version kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	cli.LoggingConfig
	CacheDir               string `name:"cache" placeholder:"DIR" default:".cache" type:"path" help:"Where to cache files to. Default: ${default}."`
	Elastic                string `short:"e" placeholder:"URL" default:"http://127.0.0.1:9200" help:"URL of the ElasticSearch instance. Default: ${default}."`
	Index                  string `placeholder:"NAME" default:"docs" help:"Name of ElasticSearch index to use. Default: ${default}."`
	DecompressionThreads   int    `placeholder:"INT" default:"0" help:"The number of threads used for decompression. Defaults to the number of available cores."`
	DecodingThreads        int    `placeholder:"INT" default:"0" help:"The number of threads used for decoding. Defaults to the number of available cores."`
	ItemsProcessingThreads int    `placeholder:"INT" default:"0" help:"The number of threads used for items processing. Defaults to the number of available cores."`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals

	// First we create all documents we search for: Wikidata entities and Wikimedia Commons and Wikipedia files.
	Wikidata       WikidataCommand       `cmd:"" help:"Populate search with Wikidata entities dump."`
	CommonsFiles   CommonsFilesCommand   `cmd:"" name:"commons-files" help:"Populate search with Wikimedia Commons files from image table SQL dump."`
	WikipediaFiles WikipediaFilesCommand `cmd:"" name:"wikipedia-files" help:"Populate search with Wikipedia files from image table SQL dump."`

	// Then we add claims from entities of Wikimedia Commons files.
	Commons CommonsCommand `cmd:"" help:"Populate search with Wikimedia Commons entities dump."`

	// We add descriptions from HTML dumps.
	WikipediaArticles         WikipediaArticlesCommand         `cmd:"" name:"wikipedia-articles" help:"Populate search with Wikipedia articles HTML dump."`
	WikipediaFileDescriptions WikipediaFileDescriptionsCommand `cmd:"" name:"wikipedia-file-descriptions" help:"Populate search with Wikipedia file descriptions HTML dump."`
	WikipediaCategories       WikipediaCategoriesCommand       `cmd:"" name:"wikipedia-categories" help:"Populate search with Wikipedia categories HTML dump."`

	// Not everything is available as dumps, so we fetch using API.
	WikipediaTemplates      WikipediaTemplatesCommand      `cmd:"" name:"wikipedia-templates" help:"Populate search with Wikipedia templates using API."`
	CommonsFileDescriptions CommonsFileDescriptionsCommand `cmd:"" name:"commons-file-descriptions" help:"Populate search with Wikimedia Commons file descriptions using API."` //nolint:lll
	CommonsCategories       CommonsCategoriesCommand       `cmd:"" name:"commons-categories" help:"Populate search with Wikimedia Commons categories using API."`
	CommonsTemplates        CommonsTemplatesCommand        `cmd:"" name:"commons-templates" help:"Populate search with Wikimedia Commons templates using API."`

	Prepare  PrepareCommand  `cmd:"" help:"Prepare populated data for search."`
	Optimize OptimizeCommand `cmd:"" help:"Optimize search data."`

	All AllCommand `cmd:"" default:"" help:"Run all passes in order using latest dumps. Default command."`
}

type runner interface {
	Run(*Globals) errors.E
}

//nolint:lll
type AllCommand struct {
	WikidataSaveSkipped          string `placeholder:"PATH" type:"path" help:"Save IDs of skipped Wikidata entities."`
	CommonsSaveSkipped           string `placeholder:"PATH" type:"path" help:"Save filenames of skipped Wikimedia Commons files."`
	WikipediaSaveSkipped         string `placeholder:"PATH" type:"path" help:"Save filenames of skipped Wikipedia files."`
	WikidataURL                  string `name:"wikidata" placeholder:"URL" help:"URL of Wikidata entities JSON dump to use. It can be a local file path, too. Default: the latest."`
	CommonsFilesURL              string `name:"commons-files" placeholder:"URL" help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest."`
	WikipediaFilesURL            string `name:"wikipedia-files" placeholder:"URL" help:"URL of Wikipedia image table SQL dump to use. It can be a local file path, too. Default: the latest."`
	CommonsURL                   string `name:"commons" placeholder:"URL" help:"URL of Wikimedia Commons entities JSON dump to use. It can be a local file path, too. Default: the latest."`
	WikipediaArticlesURL         string `name:"wikipedia-articles" placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
	WikipediaFileDescriptionsURL string `name:"wikipedia-file-descriptions" placeholder:"URL" help:"URL of Wikipedia file descriptions HTML dump to use. It can be a local file path, too. Default: the latest."`
	WikipediaCategoriesURL       string `name:"wikipedia-categories" placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *AllCommand) Run(globals *Globals) errors.E {
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
		globals.Log.Info().Msgf("running command %s", reflect.TypeOf(command).Elem().Name())
		err := command.Run(globals)
		if err != nil {
			return err
		}
	}

	return nil
}
