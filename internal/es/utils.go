package es

import (
	"context"
	_ "embed"
	"encoding/json"
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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Generate index configuration automatically from document structs?

//go:embed index.json
var indexConfiguration []byte

const (
	bulkProcessorWorkers = 2
	bulkActions          = 1000
	flushInterval        = time.Second
	clientRetryWaitMax   = 10 * 60 * time.Second
	clientRetryMax       = 9
	// TODO: Determine reasonable size for the buffer.
	bridgeBufferSize = 100
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
func ensureIndex(ctx context.Context, esClient *elastic.Client, index string, sizeField bool) errors.E {
	exists, err := esClient.IndexExists(index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if !exists {
		var config indexConfigurationStruct
		errE := x.UnmarshalWithoutUnknownFields(indexConfiguration, &config)
		if errE != nil {
			return errE
		}

		if sizeField {
			config.Mappings["_size"] = map[string]interface{}{"enabled": true}
		}

		createIndex, err := esClient.CreateIndex(index).BodyJson(config).Do(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if !createIndex.Acknowledged {
			// TODO: Wait for acknowledgment using Task API?
			return errors.New("create index not acknowledged")
		}
	}

	return nil
}

func initProcessor(ctx context.Context, logger zerolog.Logger, esClient *elastic.Client, index string) (*elastic.BulkProcessor, errors.E) {
	// TODO: Make number of workers configurable.
	// TODO: Make bulk actions configurable.
	// TODO: Make flush interval configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).BulkActions(bulkActions).FlushInterval(flushInterval).After(
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

func Standalone(logger zerolog.Logger, database, elastic, schema, index string, sizeField bool) (
	context.Context, context.CancelFunc, *retryablehttp.Client,
	*store.Store[json.RawMessage, json.RawMessage, json.RawMessage],
	*elastic.Client, *elastic.BulkProcessor, errors.E,
) {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	dbpool, errE := internal.InitPostgres(ctx, database, logger, func(_ context.Context) (string, string) {
		return schema, "moma"
	})
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	simpleHTTPClient := cleanhttp.DefaultPooledClient()

	esClient, errE := GetClient(simpleHTTPClient, logger, elastic)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	store, esProcessor, errE := InitForSite(ctx, logger, dbpool, esClient, schema, index, sizeField)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
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

	return ctx, stop, httpClient, store, esClient, esProcessor, nil
}

func Progress(logger zerolog.Logger, esProcessor *elastic.BulkProcessor, cache *Cache, skipped *int64, description string) func(ctx context.Context, p x.Progress) {
	if description == "" {
		description = "progress"
	}
	return func(_ context.Context, p x.Progress) {
		e := logger.Info()
		if esProcessor != nil {
			stats := esProcessor.Stats()
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

func InitForSite(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client, schema, index string, sizeField bool,
) (*store.Store[json.RawMessage, json.RawMessage, json.RawMessage], *elastic.BulkProcessor, errors.E) {
	// TODO: Add some monitoring of the channel contention.
	channel := make(chan store.CommittedChangeset[json.RawMessage, json.RawMessage, json.RawMessage], bridgeBufferSize)
	context.AfterFunc(ctx, func() { close(channel) })

	errE := ensureIndex(ctx, esClient, index, sizeField)
	if errE != nil {
		return nil, nil, errE
	}

	esProcessor, errE := initProcessor(ctx, logger, esClient, index)
	if errE != nil {
		return nil, nil, errE
	}

	store := &store.Store[json.RawMessage, json.RawMessage, json.RawMessage]{
		Schema:       schema,
		Committed:    channel,
		DataType:     "jsonb",
		MetadataType: "jsonb",
		PatchType:    "jsonb",
	}
	errE = store.Init(ctx, dbpool)
	if errE != nil {
		return nil, nil, errE
	}

	go Bridge(
		ctx,
		logger.With().Str("schema", schema).Str("index", index).Logger(),
		store,
		esProcessor,
		index,
		channel,
	)

	return store, esProcessor, nil
}
