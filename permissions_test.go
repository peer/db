package peerdb_test

import (
	"context"
	"io"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/store"
)

// refClaim returns a top-level reference claim.
func refClaim(id, prop, to identifier.Identifier) *document.ReferenceClaim {
	return &document.ReferenceClaim{
		CoreClaim: document.CoreClaim{ID: id, Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: prop},
		To:        document.Reference{ID: to},
	}
}

// permissionClaim returns a HAS_PERMISSION claim granting the user the action with the self scope.
func permissionClaim(t *testing.T, id identifier.Identifier, user string, action identifier.Identifier) *document.ReferenceClaim {
	t.Helper()
	claim := refClaim(id, internalCore.HasPermissionPropID, action)
	errE := claim.Add(&document.IdentifierClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: internalCore.PermissionUserPropID},
		Value:     user,
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = claim.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: internalCore.PermissionScopePropID},
		String:    auth.ScopeSelf,
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return claim
}

// docWithClaims returns a document with the given ID and top-level claims.
func docWithClaims(t *testing.T, id identifier.Identifier, claims ...document.Claim) *document.D {
	t.Helper()
	doc := &document.D{CoreDocument: document.CoreDocument{ID: id, Base: []string{"test", id.String()}}}
	for _, claim := range claims {
		errE := doc.Add(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	return doc
}

// rolesSite returns a site with the given roles.
func rolesSite(roles map[string]auth.RoleGrants) *internalSite.Site {
	site := &internalSite.Site{}
	site.Roles = roles
	return site
}

// testReadSeekCloser records whether it was closed.
type testReadSeekCloser struct {
	closed bool
}

func (f *testReadSeekCloser) Read(_ []byte) (int, error) { return 0, io.EOF }

func (f *testReadSeekCloser) Seek(_ int64, _ int) (int64, error) { return 0, nil }

func (f *testReadSeekCloser) Close() error {
	f.closed = true
	return nil
}

// TestCheckChangePermission verifies the per-change gate of edit sessions: every kind of top-level
// claim a change touches has its requirement checked, so a change moving a claim between properties
// (setting or casting its property) cannot smuggle a modification of claims of one kind past a
// permission covering only another kind.
func TestCheckChangePermission(t *testing.T) {
	t.Parallel()

	class1 := identifier.New()
	class2 := identifier.New()
	otherProp := identifier.New()

	updateGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionUpdateCode: {auth.ScopeDocuments},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	site := &internalSite.Site{}
	site.Roles = map[string]auth.RoleGrants{"editor": updateGrants}
	site.ScopeProperties = map[identifier.Identifier]bool{internalCore.InstanceOfPropID: true}

	// Documents share claim objects, so a claim present in a before and an after document serializes
	// identically and only genuinely differing claims count as changed.
	docID := identifier.New()
	makeDoc := func(claims ...document.Claim) *document.D {
		return docWithClaims(t, docID, claims...)
	}

	instanceClaim := refClaim(identifier.New(), internalCore.InstanceOfPropID, class1)
	collabUpdate := permissionClaim(t, identifier.New(), "collab", auth.ActionUpdate)
	collabPermissions := permissionClaim(t, identifier.New(), "collab", auth.ActionUpdatePermissions)
	permonlyPermissions := permissionClaim(t, identifier.New(), "permonly", auth.ActionUpdatePermissions)
	baseClaims := []document.Claim{instanceClaim, collabUpdate, collabPermissions, permonlyPermissions}
	baseDoc := makeDoc(baseClaims...)

	// collab holds update and permissions document claims, permonly holds only a permissions claim,
	// editor holds the update action on all documents through a role, and nobody holds nothing.
	ctxCollab := auth.WithSubject(context.Background(), "collab")
	ctxPermonly := auth.WithSubject(context.Background(), "permonly")
	ctxEditor := auth.WithRoles(auth.WithSubject(context.Background(), "editor"), []string{"editor"})
	ctxNobody := auth.WithSubject(context.Background(), "nobody")

	// A change to claims of an ordinary property requires the update action, which document claims
	// can grant.
	ordinaryAdded := makeDoc(append(slices.Clone(baseClaims), refClaim(identifier.New(), otherProp, class2))...)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, ordinaryAdded)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, baseDoc, ordinaryAdded)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, ordinaryAdded)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxNobody, site, baseDoc, ordinaryAdded)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// A change to permission claims requires the permissions action alone (no update action), and
	// granting is limited to actions the granter holds: permonly holds (only) the permissions action
	// and can grant it onwards, while an editor without the permissions action cannot touch
	// permission claims at all.
	permissionAdded := makeDoc(append(slices.Clone(baseClaims), permissionClaim(t, identifier.New(), "other", auth.ActionUpdatePermissions))...)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, permissionAdded)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, permissionAdded)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, baseDoc, permissionAdded)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// Granting an action the granter does not hold is rejected even for holders of the permissions
	// action: neither collab nor permonly hold the historic read action.
	historicGranted := makeDoc(append(slices.Clone(baseClaims), permissionClaim(t, identifier.New(), "other", auth.ActionReadHistoric))...)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, historicGranted)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, historicGranted)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// Modifying an existing permission claim (here its user) is granting, too: it requires the
	// action the modified claim grants, which collab holds and permonly does not.
	modifiedGrant := makeDoc(instanceClaim, permissionClaim(t, collabUpdate.ID, "other", auth.ActionUpdate), collabPermissions, permonlyPermissions)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, modifiedGrant)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, modifiedGrant)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// Removing a permission claim is not granting: it requires only the permissions action, so
	// permonly can remove the update grant of collab without holding the update action.
	removedGrant := makeDoc(instanceClaim, collabPermissions, permonlyPermissions)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, removedGrant)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A change to claims of a scope-participating property requires the update action from role
	// grants alone: document claims must not allow moving the document across scopes.
	classChanged := makeDoc(refClaim(instanceClaim.ID, internalCore.InstanceOfPropID, class2), collabUpdate, collabPermissions, permonlyPermissions)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, baseDoc, classChanged)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, classChanged)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// A change setting a permission claim's property to a scope-participating property modifies
	// claims of both kinds, so it requires both the permissions action and the role-level update
	// action: holding the permissions action alone (or with only a document-claim update) does not
	// let claims of scope-participating properties in.
	flipID := identifier.New()
	flipBefore := makeDoc(append(slices.Clone(baseClaims), permissionClaim(t, flipID, "collab", auth.ActionReadHistoric))...)
	flipAfter := makeDoc(append(slices.Clone(baseClaims), refClaim(flipID, internalCore.InstanceOfPropID, class2))...)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, flipBefore, flipAfter)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	ctxCollabEditor := auth.WithRoles(auth.WithSubject(context.Background(), "collab"), []string{"editor"})
	errE = peerdb.TestingCheckChangePermission(ctxCollabEditor, site, flipBefore, flipAfter)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A change setting a permission claim's property to an ordinary property modifies claims of both
	// kinds, so it requires both the permissions action and the update action.
	flipOtherID := identifier.New()
	flipOtherBefore := makeDoc(append(slices.Clone(baseClaims), permissionClaim(t, flipOtherID, "permonly", auth.ActionReadHistoric))...)
	flipOtherAfter := makeDoc(append(slices.Clone(baseClaims), refClaim(flipOtherID, otherProp, class2))...)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, flipOtherBefore, flipOtherAfter)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, flipOtherBefore, flipOtherAfter)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Casting a claim to another type with equal fields (a has claim and a none claim serialize the
	// same) is still a change to a claim of its property: the claim type is part of the comparison.
	castID := identifier.New()
	castBefore := makeDoc(append(slices.Clone(baseClaims), &document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: castID, Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
	})...)
	castAfter := makeDoc(append(slices.Clone(baseClaims), &document.HasClaim{
		CoreClaim: document.CoreClaim{ID: castID, Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
	})...)
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, castBefore, castAfter)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, castBefore, castAfter)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A change modifying no top-level claims at all still requires the update action.
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, makeDoc(baseClaims...))
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxNobody, site, baseDoc, makeDoc(baseClaims...))
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// A site without document-level permissions accepts no permission claim at all, not even from a
	// caller who holds the permissions action through the claims already on the document. Its other
	// claims are unaffected, and so are the role grants, whose scopes are about ordinary properties.
	site.Features.DisableDocumentPermissions = true
	errE = peerdb.TestingCheckChangePermission(ctxCollab, site, baseDoc, permissionAdded)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxPermonly, site, baseDoc, removedGrant)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, baseDoc, ordinaryAdded)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.TestingCheckChangePermission(ctxEditor, site, baseDoc, classChanged)
	require.NoError(t, errE, "% -+#.1v", errE)
	site.Features.DisableDocumentPermissions = false
}

