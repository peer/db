package main

import (
	"gitlab.com/tozd/go/errors"
)

type OptimizeCommand struct{}

func (c *OptimizeCommand) Run(globals *Globals) errors.E {
	ctx, cancel, _, esClient, processor, _, errE := initializeElasticSearch(globals)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	_, err := esClient.Forcemerge("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
