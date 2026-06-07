package auth

import "slices"

// VisibilityLevel is one entry in the ordered list of visibility levels.
// Each level has a unique, non-empty name and the roles (possibly none) that
// grant it.
type VisibilityLevel struct {
	Name  string   `json:"name"            yaml:"name"`
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`
}

// visibilityForRoles returns the highest visibility level granted by roles, and
// true when at least one role maps to a level. levels is ordered from lowest to
// highest access, so walking it from the end returns the highest matching level
// on the first match. When no role maps to a level (the caller has no such role,
// or there are no levels), it returns the zero VisibilityLevel and false.
func visibilityForRoles(levels []VisibilityLevel, roles []string) (VisibilityLevel, bool) {
	for _, v := range slices.Backward(levels) {
		level := v
		for _, role := range level.Roles {
			if slices.Contains(roles, role) {
				return level, true
			}
		}
	}
	return VisibilityLevel{}, false
}
