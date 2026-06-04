package search

import (
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
// Bulk responses are not handled here: their per-item failures are *types.ErrorCause values
// aggregated into a single error rather than mapped one-to-one.
func WithESError(v any) errors.E {
	return internalSearch.WithESError(v)
}
