package search

import "context"

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// conversionStatsContextKey contains a *ConversionStats, if any.
var conversionStatsContextKey = &contextKey{"conversionStats"} //nolint:gochecknoglobals

// WithConversionStats returns a context that accumulates conversion metrics into stats while
// FromDocument runs. The same stats may be reused across calls to accumulate totals.
func WithConversionStats(ctx context.Context, stats *ConversionStats) context.Context {
	return context.WithValue(ctx, conversionStatsContextKey, stats)
}

// conversionStatsFromContext returns the *ConversionStats attached to ctx, or nil if none.
func conversionStatsFromContext(ctx context.Context) *ConversionStats {
	stats, _ := ctx.Value(conversionStatsContextKey).(*ConversionStats)
	return stats
}

// displayDependenciesContextKey contains a *DisplayDependencies, if any.
var displayDependenciesContextKey = &contextKey{"displayDependencies"} //nolint:gochecknoglobals

// withDisplayDependencies returns a context that records, into deps, the documents fetched via
// getDocument while it is in effect. It is set around makeDisplayStrings.
func withDisplayDependencies(ctx context.Context, deps *DisplayDependencies) context.Context {
	return context.WithValue(ctx, displayDependenciesContextKey, deps)
}

func displayDependenciesFromContext(ctx context.Context) *DisplayDependencies {
	deps, _ := ctx.Value(displayDependenciesContextKey).(*DisplayDependencies)
	return deps
}
