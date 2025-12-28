package main

import (
	"gitlab.com/tozd/go/errors"
)

type OptimizeCommand struct{}

func (c *OptimizeCommand) Run(globals *Globals) errors.E {
	ctx, stop, _, _, esClient, esProcessor, _, errE := initializeElasticSearch(globals) //nolint:dogsled
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close() //nolint:errcheck

	_, err := esClient.Forcemerge(globals.Elastic.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
