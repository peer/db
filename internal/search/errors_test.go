package search_test

import (
	"context"
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

		assert.Nil(t, internalSearch.WithESError(t.Context(), nil))
	})

	t.Run("typed nil ErrorResponseBase", func(t *testing.T) {
		t.Parallel()

		// A typed nil pointer is not caught by a plain v == nil check, so it must be guarded
		// against rather than dereferenced.
		var resp *types.ErrorResponseBase
		assert.Nil(t, internalSearch.WithESError(t.Context(), resp))
	})

	t.Run("ElasticsearchError", func(t *testing.T) {
		t.Parallel()

		esErr := types.NewElasticsearchError()
		esErr.Status = 500
		esErr.ErrorCause.Type = "search_phase_execution_exception"
		reason := "Partial shards failure"
		esErr.ErrorCause.Reason = &reason

		errE := internalSearch.WithESError(t.Context(), esErr)
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

		errE := internalSearch.WithESError(t.Context(), resp)
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
		errE := internalSearch.WithESError(t.Context(), orig)
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

		errE := internalSearch.WithESError(t.Context(), asErr)
		require.NotNil(t, errE)

		details := errors.Details(errE)
		assert.NotContains(t, details, "errorCause")
	})

	t.Run("unexpected type", func(t *testing.T) {
		t.Parallel()

		errE := internalSearch.WithESError(t.Context(), 42)
		require.Error(t, errE, "% -+#.1v", errE)

		assert.Equal(t, "int", errors.Details(errE)["type"])
	})

	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()

		// Elasticsearch reports a search cancelled with the caller as a failure of the shards it was
		// cancelled on. What the call was made with is what says it is that: the context which is done
		// becomes the cause of what Elasticsearch answered, so a request which was abandoned is
		// answered as one and not as a failure of the site, and what Elasticsearch said is kept.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		esErr := types.NewElasticsearchError()
		esErr.Status = 400
		esErr.ErrorCause.Type = "search_phase_execution_exception"

		errE := internalSearch.WithESError(ctx, esErr)
		require.Error(t, errE, "% -+#.1v", errE)
		require.ErrorIs(t, errE, context.Canceled)
		require.ErrorIs(t, errE, esErr)
	})

	t.Run("canceled context without an error", func(t *testing.T) {
		t.Parallel()

		// A call which answered while the context was already done was still abandoned, so the
		// cancellation is what is reported even though there is nothing to report of the call itself.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		errE := internalSearch.WithESError(ctx, nil)
		require.Error(t, errE, "% -+#.1v", errE)
		require.ErrorIs(t, errE, context.Canceled)
	})

	t.Run("context which is not done", func(t *testing.T) {
		t.Parallel()

		// Elasticsearch cancelling a task on its own leaves the context untouched, so what is returned
		// is the error it answered with.
		esErr := types.NewElasticsearchError()
		esErr.Status = 400
		esErr.ErrorCause.Type = "task_cancelled_exception"

		errE := internalSearch.WithESError(t.Context(), esErr)
		require.Error(t, errE, "% -+#.1v", errE)
		require.NotErrorIs(t, errE, context.Canceled)
		assert.Contains(t, errors.Details(errE), "errorCause")
	})
}
