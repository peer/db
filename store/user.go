package store

import (
	"context"

	"gitlab.com/peerdb/peerdb/auth"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// User identifies a user. ID is the auth subject string.
type User = internalStore.User

// UserFromContext returns the auth subject in ctx wrapped as a *User, or nil
// when no subject is present (unauthenticated caller).
func UserFromContext(ctx context.Context) *User {
	sub, ok := auth.Subject(ctx)
	if !ok {
		return nil
	}
	return &User{ID: sub}
}
