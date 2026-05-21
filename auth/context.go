package auth

import (
	"context"
	"slices"
)

// contextKey is used as a value for context keys. Using a pointer keeps it
// distinct without leaking the type to other packages.
type contextKey struct {
	name string
}

// subjectContextKey carries the verified subject (sub claim) of the bearer token.
var subjectContextKey = &contextKey{"subject"} //nolint:gochecknoglobals

// rolesContextKey carries the list of roles granted to the caller.
var rolesContextKey = &contextKey{"roles"} //nolint:gochecknoglobals

// withSubject returns ctx with the given subject attached.
func withSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectContextKey, subject)
}

// withRoles returns ctx with the given roles attached. The slice is stored
// as-is; callers should not retain or mutate it after passing it in.
func withRoles(ctx context.Context, roles []string) context.Context {
	return context.WithValue(ctx, rolesContextKey, roles)
}

// Subject returns the verified subject from ctx, if any.
func Subject(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(subjectContextKey).(string)
	return s, ok
}

// MustSubject returns the verified subject from ctx and panics if no subject is set.
// Use only in handlers that have already gated on Subject being present.
func MustSubject(ctx context.Context) string {
	s, ok := Subject(ctx)
	if !ok {
		panic("auth: subject not present in context")
	}
	return s
}

// Roles returns the roles attached to ctx, or an empty slice if none are present.
// The returned slice must not be modified by callers.
func Roles(ctx context.Context) []string {
	roles, _ := ctx.Value(rolesContextKey).([]string)
	return roles
}

// HasRole reports whether the given role is present in ctx.
func HasRole(ctx context.Context, role string) bool {
	return slices.Contains(Roles(ctx), role)
}
