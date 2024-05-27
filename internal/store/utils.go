package store

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// transactionContextKey contains the existing transaction, if any.
var transactionContextKey = &contextKey{"transaction"} //nolint:gochecknoglobals
