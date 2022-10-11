package es

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/cli"
)

const (
	bulkProcessorWorkers = 2
	clientRetryWaitMax   = 10 * 60 * time.Second
	clientRetryMax       = 9
)

func prepareFields(keysAndValues []interface{}) {
	for i, keyOrValue := range keysAndValues {
		// We want URLs logged as strings.
		u, ok := keyOrValue.(*url.URL)
		if ok {
			keysAndValues[i] = u.String()
		}
	}
}

type retryableHTTPLoggerAdapter struct {
	log zerolog.Logger
}

func (a retryableHTTPLoggerAdapter) Error(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Error().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Info().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Debug(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Debug().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Warn(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Warn().Fields(keysAndValues).Msg(msg)
}

var _ retryablehttp.LeveledLogger = (*retryableHTTPLoggerAdapter)(nil)

func Initialize(logger zerolog.Logger, url, index string) (
	context.Context, context.CancelFunc, *retryablehttp.Client, *elastic.Client, *elastic.BulkProcessor, errors.E,
) {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)

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

	simpleHTTPClient := cleanhttp.DefaultPooledClient()

	esClient, errE := search.EnsureIndex(ctx, simpleHTTPClient, logger, url, index)
	if errE != nil {
		return nil, nil, nil, nil, nil, errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				logger.Error().Err(err).Msg("indexing error")
			} else if failed := response.Failed(); len(failed) > 0 {
				for _, f := range failed {
					logger.Error().
						Str("id", f.Id).Int("code", f.Status).
						Str("reason", f.Error.Reason).Str("type", f.Error.Type).
						Msg("indexing error")
				}
			}
		},
	).Do(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.WithStack(err)
	}

	httpClient := retryablehttp.NewClient()
	httpClient.HTTPClient = simpleHTTPClient
	httpClient.RetryWaitMax = clientRetryWaitMax
	httpClient.RetryMax = clientRetryMax
	httpClient.Logger = retryableHTTPLoggerAdapter{logger}

	// Set User-Agent header.
	httpClient.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision))
	}

	return ctx, cancel, httpClient, esClient, processor, nil
}
