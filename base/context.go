package base

import "context"

// contextKey is used as a value for context keys. Using a pointer keeps it
// distinct without leaking the type to other packages.
type contextKey struct {
	name string
}

// systemSessionContextKey marks a context used by the application itself to end an edit session.
var systemSessionContextKey = &contextKey{"systemSession"} //nolint:gochecknoglobals

// WithSystemSession returns ctx marked so that operating on an edit session with it skips the
// permission checks (ChangePermissionCheck when appending changes and EndEditPermissionCheck at
// completion): the session is driven by the application itself and not on behalf of the caller.
func WithSystemSession(ctx context.Context) context.Context {
	return context.WithValue(ctx, systemSessionContextKey, true)
}

// isSystemSession returns true when ctx is marked with WithSystemSession.
func isSystemSession(ctx context.Context) bool {
	is, _ := ctx.Value(systemSessionContextKey).(bool)
	return is
}
