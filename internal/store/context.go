package store

import (
	"context"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// transactionContextKey contains the existing transaction, if any.
var transactionContextKey = &contextKey{"transaction"} //nolint:gochecknoglobals

// schemaContextKey is a fallback context key for a database context
// when it is not part of the request.
var schemaContextKey = &contextKey{"schema"} //nolint:gochecknoglobals

// requestIDContextKey is a fallback context key for a database context
// when it is not part of the request.
var requestIDContextKey = &contextKey{"request-id"} //nolint:gochecknoglobals

// maxDBPoolConnectionsContextKey allows overriding the maximum number of
// connections in the database pool.
var maxDBPoolConnectionsContextKey = &contextKey{"maxDBPoolConnections"} //nolint:gochecknoglobals

// TestMaxDBPoolConnections is the maximum number of connections to use
// in test database pools to prevent exhausting PostgreSQL connections
// when multiple tests run in parallel.
const TestMaxDBPoolConnections int32 = 10

// WithMaxDBPoolConnections returns context with an override for the maximum
// number of connections in the database pool.
func WithMaxDBPoolConnections(ctx context.Context, maxConns int32) context.Context {
	return context.WithValue(ctx, maxDBPoolConnectionsContextKey, maxConns)
}

// WithFallbackDBContext returns context with fallback context values which are used
// to set schema and application name on PostgreSQL connections when it is not part
// of the request.
func WithFallbackDBContext(ctx context.Context, schema, name string) context.Context {
	ctx = context.WithValue(ctx, schemaContextKey, schema)
	ctx = context.WithValue(ctx, requestIDContextKey, name)
	return ctx
}

// MustGetRequestID extracts the request ID from context, trying waf request context
// first and falling back to the value set by [WithFallbackDBContext].
//
// It panics if the request ID is not found.
func MustGetRequestID(ctx context.Context) string {
	var requestID string
	r, ok := waf.RequestID(ctx)
	if ok {
		requestID = r.String()
	}
	if requestID == "" {
		requestID, _ = ctx.Value(requestIDContextKey).(string)
	}
	if requestID == "" {
		errE := errors.New("request ID is missing in context")
		panic(errE)
	}
	return requestID
}

// GetRequestWithFallback returns a function which extracts schema and request ID
// from a context, trying waf request context and getSchema first and falling back to values
// set by [WithFallbackDBContext].
//
// It panics if any of the values are not available.
func GetRequestWithFallback(getSchema func(context.Context) string) func(context.Context) (string, string) {
	return func(ctx context.Context) (string, string) {
		requestID := MustGetRequestID(ctx)

		schema := getSchema(ctx)
		if schema == "" {
			schema, _ = ctx.Value(schemaContextKey).(string)
		}
		if schema == "" {
			errE := errors.New("schema is missing in context")
			panic(errE)
		}

		return schema, requestID
	}
}
