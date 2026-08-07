package auth_test

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// makePermissionsDoc returns an empty document to which permission and reference claims are added.
func makePermissionsDoc(t *testing.T) *document.D {
	t.Helper()

	id := identifier.New()
	return &document.D{CoreDocument: document.CoreDocument{ID: id, Base: []string{"test", id.String()}}}
}

// addPermissionClaim adds to the document a HAS_PERMISSION claim with the action as the value, a
// PERMISSION_USER sub-claim with the user (unless empty), and a PERMISSION_SCOPE sub-claim per scope,
// all with the given confidence.
func addPermissionClaim(t *testing.T, doc *document.D, user string, action identifier.Identifier, confidence document.Confidence, scopes ...string) {
	t.Helper()

	addPermissionClaimOfProp(t, doc, internalCore.HasPermissionPropID, user, action, confidence, scopes...)
}

// addPermissionClaimOfProp is addPermissionClaim generalized over the claim property, so tests can
// also add HAS_REQUESTED_PERMISSION claims.
func addPermissionClaimOfProp(
	t *testing.T, doc *document.D, prop identifier.Identifier, user string, action identifier.Identifier, confidence document.Confidence, scopes ...string,
) {
	t.Helper()

	claim := &document.ReferenceClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
		Prop:      document.Reference{ID: prop},
		To:        document.Reference{ID: action},
	}
	if user != "" {
		errE := claim.Add(&document.IdentifierClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
			Prop:      document.Reference{ID: internalCore.PermissionUserPropID},
			Value:     user,
		})
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	for _, scope := range scopes {
		errE := claim.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
			Prop:      document.Reference{ID: internalCore.PermissionScopePropID},
			String:    scope,
		})
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	errE := doc.Add(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestScopes(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()

	scopes, errE := auth.ParseScopes("all&files&documents&self&" + prop.String() + "=" + value.String() + "&" + prop.String() + "=" + identifier.New().String())
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, scopes, 6)
	all, files, documents, self, claimScope, otherClaimScope := scopes[0], scopes[1], scopes[2], scopes[3], scopes[4], scopes[5]

	assert.Equal(t, prop.String()+"="+value.String(), claimScope.String())

	assert.True(t, all.CoversDocuments())
	assert.True(t, all.MatchesFiles())

	assert.True(t, documents.CoversDocuments())
	assert.False(t, documents.MatchesFiles())

	assert.False(t, files.CoversDocuments())
	assert.True(t, files.MatchesFiles())

	// The self scope comes only from document-level permission claims. It never appears in role
	// grants, the only scopes checked without a document or against files, so those checks panic.
	assert.Panics(t, func() { self.CoversDocuments() })
	assert.Panics(t, func() { self.MatchesFiles() })

	assert.True(t, claimScope.CoversDocuments())
	assert.False(t, claimScope.MatchesFiles())

	assert.True(t, otherClaimScope.CoversDocuments())
	assert.False(t, otherClaimScope.MatchesFiles())

	// An unknown literal scope cannot come out of ParseScopes, so evaluating one panics.
	unknown := auth.Scope{Literal: "unknown", Prop: identifier.Identifier{}, Value: identifier.Identifier{}}
	assert.Panics(t, func() { unknown.CoversDocuments() })
	assert.Panics(t, func() { unknown.MatchesFiles() })

	// A scope entry has to be a literal scope or a "property=value" entry with a single identifier on
	// each side.
	_, errE = auth.ParseScopes("all&foo=bar")
	assert.EqualError(t, errE, "scope entry must be a property and a value, each a single identifier")
	_, errE = auth.ParseScopes(prop.String() + ":" + prop.String() + "=" + value.String())
	assert.EqualError(t, errE, "scope entry must be a property and a value, each a single identifier")
	_, errE = auth.ParseScopes("reverse")
	assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
}

