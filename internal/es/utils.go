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
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/storage"
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
		elastic.SetURL(strings.TrimSpace(url)),
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
func ensureIndex(ctx context.Context, esClient *elastic.Client, index string) errors.E {
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

func endDocumentSession(
	ctx context.Context, s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	c *coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata],
	session identifier.Identifier, endMetadata *types.DocumentEndMetadata,
) (*types.DocumentEndMetadata, errors.E) {
	if endMetadata.Discarded {
		return nil, nil //nolint:nilnil
	}

	beginMetadata, _, errE := c.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	// TODO: Support more than 5000 changes.
	changesList, errE := c.List(ctx, session, nil)
	if errE != nil {
		return nil, errE
	}

	// changesList is sorted from newest to oldest change, but we want the opposite as we have forward patches.
	slices.Reverse(changesList)

	changes := make(document.Changes, 0, len(changesList))
	for _, ch := range changesList {
		data, _, errE := c.GetData(ctx, session, ch) //nolint:govet
		if errE != nil {
			errors.Details(errE)["change"] = ch
			return nil, errE
		}
		change, errE := document.ChangeUnmarshalJSON(data)
		if errE != nil {
			errors.Details(errE)["change"] = ch
			return nil, errE
		}
		changes = append(changes, change)
	}

	// TODO: Get latest revision at the same changeset?
	docJSON, _, errE := s.Get(ctx, beginMetadata.ID, beginMetadata.Version)
	if errE != nil {
		return nil, errE
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(docJSON, &doc)
	if errE != nil {
		return nil, errE
	}

	errE = changes.Apply(&doc)
	if errE != nil {
		return nil, errE
	}

	docJSON, errE = x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return nil, errE
	}

	metadata := &types.DocumentMetadata{
		At: beginMetadata.At,
	}

	version, errE := s.Update(ctx, beginMetadata.ID, beginMetadata.Version.Changeset, docJSON, changes, metadata, &types.NoMetadata{})
	if errE != nil {
		return nil, errE
	}

	endMetadata.Changeset = &version.Changeset
	endMetadata.Time = time.Since(time.Time(endMetadata.At)).Milliseconds()
	return endMetadata, nil
}

func NewHTTPClient(simpleHTTPClient *http.Client, logger zerolog.Logger) *retryablehttp.Client {
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

	return httpClient
}

func Standalone(logger zerolog.Logger, database, elastic, schema, index string) (
	context.Context, context.CancelFunc, *retryablehttp.Client,
	*store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	*elastic.Client, *elastic.BulkProcessor, errors.E,
) {
	// We stop the server gracefully on ctrl-c and TERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	dbpool, errE := internal.InitPostgres(ctx, database, logger, func(_ context.Context) (string, string) {
		return schema, "standalone"
	})
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	simpleHTTPClient := cleanhttp.DefaultPooledClient()

	esClient, errE := GetClient(simpleHTTPClient, logger, elastic)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	store, _, _, esProcessor, errE := InitForSite(ctx, logger, dbpool, esClient, schema, index)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	httpClient := NewHTTPClient(simpleHTTPClient, logger)

	return ctx, stop, httpClient, store, esClient, esProcessor, nil
}

func Progress(logger zerolog.Logger, esProcessor *elastic.BulkProcessor, cache *Cache, skipped *int64, description string) func(ctx context.Context, p x.Progress) {
	if description == "" {
		description = "progress"
	}
	return func(_ context.Context, p x.Progress) {
		e := logger.Info().
			Int64("count", p.Count).
			Int64("total", p.Size).
			// We format it ourselves. See: https://github.com/rs/zerolog/issues/709
			Str("eta", p.Remaining().Truncate(time.Second).String()).
			Float64("%", p.Percent())
		if esProcessor != nil {
			stats := esProcessor.Stats()
			e = e.Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded)
		}
		if cache != nil {
			e = e.Uint64("cacheMiss", cache.MissCount())
		}
		if skipped != nil {
			e = e.Int64("skipped", atomic.LoadInt64(skipped))
		}
		e.Msg(description)
	}
}

func InitForSite(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, esClient *elastic.Client, schema, index string,
) (
	*store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	*coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata],
	*storage.Storage,
	*elastic.BulkProcessor,
	errors.E,
) {
	// TODO: Add some monitoring of the channel contention.
	channel := make(
		chan store.CommittedChangeset[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
		bridgeBufferSize,
	)
	context.AfterFunc(ctx, func() { close(channel) })

	errE := ensureIndex(ctx, esClient, index)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	}, nil)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	esProcessor, errE := initProcessor(ctx, logger, esClient, index)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	s := &store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]{
		Prefix:       "docs",
		Committed:    channel,
		DataType:     "jsonb",
		MetadataType: "jsonb",
		PatchType:    "jsonb",
	}
	errE = s.Init(ctx, dbpool)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	var c *coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata]
	c = &coordinator.Coordinator[json.RawMessage, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentChangeMetadata]{
		Prefix:       "docs",
		DataType:     "jsonb",
		MetadataType: "jsonb",
		EndCallback: func(ctx context.Context, session identifier.Identifier, metadata *types.DocumentEndMetadata) (*types.DocumentEndMetadata, errors.E) {
			return endDocumentSession(ctx, s, c, session, metadata)
		},
		Appended: nil,
		Ended:    nil,
	}
	errE = c.Init(ctx, dbpool)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	storage := &storage.Storage{
		Prefix:    "storage",
		Committed: nil,
	}
	errE = storage.Init(ctx, dbpool)
	if errE != nil {
		return nil, nil, nil, nil, errE
	}

	go Bridge(
		ctx,
		logger.With().Str("schema", schema).Str("index", index).Logger(),
		s,
		esProcessor,
		index,
		channel,
	)

	return s, c, storage, esProcessor, nil
}