// TestCheckChangePermissionGrantsHeld verifies the no-amplification rule of permission claim changes
// across the two ways actions are held: an admin whose role grants actions on all documents can
// grant them document-level to another user, and a creator holding the read action through a role
// and the update (and permissions) actions through the document's own claims can grant those, while
// granting an action the granter does not hold in either way is rejected.
func TestCheckChangePermissionGrantsHeld(t *testing.T) {
	t.Parallel()

	adminGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode:              {auth.ScopeAll},
		auth.ActionUpdateCode:            {auth.ScopeAll},
		auth.ActionDeleteCode:            {auth.ScopeAll},
		auth.ActionUpdatePermissionsCode: {auth.ScopeAll},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	readerGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode: {auth.ScopeDocuments},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	site := rolesSite(map[string]auth.RoleGrants{"admin": adminGrants, "reader": readerGrants})

	// The creator holds the update and permissions actions through the document's own claims (the
	// shape the create-session seeding produces) and the read action through the reader role.
	docID := identifier.New()
	creatorUpdate := permissionClaim(t, identifier.New(), "creator", auth.ActionUpdate)
	creatorPermissions := permissionClaim(t, identifier.New(), "creator", auth.ActionUpdatePermissions)
	baseDoc := docWithClaims(t, docID, creatorUpdate, creatorPermissions)

	ctxAdmin := auth.WithRoles(auth.WithSubject(context.Background(), "boss"), []string{"admin"})
	ctxCreator := auth.WithRoles(auth.WithSubject(context.Background(), "creator"), []string{"reader"})

	// An admin holding actions on all documents through a role can grant them document-level to
	// another user.
	adminShared := docWithClaims(t, docID, creatorUpdate, creatorPermissions,
		permissionClaim(t, identifier.New(), "other", auth.ActionDelete),
		permissionClaim(t, identifier.New(), "other", auth.ActionUpdatePermissions))
	errE = peerdb.TestingCheckChangePermission(ctxAdmin, site, baseDoc, adminShared)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The creator can grant the read action (held through the role) and the update action (held
	// through the document's claims) to another user.
	creatorShared := docWithClaims(t, docID, creatorUpdate, creatorPermissions,
		permissionClaim(t, identifier.New(), "other", auth.ActionRead),
		permissionClaim(t, identifier.New(), "other", auth.ActionUpdate))
	errE = peerdb.TestingCheckChangePermission(ctxCreator, site, baseDoc, creatorShared)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The creator does not hold the delete action in either way, so granting it is rejected.
	creatorAmplified := docWithClaims(t, docID, creatorUpdate, creatorPermissions,
		permissionClaim(t, identifier.New(), "other", auth.ActionDelete))
	errE = peerdb.TestingCheckChangePermission(ctxCreator, site, baseDoc, creatorAmplified)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}

// TestCheckDocumentPermission verifies the document permission check for the caller in ctx: grants of
// the reserved everyone role apply to every caller, grants of other roles apply through the roles
// bound to ctx, and with a document its own permission claims count for the subject bound to ctx.
func TestCheckDocumentPermission(t *testing.T) {
	t.Parallel()

	site := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
		"editor":          auth.MustParseRoleGrants(map[string][]string{auth.ActionUpdateCode: {auth.ScopeDocuments}}),
	})
	doc := docWithClaims(t, identifier.New(), permissionClaim(t, identifier.New(), "collab", auth.ActionUpdate))

	// The everyone role applies to every caller.
	assert.True(t, peerdb.TestingCheckDocumentPermission(context.Background(), site, auth.ActionRead, nil))
	assert.True(t, peerdb.TestingCheckDocumentPermission(context.Background(), site, auth.ActionRead, doc))
	assert.False(t, peerdb.TestingCheckDocumentPermission(context.Background(), site, auth.ActionUpdate, nil))

	// Other roles apply only when bound to ctx, and only when the site configures them.
	ctxEditor := auth.WithRoles(context.Background(), []string{"editor"})
	assert.True(t, peerdb.TestingCheckDocumentPermission(ctxEditor, site, auth.ActionUpdate, nil))
	assert.True(t, peerdb.TestingCheckDocumentPermission(ctxEditor, site, auth.ActionUpdate, doc))
	assert.False(t, peerdb.TestingCheckDocumentPermission(auth.WithRoles(context.Background(), []string{"unknown"}), site, auth.ActionUpdate, nil))

	// The document's own permission claims count for the subject bound to ctx, but only with the
	// document present.
	ctxCollab := auth.WithSubject(context.Background(), "collab")
	assert.True(t, peerdb.TestingCheckDocumentPermission(ctxCollab, site, auth.ActionUpdate, doc))
	assert.False(t, peerdb.TestingCheckDocumentPermission(ctxCollab, site, auth.ActionUpdate, nil))
	assert.False(t, peerdb.TestingCheckDocumentPermission(auth.WithSubject(context.Background(), "other"), site, auth.ActionUpdate, doc))
}

