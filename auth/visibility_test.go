package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/auth"
)

func TestVisibilityForRoles(t *testing.T) {
	t.Parallel()

	levels := []auth.VisibilityLevel{
		{Name: "public", Roles: []string{"public"}},
		{Name: "researcher", Roles: []string{"researcher"}},
		{Name: "editor", Roles: []string{"reviewer", "editor"}},
	}

	// floorLevels has a no-roles "public" floor as the first (lowest) level,
	// as the site validation requires.
	floorLevels := []auth.VisibilityLevel{
		{Name: "public", Roles: nil},
		{Name: "researcher", Roles: []string{"researcher"}},
		{Name: "editor", Roles: []string{"reviewer", "editor"}},
	}

	tests := []struct {
		name      string
		levels    []auth.VisibilityLevel
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
		// A no-roles floor level is granted to any request.
		{"floor for no roles", floorLevels, nil, "public", true},
		{"floor for unmatched role", floorLevels, []string{"admin"}, "public", true},
		{"role beats floor", floorLevels, []string{"editor"}, "editor", true},
		{"researcher beats floor", floorLevels, []string{"researcher"}, "researcher", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			level, found := auth.VisibilityForRoles(tt.levels, tt.roles)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantName, level.Name)
		})
	}
}
