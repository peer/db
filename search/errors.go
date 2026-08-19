package search

import (
	"context"

	"gitlab.com/tozd/go/errors"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

var (
	ErrNotFound         = errors.Base("not found")
	ErrValidationFailed = errors.Base("validation failed")
)

// WithESError wraps an Elasticsearch error with its error cause and HTTP status extracted into
// error details. It accepts either an error returned by a typed Elasticsearch API ".Do" call
// (which is a *types.ElasticsearchError) or a *types.ErrorResponseBase response item from a
// multi-search response (which carries the same cause and status but is not itself a Go error).
// Any other non-nil error is wrapped with a stack trace, without extra details. It returns nil
// if v is nil.
//
// A call made with a context which is already done returns an error caused by the context's own, so a
// request which was abandoned while Elasticsearch was answering it is answered as a timeout and not
// as a failure of the site.
//
// Bulk responses are not handled here: their per-item failures are *types.ErrorCause values
// aggregated into a single error rather than mapped one-to-one.
func WithESError(ctx context.Context, v any) errors.E {
	return internalSearch.WithESError(ctx, v)
}
