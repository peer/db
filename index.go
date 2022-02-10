package search

import (
	"context"
	_ "embed"
	"net/http"

	"github.com/olivere/elastic/v7"

	"gitlab.com/tozd/go/errors"
)

// TODO: Generate index configuration automatically from document structs?

//go:embed index.json
var indexConfiguration string

// EnsureIndex creates an instance of the ElasticSearch client and makes sure
// the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of EnsureIndex would otherwise create.
func EnsureIndex(ctx context.Context, httpClient *http.Client) (*elastic.Client, errors.E) {
	esClient, err := elastic.NewClient(
		elastic.SetHttpClient(httpClient),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	exists, err := esClient.IndexExists("docs").Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !exists {
		createIndex, err := esClient.CreateIndex("docs").BodyString(indexConfiguration).Do(ctx)
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
