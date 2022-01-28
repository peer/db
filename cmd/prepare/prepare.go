package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func prepare(config *Config) errors.E {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	esClient, errE := search.EnsureIndex(ctx)
	if errE != nil {
		return errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(2).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Indexing error: %s\n", err.Error())
			}
		},
	).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer processor.Close()

	errE = saveStandardProperties(ctx, config, esClient)
	if errE != nil {
		return errE
	}

	return updateEmbeddedDocuments(ctx, config, esClient, processor)
}

func saveStandardProperties(ctx context.Context, config *Config, esClient *elastic.Client) errors.E {
	for id, property := range search.StandardProperties {
		// We do not use a bulk processor because we want these documents to be available immediately.
		_, err := esClient.Index().Index("docs").Id(id).BodyJson(&property).Do(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	// Make sure all added documents are available for search.
	_, err := esClient.Refresh().Index("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
