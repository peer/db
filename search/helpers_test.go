package search_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// initES creates and configures an ES client and a test index.
// It returns the client, a search service factory, and the index name.
func initES(t *testing.T) (*elastic.Client, func() (*elastic.SearchService, int64, int64), string) {
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
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		assert.NoError(t, err)
	})

	errE = internalSearch.EnsureIndex(ctx, esClient, index)
	require.NoError(t, errE, "% -+#.1v", errE)

	getSearchService := func() (*elastic.SearchService, int64, int64) {
		return esClient.Search().Index(index), 100, 10
	}

	return esClient, getSearchService, index
}

// indexDocument indexes a document into ES using the internal search.Document struct.
func indexDocument(t *testing.T, ctx context.Context, esClient *elastic.Client, index string, doc internalSearch.Document) { //nolint:revive
	t.Helper()

	_, err := esClient.Index().Index(index).Id(doc.ID.String()).BodyJson(doc).Do(ctx)
	require.NoError(t, err)
}

// refreshIndex forces an ES index refresh so documents are searchable.
func refreshIndex(t *testing.T, ctx context.Context, esClient *elastic.Client, index string) { //nolint:revive
	t.Helper()

	_, err := esClient.Refresh(index).Do(ctx)
	require.NoError(t, err)
}

// createSession is a test helper that creates a search session.
func createSession(t *testing.T, ctx context.Context, s *search.Session) { //nolint:revive
	t.Helper()

	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)
}
