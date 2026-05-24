package store

import (
	"slices"
)

// User identifies a user. ID is the auth subject string.
type User struct {
	ID string `json:"id"`
}

// SortedUniqueUsers returns users deduped by ID and sorted ascending by ID.
// nil entries are skipped, so callers can pass slices that may contain
// unauthenticated (nil) participants without filtering first.
func SortedUniqueUsers(users []*User) []User {
	seen := make(map[string]bool, len(users))
	out := make([]User, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		if seen[u.ID] {
			continue
		}
		seen[u.ID] = true
		out = append(out, *u)
	}
	slices.SortFunc(out, func(a, b User) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if len(out) == 0 {
		return nil
	}
	return out
}
