package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

// A silent logger.
type nullLogger struct{}

func (nullLogger) Error(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Info(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Debug(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Warn(msg string, keysAndValues ...interface{}) {}

func convert(config *Config) errors.E {
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

	client := retryablehttp.NewClient()

	// We silent debug logging from HTTP client.
	// TODO: Configure proper logger.
	client.Logger = nullLogger{}

	// Set User-Agent header.
	client.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision))
	}

	esClient, err := elastic.NewClient()
	if err != nil {
		return errors.WithStack(err)
	}

	_, _, err = esClient.Ping(elastic.DefaultURL).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	exists, err := esClient.IndexExists("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if !exists {
		createIndex, err := esClient.CreateIndex("docs").BodyString(search.IndexConfiguration).Do(ctx) //nolint:govet
		if err != nil {
			return errors.WithStack(err)
		}
		if !createIndex.Acknowledged {
			return errors.New("create index not acknowledged")
		}
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

	return mediawiki.ProcessWikidataDump(ctx, &mediawiki.ProcessDumpConfig{
		URL:                    "",
		CacheDir:               config.CacheDir,
		Client:                 client,
		DecompressionThreads:   0,
		JSONDecodeThreads:      0,
		ItemsProcessingThreads: 0,
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s, indexed: %d, failed: %d\n", p.Percent(), p.Remaining().Truncate(time.Second), stats.Indexed, stats.Failed)
		},
	}, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return processEntity(ctx, config, client, processor, entity)
	})
}