// TestCheckFilePermission verifies the file permission check for the caller in ctx: only role grants
// with a scope covering files count.
func TestCheckFilePermission(t *testing.T) {
	t.Parallel()

	site := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
		"uploader":        auth.MustParseRoleGrants(map[string][]string{auth.ActionCreateCode: {auth.ScopeFiles}}),
		"editor":          auth.MustParseRoleGrants(map[string][]string{auth.ActionUpdateCode: {auth.ScopeDocuments}}),
	})

	// The all and files scopes cover files, the documents scope does not.
	assert.True(t, peerdb.TestingCheckFilePermission(context.Background(), site, auth.ActionRead))
	assert.True(t, peerdb.TestingCheckFilePermission(auth.WithRoles(context.Background(), []string{"uploader"}), site, auth.ActionCreate))
	assert.False(t, peerdb.TestingCheckFilePermission(context.Background(), site, auth.ActionCreate))
	assert.False(t, peerdb.TestingCheckFilePermission(auth.WithRoles(context.Background(), []string{"editor"}), site, auth.ActionUpdate))
}

// TestCheckRoleDocumentPermission verifies the role-only document permission check: the document's
// own permission claims never count.
func TestCheckRoleDocumentPermission(t *testing.T) {
	t.Parallel()

	site := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
		"editor":          auth.MustParseRoleGrants(map[string][]string{auth.ActionUpdateCode: {auth.ScopeDocuments}}),
	})
	doc := docWithClaims(t, identifier.New(), permissionClaim(t, identifier.New(), "collab", auth.ActionUpdate))

	assert.True(t, peerdb.TestingCheckRoleDocumentPermission(context.Background(), site, auth.ActionRead, doc))
	assert.True(t, peerdb.TestingCheckRoleDocumentPermission(auth.WithRoles(context.Background(), []string{"editor"}), site, auth.ActionUpdate, doc))

	// The subject's own permission claims do not count.
	assert.False(t, peerdb.TestingCheckRoleDocumentPermission(auth.WithSubject(context.Background(), "collab"), site, auth.ActionUpdate, doc))
}

