package site

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		visibility []VisibilityLevel
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
			visibility: []VisibilityLevel{
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
			visibility: []VisibilityLevel{
				{Name: "researcher", Roles: []string{"nonexistent"}},
			},
			wantErr: "visibility level references an unknown role",
		},
		{
			name:  "role in more than one level",
			roles: roles,
			visibility: []VisibilityLevel{
				{Name: "researcher", Roles: []string{"reviewer"}},
				{Name: "editor", Roles: []string{"reviewer"}},
			},
			wantErr: "role is assigned to more than one visibility level",
		},
		{
			name:  "duplicate level name",
			roles: roles,
			visibility: []VisibilityLevel{
				{Name: "dup", Roles: []string{"public"}},
				{Name: "dup", Roles: []string{"researcher"}},
			},
			wantErr: "visibility level name is not unique",
		},
		{
			name:  "empty level name",
			roles: roles,
			visibility: []VisibilityLevel{
				{Name: "", Roles: []string{"public"}},
			},
			wantErr: "visibility level has an empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &Site{}
			s.Roles = tt.roles
			s.Visibility = tt.visibility

			errE := s.validateVisibility()
			if tt.wantErr == "" {
				require.NoError(t, errE)
			} else {
				require.Error(t, errE)
				assert.Contains(t, errE.Error(), tt.wantErr)
			}
		})
	}
}
