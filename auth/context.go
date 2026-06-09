package auth

import (
	"context"
	"slices"

	"github.com/rs/zerolog"
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

// visibilityContextKey carries the caller's resolved visibility level.
var visibilityContextKey = &contextKey{"visibility"} //nolint:gochecknoglobals

// WithSubject returns ctx with the given subject attached, and adds a "subject" field with it to the
// context logger so any log emitted through it identifies the caller. Called by the auth middleware after
// token verification.
func WithSubject(ctx context.Context, subject string) context.Context {
	ctx = context.WithValue(ctx, subjectContextKey, subject)
	return zerolog.Ctx(ctx).With().Str("subject", subject).Logger().WithContext(ctx)
}

// WithRoles returns ctx with the given roles attached, and adds a "roles" field with them to the context
// logger. The slice is stored as-is. Callers should not retain or mutate it after passing it in.
func WithRoles(ctx context.Context, roles []string) context.Context {
	ctx = context.WithValue(ctx, rolesContextKey, roles)
	return zerolog.Ctx(ctx).With().Strs("roles", roles).Logger().WithContext(ctx)
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

// WithVisibility returns ctx with the caller's resolved visibility level name attached, and adds a
// "visibility" field with that level to the context logger so any log emitted through it identifies
// the visibility level it ran at.
func WithVisibility(ctx context.Context, level string) context.Context {
	ctx = context.WithValue(ctx, visibilityContextKey, level)
	return zerolog.Ctx(ctx).With().Str("visibility", level).Logger().WithContext(ctx)
}

// Visibility returns the caller's resolved visibility level name from ctx, or
// "" when none was set. A configured level name is never empty (site validation
// rejects empty names), so "" unambiguously means no level is attached.
func Visibility(ctx context.Context) string {
	level, _ := ctx.Value(visibilityContextKey).(string)
	return level
}