func TestParseRoleGrants(t *testing.T) {
	t.Parallel()

	grants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode:     {auth.ScopeAll, auth.ScopeDocuments + "&" + auth.ScopeFiles},
		auth.ActionReadBulkCode: {auth.ScopeAll},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, grants[auth.ActionRead], 3)
	assert.Len(t, grants[auth.ActionReadBulk], 1)

	// The bulk read action is granted per object kind, so it takes the documents and files scopes as
	// well, allowing bulk reading of documents and of files to be granted independently.
	grants, errE = auth.ParseRoleGrants(map[string][]string{auth.ActionReadBulkCode: {auth.ScopeDocuments}})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, grants[auth.ActionReadBulk], 1)
	assert.True(t, grants[auth.ActionReadBulk][0].CoversDocuments())
	assert.False(t, grants[auth.ActionReadBulk][0].MatchesFiles())

	grants, errE = auth.ParseRoleGrants(map[string][]string{auth.ActionReadBulkCode: {auth.ScopeFiles}})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, grants[auth.ActionReadBulk], 1)
	assert.False(t, grants[auth.ActionReadBulk][0].CoversDocuments())
	assert.True(t, grants[auth.ActionReadBulk][0].MatchesFiles())

	_, errE = auth.ParseRoleGrants(map[string][]string{"ACTION_UNKNOWN": {auth.ScopeAll}})
	assert.EqualError(t, errE, "unknown permission action")

	// The self scope is valid only in document-level permission claims, not in role configuration.
	_, errE = auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeSelf}})
	assert.EqualError(t, errE, "the self scope is valid only in document-level permission claims")

	// A bulk read is not about a particular document, so claim scopes are not supported for it.
	_, errE = auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadBulkCode: {identifier.New().String() + "=" + identifier.New().String()},
	})
	assert.EqualError(t, errE, "the bulk read action supports only literal scopes")
}

func TestHasPermissionClaim(t *testing.T) {
	t.Parallel()

	// A claim with the user and the self scope grants the action, and only that action, to that user.
	doc := makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	assert.True(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))
	assert.False(t, auth.HasPermissionClaim(auth.ActionUpdate, "user1", doc))
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user2", doc))
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "", doc))

	// The PERMISSION_USER sub-claim is required.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	// The PERMISSION_SCOPE sub-claim is required as well.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence)
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	// Self is the only scope valid in document-level permission claims: any other scope grants
	// nothing, but it also does not invalidate a claim which carries the self scope besides it.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeAll)
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeAll, auth.ScopeSelf)
	assert.True(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	// The same holds for entries within one PERMISSION_SCOPE sub-claim, which are a union.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeAll+"&"+auth.ScopeSelf)
	assert.True(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	// A claim with too low confidence grants nothing.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.LowConfidence/2, auth.ScopeSelf)
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))

	// The create action is never granted: a document's own claims cannot grant creating it.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionCreate, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, auth.HasPermissionClaim(auth.ActionCreate, "user1", doc))
	assert.Empty(t, auth.PermissionClaimGrants(doc))
}

