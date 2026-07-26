package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"
	"gopkg.in/yaml.v3"

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

	claim := &document.ReferenceClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
		Prop:      document.Reference{ID: internalCore.HasPermissionPropID},
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
	assert.Len(t, auth.PermissionClaimScopes(auth.ActionRead, "user1", doc), 1)
	assert.Empty(t, auth.PermissionClaimScopes(auth.ActionRead, "", doc))

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

	// The create action never has claim scopes: a document's own claims cannot grant creating it.
	doc = makePermissionsDoc(t)
	addPermissionClaim(t, doc, "user1", auth.ActionCreate, document.HighConfidence, auth.ScopeSelf)
	assert.False(t, auth.HasPermissionClaim(auth.ActionCreate, "user1", doc))
	assert.Empty(t, auth.PermissionClaimScopes(auth.ActionCreate, "user1", doc))
}

func TestGrantsAllowsDocument(t *testing.T) {
	t.Parallel()

	grants, errE := auth.ParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeDocuments}})
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := makePermissionsDoc(t)

	// A role grant with a scope covering the document allows the action for any caller, including an
	// anonymous one (an empty user).
	assert.True(t, grants.AllowsDocument(auth.ActionRead, "", doc))
	assert.True(t, grants.AllowsDocument(auth.ActionRead, "user1", doc))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, "user1", doc))

	// With a nil document the grants are checked against documents in general.
	assert.True(t, grants.AllowsDocument(auth.ActionRead, "user1", nil))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, "user1", nil))

	// The document's own permission claims allow the action also without a covering role grant, but
	// only for the user they name, never for an anonymous caller, and never without a document.
	addPermissionClaim(t, doc, "user1", auth.ActionUpdate, document.HighConfidence, auth.ScopeSelf)
	assert.True(t, grants.AllowsDocument(auth.ActionUpdate, "user1", doc))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, "user2", doc))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, "", doc))
	assert.False(t, grants.AllowsDocument(auth.ActionUpdate, "user1", nil))

	// Nil grants (a role without configuration) still consult the document's claims.
	assert.True(t, auth.Grants(nil).AllowsDocument(auth.ActionUpdate, "user1", doc))

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

	assert.True(t, classGrants.AllowsDocument(auth.ActionRead, "", singleDoc))
	assert.False(t, classGrants.AllowsDocument(auth.ActionRead, "", multiDoc))
	assert.True(t, bothGrants.AllowsDocument(auth.ActionRead, "", multiDoc))
	assert.False(t, classGrants.AllowsDocument(auth.ActionRead, "", makePermissionsDoc(t)))

	// An unknown literal scope cannot come out of ParseRoleGrants, so evaluating one panics.
	bogus := auth.Grants{auth.ActionRead: {auth.Scope{Literal: "unknown", Prop: identifier.Identifier{}, Value: identifier.Identifier{}}}}
	assert.Panics(t, func() { bogus.AllowsDocument(auth.ActionRead, "", singleDoc) })
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

	properties := auth.ScopeProperties(map[string]auth.Grants{"role1": grants1, "role2": grants2})
	assert.Equal(t, map[identifier.Identifier]bool{prop1: true, prop2: true}, properties)
}

// TestGrantsUnmarshalYAML verifies that grants unmarshal from the configuration form (action codes and
// permission scope expressions) into their resolved form, with the same validation as ParseRoleGrants.
func TestGrantsUnmarshalYAML(t *testing.T) {
	t.Parallel()

	var grants auth.Grants
	err := yaml.Unmarshal([]byte("ACTION_READ: [all, documents&files]\nACTION_UPDATE: [documents]"), &grants)
	require.NoError(t, err)
	assert.Len(t, grants[auth.ActionRead], 3)
	assert.Len(t, grants[auth.ActionUpdate], 1)

	err = yaml.Unmarshal([]byte("ACTION_UNKNOWN: [all]"), &grants)
	assert.ErrorContains(t, err, "unknown permission action")

	err = yaml.Unmarshal([]byte("ACTION_READ: [self]"), &grants)
	assert.ErrorContains(t, err, "self scope is valid only")
}

// TestHasDocumentPermission verifies the explicit-identity permission core: role grants (of the
// listed roles and of the reserved everyone role) are evaluated with their scopes against the document,
// document-level permission claims count for actions they can grant (e.g. update) and never for the
// create action, and a claim-scoped grant covers only documents carrying a matching claim.
func TestHasDocumentPermission(t *testing.T) {
	t.Parallel()

	class1 := identifier.New()
	class2 := identifier.New()

	createGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionCreateCode: {internalCore.Namespace + ",INSTANCE_OF=" + class1.String()},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants := map[string]auth.Grants{"creator1": createGrants}

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

	// A grant of the reserved everyone role applies without any listed roles.
	grants[auth.RoleEveryone] = updateGrants
	assert.True(t, auth.HasDocumentPermission(grants, auth.ActionUpdate, "user2", nil, makeDoc(&class1)))
}

// TestHasFilePermission verifies the explicit-identity permission core for files: grants of the listed
// roles and of the reserved everyone role are evaluated for a scope covering files.
func TestHasFilePermission(t *testing.T) {
	t.Parallel()

	fileGrants, errE := auth.ParseRoleGrants(map[string][]string{
		auth.ActionUpdateCode: {auth.ScopeFiles},
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	grants := map[string]auth.Grants{"editor1": fileGrants}

	assert.True(t, auth.HasFilePermission(grants, auth.ActionUpdate, []string{"editor1"}))
	assert.False(t, auth.HasFilePermission(grants, auth.ActionUpdate, []string{"other"}))
	assert.False(t, auth.HasFilePermission(grants, auth.ActionRead, []string{"editor1"}))

	// A grant of the reserved everyone role applies without any listed roles.
	grants[auth.RoleEveryone] = fileGrants
	assert.True(t, auth.HasFilePermission(grants, auth.ActionUpdate, nil))
}
