package search_test

import (
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

func TestWithESError(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, internalSearch.WithESError(nil))
	})

	t.Run("typed nil ErrorResponseBase", func(t *testing.T) {
		t.Parallel()

		// A typed nil pointer is not caught by a plain v == nil check, so it must be guarded
		// against rather than dereferenced.
		var resp *types.ErrorResponseBase
		assert.Nil(t, internalSearch.WithESError(resp))
	})

	t.Run("ElasticsearchError", func(t *testing.T) {
		t.Parallel()

		esErr := types.NewElasticsearchError()
		esErr.Status = 500
		esErr.ErrorCause.Type = "search_phase_execution_exception"
		reason := "Partial shards failure"
		esErr.ErrorCause.Reason = &reason

		errE := internalSearch.WithESError(esErr)
		require.Error(t, errE, "% -+#.1v", errE)

		details := errors.Details(errE)
		assert.Equal(t, 500, details["status"])
		cause, ok := details["errorCause"].(types.ErrorCause)
		require.True(t, ok)
		assert.Equal(t, "search_phase_execution_exception", cause.Type)
	})

	t.Run("ErrorResponseBase", func(t *testing.T) {
		t.Parallel()

		resp := types.NewErrorResponseBase()
		resp.Status = 503
		resp.Error.Type = "query_shard_exception"

		errE := internalSearch.WithESError(resp)
		require.Error(t, errE, "% -+#.1v", errE)

		details := errors.Details(errE)
		assert.Equal(t, 503, details["status"])
		cause, ok := details["errorCause"].(types.ErrorCause)
		require.True(t, ok)
		assert.Equal(t, "query_shard_exception", cause.Type)
	})

	t.Run("plain error", func(t *testing.T) {
		t.Parallel()

		// A non-Elasticsearch error is wrapped with a stack but gets no extra details.
		orig := errors.New("boom")
		errE := internalSearch.WithESError(orig)
		require.Error(t, errE, "% -+#.1v", errE)
		assert.ErrorIs(t, errE, orig)

		details := errors.Details(errE)
		assert.NotContains(t, details, "errorCause")
		assert.NotContains(t, details, "status")
	})

	t.Run("typed nil ElasticsearchError", func(t *testing.T) {
		t.Parallel()

		// A non-nil error interface wrapping a typed nil *types.ElasticsearchError must not panic
		// nor attach an errorCause: this exercises the esErr != nil guard.
		var nilES *types.ElasticsearchError
		var asErr error = nilES

		errE := internalSearch.WithESError(asErr)
		require.NotNil(t, errE)

		details := errors.Details(errE)
		assert.NotContains(t, details, "errorCause")
	})

	t.Run("unexpected type", func(t *testing.T) {
		t.Parallel()

		errE := internalSearch.WithESError(42)
		require.Error(t, errE, "% -+#.1v", errE)

		assert.Equal(t, "int", errors.Details(errE)["type"])
	})
}
