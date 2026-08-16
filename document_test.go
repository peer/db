package peerdb_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

// newTestDoc builds a minimal document with a valid ID derived from its Base.
func newTestDoc() *document.D {
	base := []string{"test", identifier.New().String()}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(base...), Base: base},
	}
}

// TestListReadableDocuments exercises the documents listing that DocumentListGetAPI serves: it returns the
// documents the caller may read, keyset-paginated by the last returned ID, and it excludes documents a read
// would not return (here, a deleted one) even though the store's raw listing still contains them.
func TestListReadableDocuments(t *testing.T) {
	t.Parallel()

	var globals *peerdb.Globals
	// The started site (with its base) lives on globals.Sites, which serve.Init populates in place.
	startTestServer(t, func(g *peerdb.Globals, _ *peerdb.ServeCommand) {
		globals = g
		g.Sites = []internalSite.Site{
			{
				Site: waf.Site{
					Domain:   "localhost",
					CertFile: "",
					KeyFile:  "",
				},
				Build:                nil,
				IndexPrefix:          "",
				Schema:               "",
				Title:                "",
				Logo:                 nil,
				Favicon:              internalSite.Favicon{},
				LanguagePriority:     nil,
				DefaultLanguage:      "",
				LanguageCodes:        nil,
				NamingProperties:     nil,
				Features:             internalSite.SiteFeatures{},
				Roles:                nil,
				ScopeProperties:      nil,
				Visibility:           nil,
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

	site := &globals.Sites[0]
	ctx := peerdb.WithFallbackDBContext(t.Context(), site.Schema, "test")

	// Insert two documents on top of the core documents startTestServer already inserted.
	doc1 := newTestDoc()
	doc2 := newTestDoc()
	for _, doc := range []*document.D{doc1, doc2} {
		errE := site.Base.InsertOrReplaceDocument(ctx, doc)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	// The read-path permission hooks are registered (init.go does it for every site) but inert here: they
	// need a site in ctx and this ctx carries only the schema, so nothing is denied. With nothing deleted
	// the readable listing is then exactly the store's committed documents, in the same (ID) order.
	storeIDs, errE := site.Base.Documents().List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	readable, errE := peerdb.TestingListReadableDocuments(ctx, site, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, storeIDs, readable)
	assert.Contains(t, readable, doc1.ID)
	assert.Contains(t, readable, doc2.ID)

	// Pagination: requesting "after" the first ID excludes it and returns the remaining documents.
	require.NotEmpty(t, readable)
	afterFirst, errE := peerdb.TestingListReadableDocuments(ctx, site, &readable[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, readable[1:], afterFirst)

	// A deleted document drops out of the readable listing, even though the store's raw listing keeps its ID.
	errE = site.Base.DeleteDocument(ctx, doc1.ID)
	require.NoError(t, errE, "% -+#.1v", errE)

	afterDelete, errE := peerdb.TestingListReadableDocuments(ctx, site, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotContains(t, afterDelete, doc1.ID)
	assert.Contains(t, afterDelete, doc2.ID)

	rawAfterDelete, errE := site.Base.Documents().List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, rawAfterDelete, doc1.ID)
}

// TestDocumentListGetAPIRequiresBulkRead verifies the gate DocumentListGetAPI puts in front of the
// listing: enumerating documents requires the bulk read action on documents, so the read action alone
// (which allows fetching documents one by one) is not enough, and neither is the bulk read action
// granted on files only.
func TestDocumentListGetAPIRequiresBulkRead(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		grants     map[string][]string
		wantStatus int
	}{
		{
			name:       "read alone does not allow enumerating",
			grants:     map[string][]string{auth.ActionReadCode: {auth.ScopeAll}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "bulk read on files does not allow enumerating documents",
			grants:     map[string][]string{auth.ActionReadCode: {auth.ScopeAll}, auth.ActionReadBulkCode: {auth.ScopeFiles}},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "bulk read on documents allows enumerating",
			grants:     map[string][]string{auth.ActionReadCode: {auth.ScopeAll}, auth.ActionReadBulkCode: {auth.ScopeDocuments}},
			wantStatus: http.StatusOK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts, service := startTestServer(t, func(g *peerdb.Globals, _ *peerdb.ServeCommand) {
				g.Sites = []internalSite.Site{
					{
						Site: waf.Site{
							Domain:   "localhost",
							CertFile: "",
							KeyFile:  "",
						},
						Build:                nil,
						IndexPrefix:          "",
						Schema:               "",
						Title:                "Example Site",
						Logo:                 nil,
						Favicon:              internalSite.Favicon{},
						LanguagePriority:     nil,
						DefaultLanguage:      "",
						LanguageCodes:        nil,
						NamingProperties:     nil,
						Features:             internalSite.SiteFeatures{},
						Roles:                map[string]auth.RoleGrants{auth.RoleEveryone: auth.MustParseRoleGrants(tt.grants)},
						ScopeProperties:      nil,
						Visibility:           nil,
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

			apiPath, errE := service.ReverseAPI("DocumentList", nil, nil)
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
