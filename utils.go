package peerdb

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
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

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
func InsertOrReplaceDocument(
	ctx context.Context,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	doc *document.D,
) errors.E {
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}
	_, errE = store.Insert(ctx, doc.ID, data, &types.DocumentMetadata{At: types.Time(time.Now().UTC())}, &types.NoMetadata{})
	return errE
}

// UpdateDocument updates the document in the index, if it has not changed in the database since it was fetched (based on its current version).
func UpdateDocument(
	ctx context.Context,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	doc *document.D, version store.Version,
) errors.E {
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}

	// Store does not allow multiple latest versions so if document has been updated in meantime it cannot be updated again and the call will fail.
	// TODO: Set patch. Or update revision?
	//       Especially if this is done while preparing for a commit a changeset of multiple changes? But then we should not be calling store.Update but changeset.Update.
	_, errE = store.Update(ctx, doc.ID, version.Changeset, data, document.Changes{}, &types.DocumentMetadata{At: types.Time(time.Now().UTC())}, &types.NoMetadata{})
	return errE
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
