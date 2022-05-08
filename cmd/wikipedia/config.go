package main

import (
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
	CacheDir               string `name:"cache" placeholder:"DIR" default:".cache" type:"path" help:"Where to cache files to."`
	Elastic                string `short:"e" placeholder:"URL" default:"http://127.0.0.1:9200" help:"URL of the ElasticSearch instance."`
	Token                  string `placeholder:"TOKEN" env:"MEDIAWIKI_TOKEN" help:"Access token for Mediawiki API. Not required. Environment variable: ${env}"`
	APILimit               int    `placeholder:"INT" default:"50" help:"Maximum number of titles to work on in a single API request. Use 500 if you have an access token with higher limits."` //nolint:lll
	DecompressionThreads   int    `placeholder:"INT" default:"0" help:"The number of threads used for decompression. Defaults to the number of available cores."`
	DecodingThreads        int    `placeholder:"INT" default:"0" help:"The number of threads used for decoding. Defaults to the number of available cores."`
	ItemsProcessingThreads int    `placeholder:"INT" default:"0" help:"The number of threads used for items processing. Defaults to the number of available cores."`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals

	// TODO: Populate with Wikimedia Commons entities.
	// TODO: Populate with Wikimedia Commons file descriptions rendered in HTML.
	CommonsFiles              CommonsFilesCommand              `cmd:"" name:"commons-files" help:"Populate search with Wikimedia Commons files from image table SQL dump."`
	WikipediaFiles            WikipediaFilesCommand            `cmd:"" name:"wikipedia-files" help:"Populate search with Wikipedia files from image table SQL dump."`
	Wikidata                  WikidataCommand                  `cmd:"" help:"Populate search with Wikidata entities dump."`
	WikipediaFileDescriptions WikipediaFileDescriptionsCommand `cmd:"" name:"wikipedia-file-descriptions" help:"Populate search with Wikipedia file descriptions HTML dump."`
	WikipediaArticles         WikipediaArticlesCommand         `cmd:"" name:"wikipedia-articles" help:"Populate search with Wikipedia articles HTML dump."`
	WikipediaCategories       WikipediaCategoriesCommand       `cmd:"" name:"wikipedia-categories" help:"Populate search with Wikipedia categories HTML dump."`
	WikipediaTemplates        WikipediaTemplatesCommand        `cmd:"" name:"wikipedia-templates" help:"Populate search with Wikipedia templates using API."`
	Prepare                   PrepareCommand                   `cmd:"" help:"Prepare populated data for search."`
	Optimize                  OptimizeCommand                  `cmd:"" help:"Optimize search data."`

	All AllCommand `cmd:"" default:"" help:"Run all passes in order. Default command."`
}

type runner interface {
	Run(*Globals) errors.E
}

type AllCommand struct{}

func (c *AllCommand) Run(globals *Globals) errors.E {
	allCommands := []runner{
		&CommonsFilesCommand{},
		&WikipediaFilesCommand{},
		&WikidataCommand{},
		&WikipediaFileDescriptionsCommand{},
		&WikipediaArticlesCommand{},
		&WikipediaCategoriesCommand{},
		&WikipediaTemplatesCommand{},
		&PrepareCommand{},
		&OptimizeCommand{},
	}

	for _, command := range allCommands {
		err := command.Run(globals)
		if err != nil {
			return err
		}
	}

	return nil
}
