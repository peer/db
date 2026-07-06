package peerdb

import (
	"context"
	"net"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// WithFallbackDBContext returns context with fallback context values which are used
// to set schema and application name on PostgreSQL connections when it is not part
// of the request.
func WithFallbackDBContext(ctx context.Context, schema, name string) context.Context {
	return internalStore.WithFallbackDBContext(ctx, schema, name)
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

func getRequestWithFallback() func(context.Context) (string, string) {
	return internalStore.GetRequestWithFallback(func(ctx context.Context) string {
		site, ok := waf.GetSite[*internalSite.Site](ctx)
		if ok {
			return site.Schema
		}
		return ""
	})
}
