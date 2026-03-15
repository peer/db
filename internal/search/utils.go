package search

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/indexer"
)

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

// GetClient creates and configures an Elasticsearch client with the specified HTTP client, logger, and URL.
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

// EnsureIndex makes sure the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of EnsureIndex would otherwise create.
func EnsureIndex(ctx context.Context, esClient *elastic.Client, index string) errors.E {
	exists, err := esClient.IndexExists(index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if !exists {
		indexConfiguration, errE := Mapping()
		if errE != nil {
			return errE
		}
		var config indexConfigurationStruct
		errE = x.UnmarshalWithoutUnknownFields(indexConfiguration, &config)
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

// NewHTTPClient creates a retryable HTTP client with the specified base HTTP client and logger.
func NewHTTPClient(logger zerolog.Logger, httpClient *http.Client) *http.Client {
	// TODO: Make contact e-mail into a CLI argument.
	return indexer.NewHTTPClient(logger, httpClient, fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision)) //nolint:lll
}