// TestTopLevelClaimsByID verifies serialization of a document's top-level claims by their IDs:
// sub-claims are serialized within their top-level claim and are not entries themselves.
func TestTopLevelClaimsByID(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	topID := identifier.New()
	permID := identifier.New()
	doc := docWithClaims(t, identifier.New(), refClaim(topID, prop, identifier.New()), permissionClaim(t, permID, "collab", auth.ActionUpdate))

	claims, errE := peerdb.TestingTopLevelClaimsByID(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, claims, 2)
	assert.Equal(t, prop, claims[topID].Prop)
	assert.Equal(t, internalCore.HasPermissionPropID, claims[permID].Prop)
	assert.Equal(t, reflect.TypeFor[*document.ReferenceClaim](), claims[permID].Type)
	assert.Contains(t, string(claims[permID].Data), internalCore.PermissionUserPropID.String())
	assert.Contains(t, string(claims[permID].Data), "collab")

	claims, errE = peerdb.TestingTopLevelClaimsByID(docWithClaims(t, identifier.New()))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, claims)
}

// TestChangedClaimProperties verifies which properties a difference between two documents is
// attributed to, and which granted actions the HAS_PERMISSION claims among the added or changed
// claims contribute.
func TestChangedClaimProperties(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()
	subProp := identifier.New()
	target := identifier.New()
	docID := identifier.New()

	claim1 := refClaim(identifier.New(), prop1, target)
	claim2 := refClaim(identifier.New(), prop2, target)
	baseDoc := docWithClaims(t, docID, claim1, claim2)

	// Equal documents have no changed properties.
	changed, _, errE := peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, claim1, claim2))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, changed)

	// An added claim contributes its property.
	changed, _, errE = peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, claim1, claim2, refClaim(identifier.New(), prop1, identifier.New())))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true}, changed)

	// A removed claim contributes its property.
	changed, _, errE = peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, claim1))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop2: true}, changed)

	// A claim with a modified value contributes its property.
	changed, _, errE = peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, claim1, refClaim(claim2.ID, prop2, identifier.New())))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop2: true}, changed)

	// A modified sub-claim contributes its top-level claim's property.
	withSub := refClaim(claim1.ID, prop1, target)
	errE = withSub.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: subProp},
		String:    "note",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	changed, _, errE = peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, withSub, claim2))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true}, changed)

	// A claim moved to another property contributes both properties.
	changed, _, errE = peerdb.TestingChangedClaimProperties(baseDoc, docWithClaims(t, docID, refClaim(claim1.ID, prop2, target), claim2))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true, prop2: true}, changed)

	// A cast to a claim type with equal fields contributes the property.
	castID := identifier.New()
	changed, _, errE = peerdb.TestingChangedClaimProperties(
		docWithClaims(t, docID, &document.NoneClaim{
			CoreClaim: document.CoreClaim{ID: castID, Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: prop1},
		}),
		docWithClaims(t, docID, &document.HasClaim{
			CoreClaim: document.CoreClaim{ID: castID, Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: prop1},
		}),
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true}, changed)

	// An added HAS_PERMISSION claim contributes the action it grants, a removed one does not.
	grantDoc := docWithClaims(t, docID, claim1, claim2, permissionClaim(t, identifier.New(), "user1", auth.ActionUpdate))
	_, grantedActions, errE := peerdb.TestingChangedClaimProperties(baseDoc, grantDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{auth.ActionUpdate}, grantedActions)
	_, grantedActions, errE = peerdb.TestingChangedClaimProperties(grantDoc, baseDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, grantedActions)
}

