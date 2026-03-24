package search_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	essearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// initES creates and configures an ES client and a test index.
// It returns the client, a search service factory, and the index name.
func initES(t *testing.T) (*elasticsearch.TypedClient, func() (*essearch.Search, int64, int64), string) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	index := "s" + strings.ToLower(identifier.New().String())

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(context.Background())
		assert.NoError(t, err)
	})

	errE = internalSearch.EnsureIndex(ctx, esClient, index)
	require.NoError(t, errE, "% -+#.1v", errE)

	getSearchService := func() (*essearch.Search, int64, int64) {
		return esClient.Search().Index(index), 100, 10
	}

	return esClient, getSearchService, index
}

// indexDocument indexes a document into ES using the internal search.Document struct.
func indexDocument(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string, doc internalSearch.Document) { //nolint:revive
	t.Helper()

	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, err := esClient.Index(index).Id(doc.ID.String()).Raw(bytes.NewReader(data)).Do(ctx)
	require.NoError(t, err)
}

// refreshIndex forces an ES index refresh so documents are searchable.
func refreshIndex(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string) { //nolint:revive
	t.Helper()

	_, err := esClient.Indices.Refresh().Index(index).Do(ctx)
	require.NoError(t, err)
}

// createSession is a test helper that creates a search session.
func createSession(t *testing.T, ctx context.Context, s *search.Session) { //nolint:revive
	t.Helper()

	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)
}
