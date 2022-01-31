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

func EnsureIndex(ctx context.Context, httpClient *http.Client) (*elastic.Client, errors.E) {
	client, err := elastic.NewClient(
		elastic.SetHttpClient(httpClient),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	exists, err := client.IndexExists("docs").Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !exists {
		createIndex, err := client.CreateIndex("docs").BodyString(indexConfiguration).Do(ctx) //nolint:govet
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !createIndex.Acknowledged {
			return nil, errors.New("create index not acknowledged")
		}
	}

	return client, nil
}
