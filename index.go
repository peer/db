package search

import (
	"context"
	_ "embed"
	"net/http"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"

	"gitlab.com/tozd/go/errors"
)

// TODO: Generate index configuration automatically from document structs?

//go:embed index.json
var indexConfiguration string

type loggerAdapter struct {
	log   zerolog.Logger
	level zerolog.Level
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

// EnsureIndex creates an instance of the ElasticSearch client and makes sure
// the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of EnsureIndex would otherwise create.
func EnsureIndex(ctx context.Context, httpClient *http.Client, logger zerolog.Logger, url, index string) (*elastic.Client, errors.E) {
	esClient, errE := GetClient(httpClient, logger, url)
	if errE != nil {
		return nil, errE
	}

	exists, err := esClient.IndexExists(index).Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !exists {
		createIndex, err := esClient.CreateIndex(index).BodyString(indexConfiguration).Do(ctx)
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
