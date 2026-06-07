package auth

import "slices"

// VisibilityLevel is one entry in the ordered list of visibility levels.
// Each level has a unique, non-empty name and the roles (possibly none) that
// grant it.
type VisibilityLevel struct {
	Name  string   `json:"name"            yaml:"name"`
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`
}

// visibilityForRoles returns the visibility level granted to a caller with the
// given roles, and true when some level applies. levels is ordered from lowest
// to highest access, so walking it from the end returns the highest applicable
// level on the first match. A level applies when the caller holds one of its
// roles, or when the level has no roles at all: a no-roles level is the floor,
// granted to every request (including one with no roles). When no level applies
// it returns the zero VisibilityLevel and false.
func visibilityForRoles(levels []VisibilityLevel, roles []string) (VisibilityLevel, bool) {
	for i, level := range slices.Backward(levels) {
		// A level with no roles is the floor, granted to every request.
		// Just in case we check that this is really the floor.
		if len(level.Roles) == 0 && i == 0 {
			return level, true
		}
		for _, role := range level.Roles {
			if slices.Contains(roles, role) {
				return level, true
			}
		}
	}
	return VisibilityLevel{}, false
}