// TestHasPermission verifies the document permission check of the service: it requires a site in ctx
// and delegates to role grants and document permission claims.
func TestHasPermission(t *testing.T) {
	t.Parallel()

	var service peerdb.Service

	// Without a site in ctx access is denied.
	errE := service.HasDocumentPermission(context.Background(), auth.ActionRead, nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	site := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
	})
	ctx := waf.WithSite(context.Background(), site)

	errE = service.HasDocumentPermission(ctx, auth.ActionRead, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasDocumentPermission(ctx, auth.ActionUpdate, nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// The document's own permission claims count for the subject bound to ctx.
	doc := docWithClaims(t, identifier.New(), permissionClaim(t, identifier.New(), "collab", auth.ActionUpdate))
	errE = service.HasDocumentPermission(auth.WithSubject(ctx, "collab"), auth.ActionUpdate, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasDocumentPermission(auth.WithSubject(ctx, "other"), auth.ActionUpdate, doc)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}

// TestHasFilePermission verifies the file permission check of the service: it requires a site in ctx
// and delegates to role grants with a scope covering files.
func TestHasFilePermission(t *testing.T) {
	t.Parallel()

	var service peerdb.Service

	// Without a site in ctx access is denied.
	errE := service.HasFilePermission(context.Background(), auth.ActionRead)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	site := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
		"uploader":        auth.MustParseRoleGrants(map[string][]string{auth.ActionCreateCode: {auth.ScopeFiles}}),
	})
	ctx := waf.WithSite(context.Background(), site)

	errE = service.HasFilePermission(ctx, auth.ActionRead)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasFilePermission(auth.WithRoles(ctx, []string{"uploader"}), auth.ActionCreate)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasFilePermission(ctx, auth.ActionCreate)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}

