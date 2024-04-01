package es

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// TODO: Generate index configuration automatically from document structs?

//go:embed index.json
var indexConfiguration []byte

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
	logger zerolog.Logger
}

func (a retryableHTTPLoggerAdapter) Error(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Error().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Info().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Debug(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Debug().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Warn(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Warn().Fields(keysAndValues).Msg(msg)
}

var _ retryablehttp.LeveledLogger = (*retryableHTTPLoggerAdapter)(nil)

type loggerAdapter struct {
	log   zerolog.Logger
	level zerolog.Level
}

type indexConfigurationStruct struct {
	Settings map[string]interface{} `json:"settings"`
	Mappings map[string]interface{} `json:"mappings"`
}

func (a loggerAdapter) Printf(format string, v ...interface{}) {
	a.log.WithLevel(a.level).Msgf(format, v...)
}

var _ elastic.Logger = (*loggerAdapter)(nil)

func GetClient(httpClient *http.Client, logger zerolog.Logger, url string) (*elastic.Client, errors.E) {
	esClient, err := elastic.NewClient(
		elastic.SetURL(url),
		elastic.SetHttpClient(httpClient),
		elastic.SetErrorLog(loggerAdapter{logger, zerolog.ErrorLevel}),
		// We use debug level here because logging at info level is too noisy.
		elastic.SetInfoLog(loggerAdapter{logger, zerolog.DebugLevel}),
		elastic.SetTraceLog(loggerAdapter{logger, zerolog.TraceLevel}),
		// TODO: Should this be a CLI parameter?
		// We disable sniffing and healthcheck so that Docker setup is easier.
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false),
	)
	return esClient, errors.WithStack(err)
}

// ensureIndex makes sure the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of ensureIndex would otherwise create.
func ensureIndex(ctx context.Context, esClient *elastic.Client, index string, sizeField bool) (*elastic.Client, errors.E) {
	exists, err := esClient.IndexExists(index).Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !exists {
		var config indexConfigurationStruct
		errE := x.UnmarshalWithoutUnknownFields(indexConfiguration, &config)
		if errE != nil {
			return nil, errE
		}

		if sizeField {
			config.Mappings["_size"] = map[string]interface{}{"enabled": true}
		}

		createIndex, err := esClient.CreateIndex(index).BodyJson(config).Do(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !createIndex.Acknowledged {
			// TODO: Wait for acknowledgment using Task API?
			return nil, errors.New("create index not acknowledged")
		}
	}

	return esClient, nil
}

func Init(ctx context.Context, logger zerolog.Logger, esClient *elastic.Client, index string, sizeField bool) (*elastic.BulkProcessor, errors.E) {
	esClient, errE := ensureIndex(ctx, esClient, index, sizeField)
	if errE != nil {
		return nil, errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).After(
		func(_ int64, _ []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				logger.Error().Err(err).Str("index", index).Msg("indexing error")
			} else if failed := response.Failed(); len(failed) > 0 {
				for _, f := range failed {
					logger.Error().
						Str("index", index).
						Str("id", f.Id).Int("code", f.Status).
						Str("reason", f.Error.Reason).Str("type", f.Error.Type).
						Msg("indexing error")
				}
			}
		},
		// Do's documentation states that passed context should not be used for cancellation,
		// so we pass a new context here and register context.AfterFunc later on.
	).Do(context.Background()) //nolint:contextcheck
	if err != nil {
		return nil, errors.WithStack(err)
	}

	context.AfterFunc(ctx, func() { processor.Close() })

	return processor, nil
}

func Standalone(logger zerolog.Logger, url, index string, sizeField bool) (
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

	esClient, errE := GetClient(simpleHTTPClient, logger, url)
	if errE != nil {
		return nil, nil, nil, nil, nil, errE
	}

	processor, errE := Init(ctx, logger, esClient, index, sizeField)
	if errE != nil {
		return nil, nil, nil, nil, nil, errE
	}

	httpClient := retryablehttp.NewClient()
	httpClient.HTTPClient = simpleHTTPClient
	httpClient.RetryWaitMax = clientRetryWaitMax
	httpClient.RetryMax = clientRetryMax
	httpClient.Logger = retryableHTTPLoggerAdapter{logger}

	// Set User-Agent header.
	httpClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, _ int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision))
	}

	return ctx, cancel, httpClient, esClient, processor, nil
}

func Progress(logger zerolog.Logger, processor *elastic.BulkProcessor, cache *Cache, skipped *int64, description string) func(ctx context.Context, p x.Progress) {
	if description == "" {
		description = "progress"
	}
	return func(_ context.Context, p x.Progress) {
		e := logger.Info()
		if processor != nil {
			stats := processor.Stats()
			e = e.Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded)
		}
		e = e.Int64("count", p.Count)
		if cache != nil {
			e = e.Uint64("cacheMiss", cache.MissCount())
		}
		e = e.Str("eta", p.Remaining().Truncate(time.Second).String())
		if skipped != nil {
			e = e.Int64("skipped", atomic.LoadInt64(skipped))
		}
		e.Msgf("%s %0.2f%%", description, p.Percent())
	}
}
