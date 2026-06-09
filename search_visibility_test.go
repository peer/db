package peerdb_test

import (
	"net/http"
	"testing"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/auth"
)

// TestSearchVisibilityReadRouting drives the read-path routing through a real search handler. An anonymous
// caller resolves to no visibility level when the site's lowest level is role-gated (no floor), so
// resolveReadIndex denies the read and the endpoint returns 403 Forbidden. When the site's lowest level is a
// no-roles floor, the same anonymous caller resolves to it and reads its index, so the endpoint returns 200.
func TestSearchVisibilityReadRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		visibility []auth.VisibilityLevel
		wantStatus int
	}{
		{
			// No floor: the lowest level is role-gated, so an anonymous caller matches no level. The top
			// no-roles "all" level is the unfiltered superset (no role resolves to it).
			name: "no floor denies anonymous",
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: []string{"researcher"}},
				{Name: "all", Roles: nil},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			// Floor: the lowest level has no roles, so it is granted to every request including an anonymous one.
			name: "floor allows anonymous",
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: nil},
				{Name: "editor", Roles: []string{"researcher"}},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts, service := startTestServer(t, func(globals *peerdb.Globals, _ *peerdb.ServeCommand) {
				globals.Sites = []internalSite.Site{
					{
						Site: waf.Site{
							Domain:   "localhost",
							CertFile: "",
							KeyFile:  "",
						},
						Build:                nil,
						Index:                "",
						Schema:               "",
						Title:                "Example Site",
						Logo:                 "",
						LogoCompact:          "",
						LanguagePriority:     nil,
						DefaultLanguage:      "",
						LanguageCodes:        nil,
						Features:             internalSite.SiteFeatures{},
						Roles:                map[string][]string{"researcher": nil},
						Visibility:           tt.visibility,
						Auth:                 internalSite.SiteAuthConfig{},
						MetadataHeaderPrefix: "",
						Base:                 nil,
						DBPool:               nil,
						ESClient:             nil,
						RiverClient:          nil,
						Authenticator:        nil,
						DebugRiverHandler:    nil,
					},
				}
			})

			apiPath, errE := service.ReverseAPI("SearchJustResults", nil, nil)
			require.NoError(t, errE, "% -+#.1v", errE)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+apiPath, nil)
			require.NoError(t, err)
			req.Host = "localhost"

			resp, err := ts.Client().Do(req) //nolint:bodyclose
			require.NoError(t, err)
			t.Cleanup(func(r *http.Response) func() { return func() { r.Body.Close() } }(resp)) //nolint:errcheck,gosec

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}
