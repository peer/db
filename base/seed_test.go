package base_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
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

// TestRequestedPermissionClaimsChanges verifies that an access request for several actions records
// one claim per action, each naming the requesting user, scoped to the document itself, and carrying
// the note.
func TestRequestedPermissionClaimsChanges(t *testing.T) {
	t.Parallel()

	session := identifier.New()
	docID := identifier.New()
	docBase := []string{"test", docID.String()}
	actions := []identifier.Identifier{auth.ActionRead, auth.ActionUpdate}

	changes := base.RequestedPermissionClaimsChanges("user1", actions, "why not", session, docBase, 0)
	doc := &document.D{CoreDocument: document.CoreDocument{ID: docID, Base: docBase}}
	errE := changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	claims := document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, internalCore.HasRequestedPermissionPropID, document.LowConfidence)
	require.Len(t, claims, len(actions))
	requested := make([]identifier.Identifier, 0, len(claims))
	for _, claim := range claims {
		requested = append(requested, claim.To.ID)
		users := document.GetClaimsOfTypeWithConfidence[document.IdentifierClaim](claim, internalCore.PermissionUserPropID, document.LowConfidence)
		require.Len(t, users, 1)
		assert.Equal(t, "user1", users[0].Value)
		scopes := document.GetClaimsOfTypeWithConfidence[document.StringClaim](claim, internalCore.PermissionScopePropID, document.LowConfidence)
		require.Len(t, scopes, 1)
		assert.Equal(t, auth.ScopeSelf, scopes[0].String)
		notes := document.GetClaimsOfTypeWithConfidence[document.StringClaim](claim, internalCore.DescriptionPropID, document.LowConfidence)
		require.Len(t, notes, 1)
		assert.Equal(t, "why not", notes[0].String)
	}
	assert.ElementsMatch(t, actions, requested)

	// Without a note there is no claim describing the request.
	changes = base.RequestedPermissionClaimsChanges("user1", []identifier.Identifier{auth.ActionRead}, "", session, docBase, 0)
	doc = &document.D{CoreDocument: document.CoreDocument{ID: docID, Base: docBase}}
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	claims = document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, internalCore.HasRequestedPermissionPropID, document.LowConfidence)
	require.Len(t, claims, 1)
	assert.Empty(t, document.GetClaimsOfTypeWithConfidence[document.StringClaim](claims[0], internalCore.DescriptionPropID, document.LowConfidence))
}
