package es

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/indexer"
	"gitlab.com/peerdb/peerdb/internal/mapping"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
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
		indexConfiguration, errE := mapping.Generate()
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

// CompleteDocumentSession completes the document editing session.
func CompleteDocumentSession(
	ctx context.Context, s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	c *coordinator.Coordinator[json.RawMessage, *types.DocumentChangeMetadata, *types.DocumentBeginMetadata, *types.DocumentEndMetadata, *types.DocumentCompleteMetadata],
	session identifier.Identifier,
) (*types.DocumentCompleteMetadata, errors.E) {
	beginMetadata, endMetadata, _, errE := c.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	if endMetadata.Discarded {
		return &types.DocumentCompleteMetadata{
			Changeset: nil,
			Time:      time.Since(time.Time(endMetadata.At)).Milliseconds(),
		}, nil
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
		data, _, errE := c.GetData(ctx, session, ch)
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

	return &types.DocumentCompleteMetadata{
		Changeset: &version.Changeset,
		Time:      time.Since(time.Time(endMetadata.At)).Milliseconds(),
	}, nil
}

// NewHTTPClient creates a retryable HTTP client with the specified base HTTP client and logger.
func NewHTTPClient(logger zerolog.Logger, httpClient *http.Client) *http.Client {
	// TODO: Make contact e-mail into a CLI argument.
	return indexer.NewHTTPClient(logger, httpClient, fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision)) //nolint:lll
}
