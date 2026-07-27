package base_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// sourceCheckVisibility is the visibility configuration used by the default source check tests: a
// no-roles floor, a researcher level, and an editor level on top.
func sourceCheckVisibility() []auth.VisibilityLevel {
	return []auth.VisibilityLevel{
		{Name: "public", Roles: nil},
		{Name: "researcher", Roles: []string{"researcher"}},
		{Name: "editor", Roles: []string{"reviewer", "editor"}},
	}
}

// sourceCheckGrants returns role grants for the given roles, each granting read on everything, plus
// read on everything for everyone when withEveryone is set. The level-assigned roles plus the
// unassigned translator role always exist.
func sourceCheckGrants(withEveryone bool, readRoles ...string) map[string]auth.RoleGrants {
	readAll := map[string][]string{auth.ActionReadCode: {auth.ScopeAll}}
	grants := map[string]auth.RoleGrants{}
	for _, role := range []string{"researcher", "reviewer", "editor", "translator"} {
		if slices.Contains(readRoles, role) {
			grants[role] = auth.MustParseRoleGrants(readAll)
		} else {
			grants[role] = auth.MustParseRoleGrants(nil)
		}
	}
	if withEveryone {
		grants[auth.RoleEveryone] = auth.MustParseRoleGrants(readAll)
	}
	return grants
}

func TestDefaultIndexingSourceCheck(t *testing.T) {
	t.Parallel()

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
	}

	// With read for everyone, every document is a source at every level, and a document-level
	// permission claim changes nothing: permission claims grant actions to specific subjects and never
	// take access away, so they do not affect uniform readability.
	check := base.DefaultIndexingSourceCheck(sourceCheckVisibility(), sourceCheckGrants(true), false)
	claimed := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: internalCore.HasPermissionPropID},
					To:        document.Reference{ID: auth.ActionRead},
				},
			},
		},
	}
	for _, level := range []string{"public", "researcher", "editor"} {
		ctx := auth.WithVisibility(context.Background(), level)
		errE := check(ctx, doc, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = check(ctx, claimed, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	// Without read for everyone, but with read for every level-assigned role, the floor's anonymous
	// searcher cannot read, so documents are not sources at the floor, while at the role levels every
	// searcher holds a reading role.
	check = base.DefaultIndexingSourceCheck(sourceCheckVisibility(), sourceCheckGrants(false, "researcher", "reviewer", "editor"), false)
	errE := check(auth.WithVisibility(context.Background(), "public"), doc, nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	for _, level := range []string{"researcher", "editor"} {
		errE := check(auth.WithVisibility(context.Background(), level), doc, nil)
		require.NoError(t, errE, "% -+#.1v", errE, "level %s", level)
	}

	// When one of a level's roles cannot read, the level has a searcher who cannot read the document,
	// so it is not a source there, while a level whose single role reads stays fine.
	check = base.DefaultIndexingSourceCheck(sourceCheckVisibility(), sourceCheckGrants(false, "researcher", "editor"), false)
	errE = check(auth.WithVisibility(context.Background(), "editor"), doc, nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)
	errE = check(auth.WithVisibility(context.Background(), "researcher"), doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A no-roles last level is resolved to by no request (it is the unfiltered superset for internal
	// paths), so everything is a source there even when nobody has read granted.
	visibility := append(sourceCheckVisibility(), auth.VisibilityLevel{Name: "all", Roles: nil})
	check = base.DefaultIndexingSourceCheck(visibility, sourceCheckGrants(false), false)
	errE = check(auth.WithVisibility(context.Background(), "all"), doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = check(auth.WithVisibility(context.Background(), "public"), doc, nil)
	assert.ErrorIs(t, errE, auth.ErrAccessDenied)

	// A level not among the configured ones, including a ctx without any visibility, is a programming
	// error and fails loudly instead of treating the document as a source.
	errE = check(auth.WithVisibility(context.Background(), "staging"), doc, nil)
	assert.EqualError(t, errE, "unknown visibility level")
	errE = check(context.Background(), doc, nil)
	assert.EqualError(t, errE, "unknown visibility level")
}
