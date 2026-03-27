package search

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/indexer"
)

type loggerAdapter struct {
	log zerolog.Logger
}

type indexConfigurationStruct struct {
	Settings map[string]interface{} `json:"settings"`
	Mappings map[string]interface{} `json:"mappings"`
}

// LogRoundTrip logs the request and response details using zerolog.
func (a loggerAdapter) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	event := a.log.Debug()
	if err != nil {
		event = a.log.Error().Err(err)
	} else if res != nil && res.StatusCode >= http.StatusBadRequest {
		event = a.log.Error()
	}

	event.
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Dur("duration", dur).
		Time("start", start)

	if res != nil {
		event.Int("statusCode", res.StatusCode)
	}

	if a.RequestBodyEnabled() && req != nil && req.Body != nil && req.Body != http.NoBody {
		var buf bytes.Buffer
		if req.GetBody != nil {
			b, _ := req.GetBody()
			buf.ReadFrom(b) //nolint:errcheck,gosec
		} else {
			buf.ReadFrom(req.Body) //nolint:errcheck,gosec
		}
		event.Str("request", buf.String())
	}

	if a.ResponseBodyEnabled() && res != nil && res.Body != nil && res.Body != http.NoBody {
		defer res.Body.Close() //nolint:errcheck
		var buf bytes.Buffer
		buf.ReadFrom(res.Body) //nolint:errcheck,gosec
		event.Str("response", buf.String())
	}

	event.Msg("elasticsearch request")

	return nil
}

// RequestBodyEnabled returns false because we do not log request bodies.
func (a loggerAdapter) RequestBodyEnabled() bool {
	return false
}

// ResponseBodyEnabled returns false because we do not log response bodies.
func (a loggerAdapter) ResponseBodyEnabled() bool {
	return false
}

var _ elastictransport.Logger = (*loggerAdapter)(nil)

// GetClient creates and configures an Elasticsearch typed client with the specified HTTP client, logger, and URL.
func GetClient(httpClient *http.Client, logger zerolog.Logger, url string) (*elasticsearch.TypedClient, errors.E) {
	cfg := elasticsearch.Config{ //nolint:exhaustruct
		Addresses: []string{strings.TrimSpace(url)},
		Transport: httpClient.Transport,
		Logger:    &loggerAdapter{logger},
		// We do not enable discovery so that Docker setup is easier.
		// TODO: Should enabling discovery be a CLI parameter?
	}
	esClient, err := elasticsearch.NewTypedClient(cfg)
	return esClient, errors.WithStack(err)
}

// EnsureIndex makes sure the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of EnsureIndex would otherwise create.
// The shards parameter specifies the number of primary shards for the index.
func EnsureIndex(ctx context.Context, esClient *elasticsearch.TypedClient, index string, shards int) errors.E {
	exists, err := esClient.Indices.Exists(index).IsSuccess(ctx)
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

		config.Settings["number_of_shards"] = shards
		config.Settings["number_of_replicas"] = 0

		configJSON, errE := x.MarshalWithoutEscapeHTML(config)
		if errE != nil {
			return errE
		}

		createIndex, err := esClient.Indices.Create(index).Raw(bytes.NewReader(configJSON)).Do(ctx)
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
