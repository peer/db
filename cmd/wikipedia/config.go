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
	CacheDir string `name:"cache" placeholder:"DIR" default:".cache" type:"path" help:"Where to cache files to."`
	Elastic  string `short:"e" placeholder:"URL" default:"http://127.0.0.1:9200" help:"URL of the ElasticSearch instance."`
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