// TestDefaultDocumentPreHook verifies the partial read check before a document is fetched: internal
// reads (no site in ctx) and authenticated callers always pass, anonymous callers pass only when a
// role grant can cover documents.
func TestDefaultDocumentPreHook(t *testing.T) {
	t.Parallel()

	errE := peerdb.DefaultDocumentPreHook(context.Background(), identifier.New(), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	openSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
	})
	closedSite := rolesSite(map[string]auth.RoleGrants{})

	errE = peerdb.DefaultDocumentPreHook(waf.WithSite(context.Background(), openSite), identifier.New(), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.DefaultDocumentPreHook(waf.WithSite(context.Background(), closedSite), identifier.New(), nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = peerdb.DefaultDocumentPreHook(auth.WithSubject(waf.WithSite(context.Background(), closedSite), "user1"), identifier.New(), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
}

// TestDefaultDocumentPostHook verifies the read check on a fetched document: an error or a missing
// document passes through, internal reads (no site in ctx) pass, and otherwise role grants and the
// document's own permission claims decide, with another version than the latest readable only at
// its own state or with the historic read action on the latest version.
func TestDefaultDocumentPostHook(t *testing.T) {
	t.Parallel()

	openSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
	})
	closedSite := rolesSite(map[string]auth.RoleGrants{})
	doc := docWithClaims(t, identifier.New(), permissionClaim(t, identifier.New(), "collab", auth.ActionRead))
	version := store.Version{Changeset: identifier.New(), Revision: 1}

	errIn := errors.New("test error")
	gotDoc, _, _, _, errE := peerdb.DefaultDocumentPostHook(waf.WithSite(context.Background(), closedSite), doc, doc, nil, version, nil, errIn) //nolint:dogsled
	assert.Equal(t, errIn, errE)
	assert.Equal(t, doc, gotDoc)

	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(waf.WithSite(context.Background(), closedSite), nil, nil, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(context.Background(), doc, doc, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(waf.WithSite(context.Background(), openSite), doc, doc, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	ctxCollabClosed := auth.WithSubject(waf.WithSite(context.Background(), closedSite), "collab")
	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(ctxCollabClosed, doc, doc, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(waf.WithSite(context.Background(), closedSite), doc, doc, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// The versioned-read rule. A version carrying the caller's read claim is readable while the
	// latest version carries it, too.
	docID := identifier.New()
	latestWith := docWithClaims(t, docID, permissionClaim(t, identifier.New(), "collab", auth.ActionRead))
	latestWithout := docWithClaims(t, docID)
	latestHistoric := docWithClaims(t, docID,
		permissionClaim(t, identifier.New(), "collab", auth.ActionRead), permissionClaim(t, identifier.New(), "collab", auth.ActionReadHistoric))
	oldWith := docWithClaims(t, docID, permissionClaim(t, identifier.New(), "collab", auth.ActionRead))
	oldWithout := docWithClaims(t, docID)
	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(ctxCollabClosed, oldWith, latestWith, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	// Without the read action at the latest version no version is readable: revoking access revokes
	// it for all versions.
	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(ctxCollabClosed, oldWith, latestWithout, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// A version at which the caller had no read access needs the historic read action on the latest
	// version.
	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(ctxCollabClosed, oldWithout, latestWith, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	_, _, _, _, errE = peerdb.DefaultDocumentPostHook(ctxCollabClosed, oldWithout, latestHistoric, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)

	// A document deleted at its latest version (nil latest) is not accessible at any version, and
	// nothing of it is returned.
	gotDoc, _, _, _, errE = peerdb.DefaultDocumentPostHook(waf.WithSite(context.Background(), openSite), doc, nil, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	assert.Nil(t, gotDoc)
}

// TestDefaultFilePreHook verifies the read check before a file is fetched: internal reads (no site
// in ctx) pass, and otherwise a role grant with a scope covering files is required.
func TestDefaultFilePreHook(t *testing.T) {
	t.Parallel()

	errE := peerdb.DefaultFilePreHook(context.Background(), identifier.New(), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	openSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
	})
	documentsSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeDocuments}}),
	})

	errE = peerdb.DefaultFilePreHook(waf.WithSite(context.Background(), openSite), identifier.New(), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = peerdb.DefaultFilePreHook(waf.WithSite(context.Background(), documentsSite), identifier.New(), nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}

// TestDefaultFilePostHook verifies the read check on a fetched file: an error passes through,
// internal reads (no site in ctx) pass, the file's latest version requires the read action on
// files, any other version also the historic read action, and a denied fetch closes the file
// handle and returns no file.
func TestDefaultFilePostHook(t *testing.T) {
	t.Parallel()

	openSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
	})
	historicSite := rolesSite(map[string]auth.RoleGrants{
		auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{
			auth.ActionReadCode:         {auth.ScopeAll},
			auth.ActionReadHistoricCode: {auth.ScopeAll},
		}),
	})
	closedSite := rolesSite(map[string]auth.RoleGrants{})
	version := store.Version{Changeset: identifier.New(), Revision: 1}
	latestVersion := store.Version{Changeset: identifier.New(), Revision: 1}

	file := &testReadSeekCloser{closed: false}
	errIn := errors.New("test error")
	gotFile, _, _, _, errE := peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), closedSite), file, &version, nil, version, nil, errIn) //nolint:dogsled
	assert.Equal(t, errIn, errE)
	assert.Equal(t, file, gotFile)
	assert.False(t, file.closed)

	_, _, _, _, errE = peerdb.DefaultFilePostHook(context.Background(), file, &version, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, file.closed)

	// The file's latest version requires the read action on files alone.
	_, _, _, _, errE = peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), openSite), file, &version, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, file.closed)

	// Any other version requires the historic read action on files as well.
	_, _, _, _, errE = peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), historicSite), file, &latestVersion, nil, version, nil, nil) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, file.closed)

	gotFile, _, _, _, errE = peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), openSite), file, &latestVersion, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	assert.Nil(t, gotFile)
	assert.True(t, file.closed)
	file.closed = false

	// A file deleted at its latest version (nil latest version) is not accessible at any version.
	gotFile, _, _, _, errE = peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), historicSite), file, nil, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	assert.Nil(t, gotFile)
	assert.True(t, file.closed)
	file.closed = false

	gotFile, _, _, _, errE = peerdb.DefaultFilePostHook(waf.WithSite(context.Background(), closedSite), file, &version, nil, version, nil, nil) //nolint:dogsled
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	assert.Nil(t, gotFile)
	assert.True(t, file.closed)
}

