package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/store"
)

func TestValidateVisibility(t *testing.T) {
	t.Parallel()

	roles := map[string][]string{
		"public":     nil,
		"researcher": nil,
		"reviewer":   nil,
		"editor":     nil,
		"admin":      nil,
	}

	tests := []struct {
		name       string
		roles      map[string][]string
		visibility []auth.VisibilityLevel
		wantErr    string
	}{
		{
			name:       "nil visibility is valid",
			roles:      roles,
			visibility: nil,
			wantErr:    "",
		},
		{
			name:  "role in no level is allowed",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: []string{"public"}},
				{Name: "researcher", Roles: []string{"researcher"}},
				{Name: "editor", Roles: []string{"reviewer", "editor"}},
				// "admin" is in no level, which is allowed.
			},
			wantErr: "",
		},
		{
			name:  "no-roles floor level as the first level is allowed",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: nil},
				{Name: "researcher", Roles: []string{"researcher"}},
				{Name: "editor", Roles: []string{"reviewer", "editor"}},
			},
			wantErr: "",
		},
		{
			name:  "no-roles level in the middle is rejected",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: []string{"public"}},
				{Name: "floor", Roles: nil},
				{Name: "editor", Roles: []string{"editor"}},
			},
			wantErr: "a visibility level with no roles must be the first or the last level",
		},
		{
			name:  "no-roles level as the last (top) level is allowed",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "public", Roles: []string{"public"}},
				{Name: "editor", Roles: []string{"editor"}},
				{Name: "all", Roles: nil},
			},
			wantErr: "",
		},
		{
			name:  "unknown role",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "researcher", Roles: []string{"nonexistent"}},
			},
			wantErr: "visibility level references an unknown role",
		},
		{
			name:  "role in more than one level",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "researcher", Roles: []string{"reviewer"}},
				{Name: "editor", Roles: []string{"reviewer"}},
			},
			wantErr: "role is assigned to more than one visibility level",
		},
		{
			name:  "duplicate level name",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "dup", Roles: []string{"public"}},
				{Name: "dup", Roles: []string{"researcher"}},
			},
			wantErr: "visibility level name is not unique",
		},
		{
			name:  "empty level name",
			roles: roles,
			visibility: []auth.VisibilityLevel{
				{Name: "", Roles: []string{"public"}},
			},
			wantErr: "visibility level has an empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &site.Site{}
			s.Roles = tt.roles
			s.Visibility = tt.visibility

			errE := s.Validate()
			if tt.wantErr == "" {
				require.NoError(t, errE, "% -+#.1v", errE)
			} else {
				require.Error(t, errE)
				assert.Contains(t, errE.Error(), tt.wantErr, "% -+#.1v", errE)
			}
		})
	}
}

func TestReadIndex(t *testing.T) {
	t.Parallel()

	s := &site.Site{}
	s.IndexPrefix = "myindex"

	// A caller routes to the index for its resolved visibility level.
	idx, errE := s.ReadIndex(auth.WithVisibility(context.Background(), "public"))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "myindex_public", idx)

	idx, errE = s.ReadIndex(auth.WithVisibility(context.Background(), "editor"))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "myindex_editor", idx)

	// A caller with no visibility level is denied, so the read route returns 403 Forbidden.
	_, errE = s.ReadIndex(context.Background())
	require.Error(t, errE)
	assert.ErrorIs(t, errE, store.ErrAccessDenied)
}

func TestLevelIndexes(t *testing.T) {
	t.Parallel()

	// The configured levels each map to their own index, from lowest to highest.
	s := &site.Site{}
	s.IndexPrefix = "myindex"
	s.Visibility = []auth.VisibilityLevel{
		{Name: "public", Roles: nil},
		{Name: "researcher", Roles: []string{"researcher"}},
		{Name: "editor", Roles: []string{"editor"}},
	}
	assert.Equal(t, []string{"myindex_public", "myindex_researcher", "myindex_editor"}, s.LevelIndexes())
	assert.Equal(t, "myindex_editor", s.TopIndex())

	// A site that configures no levels defaults to a single "all" level that is both floor and top.
	s = &site.Site{}
	s.IndexPrefix = "myindex"
	assert.Equal(t, []string{"myindex_all"}, s.LevelIndexes())
	assert.Equal(t, "myindex_all", s.TopIndex())
}
