package main

import (
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
)

func (c *PopulateCommand) runIndex(globals *Globals, index string, sizeField bool) errors.E {
	ctx, _, _, esClient, processor, err := es.Initialize(globals.Log, globals.Elastic, index, sizeField)
	if err != nil {
		return err
	}

	err = search.SaveCoreProperties(ctx, globals.Log, esClient, processor, index)
	if err != nil {
		return err
	}

	return nil
}

func (c *PopulateCommand) Run(globals *Globals) errors.E {
	if len(globals.Sites) > 0 {
		for _, site := range globals.Sites {
			err := c.runIndex(globals, site.Index, site.SizeField)
			if err != nil {
				return err
			}
		}
	} else {
		err := c.runIndex(globals, globals.Index, globals.SizeField)
		if err != nil {
			return err
		}
	}

	globals.Log.Info().Msg("Done.")

	return nil
}