// TestSessionPermissions verifies against a running site the seeding of permission claims into
// create sessions and session access across the session lifecycle.
func TestSessionPermissions(t *testing.T) {
	t.Parallel()

	var globals *peerdb.Globals
	_, service := startTestServer(t, func(g *peerdb.Globals, _ *peerdb.ServeCommand) {
		globals = g
		g.Sites = []internalSite.Site{
			{
				Site: waf.Site{
					Domain:   "localhost",
					CertFile: "",
					KeyFile:  "",
				},
				Build:            nil,
				IndexPrefix:      "",
				Schema:           "",
				Title:            "Example Site",
				Logo:             nil,
				Favicon:          internalSite.Favicon{},
				LanguagePriority: nil,
				DefaultLanguage:  "",
				LanguageCodes:    nil,
				Features:         internalSite.SiteFeatures{},
				Roles: map[string]auth.RoleGrants{
					auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
					"creator":         auth.MustParseRoleGrants(map[string][]string{auth.ActionCreateCode: {auth.ScopeDocuments}}),
				},
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

	ctx := peerdb.WithFallbackDBContext(waf.WithSite(t.Context(), site), site.Schema, "tests")
	sysCtx := base.WithSystemSession(ctx)
	ctxCollab := auth.WithSubject(ctx, "collab")
	ctxStranger := auth.WithSubject(ctx, "stranger")
	ctxCreator := auth.WithRoles(ctx, []string{"creator"})

	// A session which does not exist is reported as not found and not as a permission denial.
	errE := service.HasSessionPermission(ctx, identifier.New())
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	// Whether a session is accessible through the API depends on the context it was begun with, and
	// on nothing else: the two sessions below reach the same document state (the permission claims a
	// create session seeds, granting their user the update action) through the same system-context
	// appends, and are checked by the same caller.
	seedSession := func(t *testing.T, beginCtx context.Context) identifier.Identifier {
		t.Helper()

		sessionBase := []string{"test", identifier.New().String()}
		s, errE := site.Base.BeginCreateDocument(beginCtx, sessionBase)
		require.NoError(t, errE, "% -+#.1v", errE)
		// The seeding of a create session appends with a system context, on behalf of the application.
		_, errE = site.Base.AppendDocumentChanges(sysCtx, s, base.PermissionClaimsChanges(
			"creator1", []identifier.Identifier{auth.ActionUpdate}, s, sessionBase, 0,
		), 0)
		require.NoError(t, errE, "% -+#.1v", errE)
		return s
	}
	ctxSeeded := auth.WithSubject(ctx, "creator1")

	// A session opened on behalf of the caller stays the caller's session even though the
	// application appended to it with a system context.
	callerSession := seedSession(t, ctx)
	errE = service.HasSessionPermission(ctxSeeded, callerSession)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A session begun with a system context is driven by the application itself: it is denied to
	// every caller, including the one the same session grants the update action to.
	systemSession := seedSession(t, sysCtx)
	errE = service.HasSessionPermission(ctxSeeded, systemSession)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// permissionClaimChanges builds a complete permission claim granting the user the action, which
	// can be appended to the session and applied to a document.
	docBase := []string{"test", identifier.New().String()}
	session, errE := site.Base.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)
	claimChanges := base.PermissionClaimsChanges("collab", []identifier.Identifier{auth.ActionUpdate}, session, docBase, 0)
	require.Len(t, claimChanges, 3)
	_, errE = site.Base.AppendDocumentChanges(sysCtx, session, claimChanges, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc := &document.D{CoreDocument: document.CoreDocument{ID: identifier.From(docBase...), Base: docBase}}
	errE = claimChanges.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, auth.HasPermissionClaim(auth.ActionUpdate, "collab", doc))
	_, doc, errE = site.Base.SessionDocumentRaw(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, auth.HasPermissionClaim(auth.ActionUpdate, "collab", doc))
	assert.False(t, auth.HasPermissionClaim(auth.ActionUpdate, "stranger", doc))

	// PermissionClaimsChanges returns the changes granting the creator the given actions.
	seedChanges := base.PermissionClaimsChanges("creator1", base.DefaultCreatorActions, session, docBase, 3)
	assert.Len(t, seedChanges, 15)
	_, errE = site.Base.AppendDocumentChanges(sysCtx, session, seedChanges, 3)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, doc, errE = site.Base.SessionDocumentRaw(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	for _, action := range base.DefaultCreatorActions {
		assert.True(t, auth.HasPermissionClaim(action, "creator1", doc), action.String())
	}

	// An active session is accessible to users granted the update action within it, and only to them.
	errE = service.HasSessionPermission(ctxCollab, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasSessionPermission(ctxStranger, session)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// After the session commits, the check is made against the committed version, so access does not
	// change across the completion boundary.
	errE = site.Base.EndEditDocument(sysCtx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, errE := site.Base.SessionDocumentRaw(ctx, session)
		assert.ErrorIs(c, errE, coordinator.ErrAlreadyCompleted)
	}, 10*time.Second, 10*time.Millisecond)
	errE = service.HasSessionPermission(ctxCollab, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasSessionPermission(ctxStranger, session)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// After an edit session completes without committing, the check is made against the version the
	// session branched from: users granted the update action before the session keep access.
	docID := identifier.From(docBase...)
	parentDoc, _, version, _, errE := site.Base.GetDocumentLatestDoc(sysCtx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	editSession, errE := site.Base.BeginEditDocument(ctx, version, parentDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = site.Base.EndEditDocument(sysCtx, editSession, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, errE := site.Base.SessionDocumentRaw(ctx, editSession)
		assert.ErrorIs(c, errE, coordinator.ErrAlreadyCompleted)
	}, 10*time.Second, 10*time.Millisecond)
	errE = service.HasSessionPermission(ctxCollab, editSession)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasSessionPermission(ctxStranger, editSession)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// After a create session completes without committing, no document related to the session exists
	// anymore and only role grants of the create action allow access.
	createSession, errE := site.Base.BeginCreateDocument(ctx, []string{"test", identifier.New().String()})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = site.Base.EndEditDocument(sysCtx, createSession, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, errE := site.Base.SessionDocumentRaw(ctx, createSession)
		assert.ErrorIs(c, errE, coordinator.ErrAlreadyCompleted)
	}, 10*time.Second, 10*time.Millisecond)
	errE = service.HasSessionPermission(ctxCreator, createSession)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = service.HasSessionPermission(ctxCollab, createSession)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
}
