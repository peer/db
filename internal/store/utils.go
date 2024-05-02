package store

import (
	"strings"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// transactionContextKey contains the existing transaction, if any.
var transactionContextKey = &contextKey{"transaction"} //nolint:gochecknoglobals

// See: https://github.com/golang/go/issues/46336
func lastCut(s, sep string) (string, string, bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}
