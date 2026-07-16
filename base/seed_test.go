package base_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
)

// TestRequestedClaimsChanges verifies the request-derived seeding of create sessions: claims of
// scope-participating properties requested through the query string become the session's first
// changes, while other properties are rejected.
func TestRequestedClaimsChanges(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()
	scopeProperties := map[identifier.Identifier]bool{prop: true}
	session := identifier.New()
	docBase := []string{"test", identifier.New().String()}

	allowedReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/?"+prop.String()+"="+value.String(), strings.NewReader("{}"))
	changes, errE := base.RequestedClaimsChanges(allowedReq, scopeProperties, session, docBase, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, changes, 1)
	docID := identifier.New()
	doc := &document.D{CoreDocument: document.CoreDocument{ID: docID, Base: []string{"test", docID.String()}}}
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, doc.Get(prop), 1)

	otherProp := identifier.New()
	otherReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/?"+otherProp.String()+"="+value.String(), strings.NewReader("{}"))
	_, errE = base.RequestedClaimsChanges(otherReq, scopeProperties, session, docBase, 0)
	assert.EqualError(t, errE, "property does not participate in permission scopes")
}
