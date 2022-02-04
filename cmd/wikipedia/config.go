package main

import (
	"time"

	"github.com/alecthomas/kong"
	"gitlab.com/tozd/go/errors"
)

const (
	bulkProcessorWorkers = 2
	clientRetryWaitMax   = 10 * 60 * time.Second
	clientRetryMax       = 9
)

// Globals describes top-level (global) flags.
type Globals struct {
	Version  kong.VersionFlag `short:"V" help:"Show program's version and exit."`
	CacheDir string           `name:"cache" placeholder:"DIR" default:".cache" type:"path" help:"Where to cache files to."`
}

// Config provides configuration.
// It is used as configuration for Kong command-line parser as well.
type Config struct {
	Globals

	// TODO: Populate with Wikimedia Commons entities.
	// TODO: Populate with Wikimedia Commons file descriptions rendered in HTML.
	CommonsMediaInfo   CommonsMediaInfoCommand   `cmd:"" name:"commons-mediainfo" help:"Populate search with Wikimedia Commons mediainfo SQL dump."`
	Wikidata           WikidataCommand           `cmd:"" help:"Populate search with Wikidata entities dump."`
	Prepare            PrepareCommand            `cmd:"" help:"Prepare populated data for search."`
	WikipediaMediaInfo WikipediaMediaInfoCommand `cmd:"" name:"wikipedia-mediainfo" help:"Populate search with Wikipedia mediainfo SQL dump."`
	WikipediaFiles     WikipediaFilesCommand     `cmd:"" help:"Populate search with Wikipedia file descriptions HTML dump."`
	WikipediaArticles  WikipediaArticlesCommand  `cmd:"" help:"Populate search with Wikipedia articles HTML dump."`

	All AllCommand `cmd:"" default:"" help:"Run all passes in order. Default command."`
}

type runner interface {
	Run(*Globals) errors.E
}

type AllCommand struct{}

func (c *AllCommand) Run(globals *Globals) errors.E {
	allCommands := []runner{
		&CommonsMediaInfoCommand{},
		&WikidataCommand{},
		&PrepareCommand{},
		&WikipediaMediaInfoCommand{},
		&WikipediaFilesCommand{},
		&WikipediaArticlesCommand{},
	}

	for _, command := range allCommands {
		err := command.Run(globals)
		if err != nil {
			return err
		}
	}

	return nil
}
