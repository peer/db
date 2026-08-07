package base_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/internal/testutils"
)

// TestDefaultSearchQueryHook verifies the default search access filter: unrestricted callers get no
// filter, and restricted callers get a filter matching their roles and, when signed in, their
// document-level permission claims.
func TestDefaultSearchQueryHook(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	class := identifier.From("class")
	b := &base.B{ //nolint:exhaustruct
		Roles: map[string]auth.RoleGrants{
			auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{
				auth.ActionReadCode: {prop.String() + "=" + class.String()},
			}),
			"admin":  auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}),
			"member": auth.MustParseRoleGrants(map[string][]string{}),
		},
	}
	hook := b.DefaultSearchQueryHook

	// A caller whose role grants read on all documents gets no restriction.
	q, errE := hook(auth.WithRoles(t.Context(), []string{"admin"}))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, q)

	// An anonymous caller is filtered to the documents readable by everyone.
	q, errE = hook(t.Context())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t,
		`{"bool":{"minimum_should_match":1,"should":[{"terms":{"readableByRoles":[""]}}]}}`,
		testutils.QueryJSON(t, q))

	// A signed-in caller with a role matches through the role (and the everyone role) or through the
	// document-level permission claims granting them the read action.
	q, errE = hook(auth.WithRoles(auth.WithSubject(t.Context(), "user1"), []string{"member"}))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t,
		`{"bool":{"minimum_should_match":1,"should":[`+
			`{"terms":{"readableByRoles":["","member"]}},`+
			`{"term":{"readableByUsers":{"value":"user1"}}}]}}`,
		testutils.QueryJSON(t, q))

	// A site whose grants make every document readable by everyone gets no restriction for any caller.
	openBase := &base.B{ //nolint:exhaustruct
		Roles: map[string]auth.RoleGrants{
			auth.RoleEveryone: auth.MustParseRoleGrants(map[string][]string{auth.ActionReadCode: {auth.ScopeDocuments}}),
		},
	}
	q, errE = openBase.DefaultSearchQueryHook(t.Context())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, q)
}
