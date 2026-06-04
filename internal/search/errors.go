package search

import (
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
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
	if v == nil {
		return nil
	}

	switch e := v.(type) {
	case *types.ErrorResponseBase:
		// A typed nil pointer is not caught by the v == nil check above, so guard against it
		// here to avoid dereferencing it below.
		if e == nil {
			return nil
		}
		errE := errors.New("elasticsearch error")
		details := errors.Details(errE)
		details["errorCause"] = e.Error
		details["status"] = e.Status
		return errE
	case error:
		errE := errors.WithStack(e)
		esErr, ok := errors.AsType[*types.ElasticsearchError](errE)
		if ok && esErr != nil {
			details := errors.Details(errE)
			details["errorCause"] = esErr.ErrorCause
			details["status"] = esErr.Status
		}
		return errE
	default:
		errE := errors.New("unexpected value passed to WithESError")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", v)
		return errE
	}
}