func TestGrantsAllowsDocument(t *testing.T) {
	t.Parallel()

	grants, errE := auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeDocuments}})
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := makePermissionsDoc(t)

	// A role grant with a scope covering the document allows the action.
	assert.True(t, grants.AllowsDocument(auth.ActionRead, doc))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, doc))

	// With a nil document the grants are checked against documents in general.
	assert.True(t, grants.AllowsDocument(auth.ActionRead, nil))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, nil))

	// The document's own permission claims are a separate arm which does not flow through role
	// grants: AllowsDocument stays false, while HasDocumentPermission combines both arms and grants
	// the action to the user the claims name, never to an anonymous caller, and never without a
	// document.
	addPermissionClaim(t, doc, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, doc))
	roleGrants := map[string]auth.RoleGrants{auth.RoleEveryone: grants}
	assert.True(t, auth.HasDocumentPermission(roleGrants, auth.ActionUpdate, "user1", nil, doc))
	assert.False(t, auth.HasDocumentPermission(roleGrants, auth.ActionUpdate, "user2", nil, doc))
	assert.False(t, auth.HasDocumentPermission(roleGrants, auth.ActionUpdate, "", nil, doc))
	assert.False(t, auth.HasDocumentPermission(roleGrants, auth.ActionUpdate, "user1", nil, nil))

	// Even without any role grants the claim arm applies, as long as it grants what the action requires
	// as well: here reading comes from the everyone grant above, so without it the update action alone
	// is not held.
	assert.False(t, auth.HasDocumentPermission(nil, auth.ActionUpdate, "user1", nil, doc))
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	assert.True(t, auth.HasDocumentPermission(nil, auth.ActionUpdate, "user1", nil, doc))

	// Files are covered only by grants whose scopes match files.
	assert.False(t, grants.AllowsFiles(auth.ActionRead))
	allGrants, errE := auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, allGrants.AllowsFiles(auth.ActionRead))

	// Claim scopes have to be fully satisfied by the document: for a granted property, every value the
	// document carries has to be granted, and a document without any claim of a granted property is
	// not allowed.
	prop := identifier.New()
	classA := identifier.New()
	classB := identifier.New()
	addClass := func(doc *document.D, class identifier.Identifier) {
		errE := doc.Add(&document.ReferenceClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: prop},
			To:        document.Reference{ID: class},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	singleDoc := makePermissionsDoc(t)
	addClass(singleDoc, classA)
	multiDoc := makePermissionsDoc(t)
	addClass(multiDoc, classA)
	addClass(multiDoc, classB)

	classGrants, errE := auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {prop.String() + "=" + classA.String()}})
	require.NoError(t, errE, "% -+#.1v", errE)
	bothGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode: {prop.String() + "=" + classA.String() + "&" + prop.String() + "=" + classB.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.True(t, classGrants.AllowsDocument(auth.ActionRead, singleDoc))
	assert.False(t, classGrants.AllowsDocument(auth.ActionRead, multiDoc))
	assert.True(t, bothGrants.AllowsDocument(auth.ActionRead, multiDoc))
	assert.False(t, classGrants.AllowsDocument(auth.ActionRead, makePermissionsDoc(t)))

	// An unknown literal scope cannot come out of ParseRoleGrants, so evaluating one panics, and so
	// does the self scope, which is valid only in document-level permission claims.
	bogus := auth.RoleGrants{auth.ActionRead: {auth.Scope{Literal: "unknown", Prop: identifier.Identifier{}, Value: identifier.Identifier{}}}}
	assert.Panics(t, func() { bogus.AllowsDocument(auth.ActionRead, singleDoc) })
	selfish := auth.RoleGrants{auth.ActionRead: {auth.Scope{Literal: auth.ScopeSelf, Prop: identifier.Identifier{}, Value: identifier.Identifier{}}}}
	assert.Panics(t, func() { selfish.AllowsDocument(auth.ActionRead, singleDoc) })
}

func TestScopeProperties(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()
	value := identifier.New()

	grants1, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode:   {auth.ScopeAll, prop1.String() + "=" + value.String()},
		auth.ActionCreateCode: {prop2.String() + "=" + value.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants2, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionUpdateCode: {auth.ScopeDocuments},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	properties := auth.ScopeProperties(map[string]auth.RoleGrants{"role1": grants1, "role2": grants2})
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true, prop2: true}, properties)
}

// TestGrantsUnmarshalYAML verifies that grants unmarshal from the configuration form (action codes and
// permission scope expressions) into their resolved form, with the same validation as ParseRoleGrants.
// A decoder which does not call the unmarshaler decodes the map itself and fails on the action codes,
// which are not document IDs.
func TestGrantsUnmarshalYAML(t *testing.T) {
	t.Parallel()

	var grants auth.RoleGrants
	err := yaml.Unmarshal([]byte("ACTION_READ: [all, documents&files]\nACTION_UPDATE: [documents]"), &grants)
	require.NoError(t, err)
	assert.Len(t, grants[auth.ActionRead], 3)
	assert.Len(t, grants[auth.ActionUpdate], 1)

	// The grants of a whole role map resolve the same way, which is how a site configures them.
	var roles map[string]auth.RoleGrants
	err = yaml.Unmarshal([]byte("admin:\n  ACTION_READ: [all]\n"), &roles)
	require.NoError(t, err)
	assert.Len(t, roles["admin"][auth.ActionRead], 1)

	// A role which configures no actions has no grants.
	var empty auth.RoleGrants
	err = yaml.Unmarshal([]byte(""), &empty)
	require.NoError(t, err)
	assert.Empty(t, empty)

	err = yaml.Unmarshal([]byte("ACTION_UNKNOWN: [all]"), &grants)
	assert.ErrorContains(t, err, "unknown permission action")

	err = yaml.Unmarshal([]byte("ACTION_READ: [self]"), &grants)
	assert.ErrorContains(t, err, "self scope is valid only")
}

