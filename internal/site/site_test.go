package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/internal/site"
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
		visibility []site.VisibilityLevel
		wantErr    string
	}{
		{
			name:       "nil visibility is valid",
			roles:      roles,
			visibility: nil,
			wantErr:    "",
		},
		{
			name:  "role in no level and level with no roles are both allowed",
			roles: roles,
			visibility: []site.VisibilityLevel{
				{Name: "public", Roles: []string{"public"}},
				{Name: "researcher", Roles: []string{"researcher"}},
				{Name: "editor", Roles: []string{"reviewer", "editor"}},
				{Name: "none", Roles: nil},
				// "admin" is in no level, which is allowed.
			},
			wantErr: "",
		},
		{
			name:  "unknown role",
			roles: roles,
			visibility: []site.VisibilityLevel{
				{Name: "researcher", Roles: []string{"nonexistent"}},
			},
			wantErr: "visibility level references an unknown role",
		},
		{
			name:  "role in more than one level",
			roles: roles,
			visibility: []site.VisibilityLevel{
				{Name: "researcher", Roles: []string{"reviewer"}},
				{Name: "editor", Roles: []string{"reviewer"}},
			},
			wantErr: "role is assigned to more than one visibility level",
		},
		{
			name:  "duplicate level name",
			roles: roles,
			visibility: []site.VisibilityLevel{
				{Name: "dup", Roles: []string{"public"}},
				{Name: "dup", Roles: []string{"researcher"}},
			},
			wantErr: "visibility level name is not unique",
		},
		{
			name:  "empty level name",
			roles: roles,
			visibility: []site.VisibilityLevel{
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

func TestVisibilityForRoles(t *testing.T) {
	t.Parallel()

	levels := []site.VisibilityLevel{
		{Name: "public", Roles: []string{"public"}},
		{Name: "researcher", Roles: []string{"researcher"}},
		{Name: "editor", Roles: []string{"reviewer", "editor"}},
		{Name: "none", Roles: nil},
	}

	tests := []struct {
		name      string
		levels    []site.VisibilityLevel
		roles     []string
		wantName  string
		wantFound bool
	}{
		{"no levels defined", nil, []string{"editor"}, "", false},
		{"no roles", levels, nil, "", false},
		{"role in no level", levels, []string{"admin"}, "", false},
		{"single match", levels, []string{"researcher"}, "researcher", true},
		{"highest among several", levels, []string{"public", "editor"}, "editor", true},
		{"reviewer maps to editor level", levels, []string{"reviewer"}, "editor", true},
		{"lower level when higher role absent", levels, []string{"public", "admin"}, "public", true},
		{"duplicate roles", levels, []string{"researcher", "researcher"}, "researcher", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &site.Site{}
			s.Visibility = tt.levels

			level, found := s.VisibilityForRoles(tt.roles)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantName, level.Name)
		})
	}
}
