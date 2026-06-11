package search_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/internal/testutils"
)

func initESClient(t *testing.T) (context.Context, *elasticsearch.TypedClient) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, esClient
}

func TestEnsureIndexAliasLayout(t *testing.T) {
	t.Parallel()

	ctx, esClient := initESClient(t)

	name := "s" + strings.ToLower(identifier.New().String()) + "_all"

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		errE := internalSearch.DeleteIndex(context.Background(), esClient, name)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	errE := internalSearch.EnsureIndex(ctx, esClient, name, 1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The name is an alias to exactly one timestamped index which is the write index.
	res, err := esClient.Indices.GetAlias().Name(name).Do(ctx)
	testutils.RequireNoESError(t, err)
	require.Len(t, res, 1)
	for index, aliases := range res {
		assert.True(t, strings.HasPrefix(index, name+"_"), index)
		require.Contains(t, aliases.Aliases, name)
		isWriteIndex := aliases.Aliases[name].IsWriteIndex
		require.NotNil(t, isWriteIndex)
		assert.True(t, *isWriteIndex)
	}

	// EnsureIndex leaves an existing alias layout unchanged.
	errE = internalSearch.EnsureIndex(ctx, esClient, name, 1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	resAgain, err := esClient.Indices.GetAlias().Name(name).Do(ctx)
	testutils.RequireNoESError(t, err)
	assert.Equal(t, res, resAgain)

	// DeleteIndex removes the alias together with its concrete index.
	errE = internalSearch.DeleteIndex(ctx, esClient, name)
	require.NoError(t, errE, "% -+#.1v", errE)
	exists, err := esClient.Indices.Exists(name).IsSuccess(ctx)
	testutils.RequireNoESError(t, err)
	assert.False(t, exists)

	// DeleteIndex of a name which does not exist is not an error.
	errE = internalSearch.DeleteIndex(ctx, esClient, name)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestDeleteIndexConcreteIndex(t *testing.T) {
	t.Parallel()

	ctx, esClient := initESClient(t)

	// A concrete index under the name itself is the layout from before the alias layout.
	name := "s" + strings.ToLower(identifier.New().String()) + "_all"
	_, err := esClient.Indices.Create(name).Do(ctx)
	testutils.RequireNoESError(t, err)

	errE := internalSearch.DeleteIndex(ctx, esClient, name)
	require.NoError(t, errE, "% -+#.1v", errE)

	exists, err := esClient.Indices.Exists(name).IsSuccess(ctx)
	testutils.RequireNoESError(t, err)
	assert.False(t, exists)
}