// TestHasDocumentPermission verifies the explicit-identity permission core: role grants (of the
// listed roles and of the reserved everyone role) are evaluated with their scopes against the document,
// document-level permission claims count for actions they can grant (e.g. update) and never for the
// create action, a claim-scoped grant covers only documents carrying a matching claim, and an action is
// held only together with what it requires.
func TestHasDocumentPermission(t *testing.T) {
	t.Parallel()

	class1 := identifier.New()
	class2 := identifier.New()

	createGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionCreateCode: {internalCore.Namespace + ",INSTANCE_OF=" + class1.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	// Everyone reads every document, the shape of a site where the update action is the one being
	// granted: the update action requires reading, so it is held only where reading is.
	readGrants, errE := auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeDocuments}})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants := map[string]auth.RoleGrants{"creator1": createGrants, auth.RoleEveryone: readGrants}

	makeDoc := func(class *identifier.Identifier) *document.D {
		doc := makePermissionsDoc(t)
		if class != nil {
			errE := doc.Add(&document.ReferenceClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
				To:        document.Reference{ID: *class},
			})
			require.NoError(t, errE, "% -+#.1v", errE)
		}
		return doc
	}

	// A claim-scoped grant covers a document of the granted class.
	assert.True(t, auth.HasDocumentPermission(grants, auth.ActionCreate, "user1", []string{"creator1"}, makeDoc(&class1)))

	// A document of another class, or without a class, is not covered.
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionCreate, "user1", []string{"creator1"}, makeDoc(&class2)))
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionCreate, "user1", []string{"creator1"}, makeDoc(nil)))

	// Only the listed roles are evaluated.
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionCreate, "user1", []string{"other"}, makeDoc(&class1)))

	// Permission claims on the document never grant the create action.
	doc := makeDoc(&class2)
	addPermissionClaim(t, doc, "user1", auth.ActionCreate, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionCreate, "user1", []string{"creator1"}, doc))

	// For the update action the document's claims do count, for the claimed user only.
	doc = makeDoc(&class2)
	addPermissionClaim(t, doc, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	assert.True(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user1", nil, doc))
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user2", nil, doc))

	// A claim-scoped update grant covers only documents staying in the granted class.
	updateGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionUpdateCode: {internalCore.Namespace + ",INSTANCE_OF=" + class1.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants["editor1"] = updateGrants
	assert.True(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user1", []string{"editor1"}, makeDoc(&class1)))
	assert.False(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user1", []string{"editor1"}, makeDoc(&class2)))

	// A grant of the reserved everyone role applies without any listed roles. It grants reading as well,
	// because the update action is held only together with it.
	everyoneGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionReadCode:   {auth.ScopeDocuments},
		auth.ActionUpdateCode: {internalCore.Namespace + ",INSTANCE_OF=" + class1.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants[auth.RoleEveryone] = everyoneGrants
	assert.True(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user2", nil, makeDoc(&class1)))

	// An action is held only together with what it requires: without the read action nothing grants
	// updating, whichever arm grants the update action itself.
	noRead := map[string]auth.RoleGrants{"editor1": updateGrants}
	assert.False(t, auth.HasDocumentPermission(noRead, auth.ActionUpdate, "user1", []string{"editor1"}, makeDoc(&class1)))
	claimed := makeDoc(&class1)
	addPermissionClaim(t, claimed, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, auth.HasDocumentPermission(nil, auth.ActionUpdate, "user1", nil, claimed))
	// The requirement can come from the other arm: reading through a role grant is enough for an update
	// action a claim grants.
	assert.True(t, auth.HasDocumentPermission(map[string]auth.RoleGrants{auth.RoleEveryone: readGrants}, auth.ActionUpdate, "user1", nil, claimed))
}

// TestHasFilePermission verifies the explicit-identity permission core for files: grants of the listed
// roles and of the reserved everyone role are evaluated for a scope covering files.
func TestHasFilePermission(t *testing.T) {
	t.Parallel()

	fileGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionUpdateCode: {auth.ScopeFiles},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants := map[string]auth.RoleGrants{"editor1": fileGrants}

	assert.True(t, auth.HasFilePermission(grants, auth.ActionUpdate, []string{"editor1"}))
	assert.False(t, auth.HasFilePermission(grants, auth.ActionUpdate, []string{"other"}))
	assert.False(t, auth.HasFilePermission(grants, auth.ActionRead, []string{"editor1"}))

	// A grant of the reserved everyone role applies without any listed roles.
	grants[auth.RoleEveryone] = fileGrants
	assert.True(t, auth.HasFilePermission(grants, auth.ActionUpdate, nil))
}

// TestPermissionClaimGrants verifies the enumeration of document-level permission grants and that it
// matches HasPermissionClaim exactly.
func TestPermissionClaimGrants(t *testing.T) {
	t.Parallel()

	doc := makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	addPermissionClaim(t, doc, "user2", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	addPermissionClaim(t, doc, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	// A claim without the self scope, a claim below low confidence, and a create-action claim
	// contribute nothing.
	addPermissionClaim(t, doc, "user3", auth.ActionRead, document.HighConfidence, auth.ScopeAll)
	addPermissionClaim(t, doc, "user4", auth.ActionRead, document.LowConfidence/2, auth.ScopeSelf)
	addPermissionClaim(t, doc, "user5", auth.ActionCreate, document.HighConfidence, auth.ScopeSelf)

	assert.Equal(t, map[identifier.Identifier][]string{
		auth.ActionRead:   {"user1", "user2"},
		auth.ActionUpdate: {"user1"},
	}, auth.PermissionClaimGrants(doc))
}

// TestRequestedPermissionClaimGrants verifies that access requests are evaluated over
// HAS_REQUESTED_PERMISSION claims by the same rules as grants, and that requests and grants never
// leak into each other.
func TestRequestedPermissionClaimGrants(t *testing.T) {
	t.Parallel()

	doc := makePermissionsDoc(t)
	addPermissionClaimOfProp(t, doc, internalCore.HasRequestedPermissionPropID, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	addPermissionClaim(t, doc, "user2", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)

	assert.True(t, auth.HasRequestedPermissionClaim(auth.ActionRead, "user1", doc))
	assert.False(t, auth.HasRequestedPermissionClaim(auth.ActionRead, "user2", doc))
	assert.False(t, auth.HasPermissionClaim(auth.ActionRead, "user1", doc))
	assert.True(t, auth.HasPermissionClaim(auth.ActionRead, "user2", doc))
	assert.Equal(t, map[identifier.Identifier][]string{auth.ActionRead: {"user1"}}, auth.RequestedPermissionClaimGrants(doc))
}

// TestIsAction verifies that the permission action document IDs are recognized and other identifiers
// are not.
func TestIsAction(t *testing.T) {
	t.Parallel()

	for code, action := range auth.Actions {
		assert.True(t, auth.IsAction(action), code)
	}
	assert.False(t, auth.IsAction(identifier.New()))
	assert.False(t, auth.IsAction(identifier.Identifier{}))
}

// TestActionRequirements verifies which actions an action builds on, and that the returned slice is
// the caller's own.
func TestActionRequirements(t *testing.T) {
	t.Parallel()

	assert.Empty(t, auth.ActionRequirements(auth.ActionRead))
	assert.Empty(t, auth.ActionRequirements(auth.ActionCreate))
	assert.Empty(t, auth.ActionRequirements(identifier.New()))
	assert.Equal(t, []identifier.Identifier{auth.ActionRead}, auth.ActionRequirements(auth.ActionUpdate))
	assert.Equal(t, []identifier.Identifier{auth.ActionRead}, auth.ActionRequirements(auth.ActionReadHistoric))
	assert.Equal(t, []identifier.Identifier{auth.ActionRead}, auth.ActionRequirements(auth.ActionDelete))
	assert.Equal(t, []identifier.Identifier{auth.ActionUpdate}, auth.ActionRequirements(auth.ActionUpdatePermissions))

	requirements := auth.ActionRequirements(auth.ActionUpdate)
	requirements[0] = auth.ActionDelete
	assert.Equal(t, []identifier.Identifier{auth.ActionRead}, auth.ActionRequirements(auth.ActionUpdate))
}

// TestActionsClosure verifies that requirements are added transitively, that the given actions keep
// their order and come first, and that duplicates are dropped.
func TestActionsClosure(t *testing.T) {
	t.Parallel()

	assert.Empty(t, auth.ActionsClosure(nil))
	assert.Equal(t, []identifier.Identifier{auth.ActionRead}, auth.ActionsClosure([]identifier.Identifier{auth.ActionRead}))
	assert.Equal(t, []identifier.Identifier{auth.ActionUpdate, auth.ActionRead}, auth.ActionsClosure([]identifier.Identifier{auth.ActionUpdate}))
	assert.Equal(t, []identifier.Identifier{auth.ActionUpdatePermissions, auth.ActionUpdate, auth.ActionRead},
		auth.ActionsClosure([]identifier.Identifier{auth.ActionUpdatePermissions}))
	assert.Equal(t, []identifier.Identifier{auth.ActionDelete, auth.ActionRead, auth.ActionReadHistoric},
		auth.ActionsClosure([]identifier.Identifier{auth.ActionDelete, auth.ActionRead, auth.ActionReadHistoric}))
}

// TestValidateActions verifies that a set of actions is accepted only when it contains everything its
// actions require, and that the missing actions are reported.
func TestValidateActions(t *testing.T) {
	t.Parallel()

	require.NoError(t, auth.ValidateActions(nil))
	require.NoError(t, auth.ValidateActions([]identifier.Identifier{auth.ActionRead}))
	require.NoError(t, auth.ValidateActions([]identifier.Identifier{auth.ActionRead, auth.ActionUpdate}))
	require.NoError(t, auth.ValidateActions([]identifier.Identifier{auth.ActionRead, auth.ActionUpdate, auth.ActionUpdatePermissions}))

	errE := auth.ValidateActions([]identifier.Identifier{auth.ActionUpdate})
	assert.EqualError(t, errE, "permission actions are missing actions they require")
	assert.Equal(t, []string{auth.ActionRead.String()}, errors.Details(errE)["missing"])

	// The permissions action requires updating, which requires reading, so both are missing.
	errE = auth.ValidateActions([]identifier.Identifier{auth.ActionUpdatePermissions, auth.ActionDelete})
	assert.EqualError(t, errE, "permission actions are missing actions they require")
	assert.ElementsMatch(t, []string{auth.ActionRead.String(), auth.ActionUpdate.String()}, errors.Details(errE)["missing"])
}

// TestActionsRequiring verifies which actions build on an action, transitively.
func TestActionsRequiring(t *testing.T) {
	t.Parallel()

	assert.Empty(t, auth.ActionsRequiring(auth.ActionUpdatePermissions))
	assert.Empty(t, auth.ActionsRequiring(auth.ActionCreate))
	assert.Empty(t, auth.ActionsRequiring(identifier.New()))
	assert.ElementsMatch(t, []identifier.Identifier{auth.ActionUpdatePermissions}, auth.ActionsRequiring(auth.ActionUpdate))
	// Everything builds on reading, the permissions action through the update action.
	assert.ElementsMatch(t, []identifier.Identifier{
		auth.ActionReadBulk, auth.ActionReadHistoric, auth.ActionUpdate, auth.ActionUpdatePermissions, auth.ActionDelete,
	}, auth.ActionsRequiring(auth.ActionRead))
}

// TestRequestedPermissionClaims verifies which request claims are found for a subject: only claims of
// the requested actions, scoped to the document itself, and naming that subject.
func TestRequestedPermissionClaims(t *testing.T) {
	t.Parallel()

	doc := makePermissionsDoc(t)
	addPermissionClaimOfProp(t, doc, internalCore.HasRequestedPermissionPropID, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	addPermissionClaimOfProp(t, doc, internalCore.HasRequestedPermissionPropID, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	// Another user's request, a request scoped elsewhere, and a grant are all left alone.
	addPermissionClaimOfProp(t, doc, internalCore.HasRequestedPermissionPropID, "user2", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)
	addPermissionClaimOfProp(t, doc, internalCore.HasRequestedPermissionPropID, "user1", auth.ActionDelete, document.HighConfidence, auth.ScopeAll)
	addPermissionClaim(t, doc, "user1", auth.ActionRead, document.HighConfidence, auth.ScopeSelf)

	assert.Len(t, auth.RequestedPermissionClaims([]identifier.Identifier{auth.ActionRead}, "user1", doc), 1)
	assert.Len(t, auth.RequestedPermissionClaims([]identifier.Identifier{auth.ActionRead, auth.ActionUpdate}, "user1", doc), 2)
	assert.Empty(t, auth.RequestedPermissionClaims([]identifier.Identifier{auth.ActionDelete}, "user1", doc))
	assert.Empty(t, auth.RequestedPermissionClaims([]identifier.Identifier{auth.ActionRead}, "user3", doc))
	assert.Empty(t, auth.RequestedPermissionClaims([]identifier.Identifier{auth.ActionRead}, "", doc))
	assert.Empty(t, auth.RequestedPermissionClaims(nil, "user1", doc))
}
