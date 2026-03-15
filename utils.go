package peerdb

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/waf"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// schemaContextKey is a fallback context key for a database context
// when it is not part of the request.
var schemaContextKey = &contextKey{"schema"} //nolint:gochecknoglobals

// requestIDContextKey is a fallback context key for a database context
// when it is not part of the request.
var requestIDContextKey = &contextKey{"request-id"} //nolint:gochecknoglobals

func hasConnectionUpgrade(req *http.Request) bool {
	for _, value := range strings.Split(req.Header.Get("Connection"), ",") {
		if strings.ToLower(strings.TrimSpace(value)) == "upgrade" {
			return true
		}
	}
	return false
}

// Same as in zerolog/hlog/hlog.go.
func getHost(hostPort string) string {
	if hostPort == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort
	}
	return host
}

func getRequestWithFallback(logger zerolog.Logger) func(context.Context) (string, string) {
	return func(ctx context.Context) (string, string) {
		var requestID string
		r, ok := waf.RequestID(ctx)
		if ok {
			requestID = r.String()
		} else {
			// Fallback for non-request contexts.
			requestID, ok = ctx.Value(requestIDContextKey).(string)
			if !ok {
				logger.Error().Msg("request ID is missing in context")
			}
		}

		var schema string
		site, ok := waf.GetSite[*Site](ctx)
		if ok {
			schema = site.Schema
		} else {
			// Fallback for non-request contexts.
			schema, ok = ctx.Value(schemaContextKey).(string)
			if !ok {
				logger.Error().Msg("schema is missing in context")
			}
		}

		return schema, requestID
	}
}
