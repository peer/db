package search_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/search"
)

// prefilterSession returns a session carrying only the given prefilters. PrefilterExcludeFullPaths reads
// nothing else off the session.
func prefilterSession(prefilters []search.Filter) *search.Session {
	return &search.Session{ //nolint:exhaustruct
		SessionData: search.SessionData{ //nolint:exhaustruct
			Prefilters: prefilters,
		},
	}
}

// newFilterID returns a pointer to a fresh filter ID.
func newFilterID() *identifier.Identifier {
	id := identifier.New()
	return &id
}

func TestPrefilterExcludeFullPathsRef(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	v1 := identifier.New()
	v2 := identifier.New()
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		switch id {
		case v1:
			return []string{"h:r/v1"}, nil
		case v2:
			return []string{"h:r/v2"}, nil
		default:
			return nil, nil
		}
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: v1}, {ID: v2}}, Direct: nil, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []string{"h:r/v1", "h:r/v2"}, excludes.Ref(prop))
	// A property with no prefilter, and any sub-reference facet, yield nothing.
	assert.Nil(t, excludes.Ref(identifier.New()))
	assert.Nil(t, excludes.SubRef(prop, prop))
}

func TestPrefilterExcludeFullPathsSubRef(t *testing.T) {
	t.Parallel()

	parentProp := identifier.New()
	prop := identifier.New()
	value := identifier.New()
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		if id == value {
			return []string{"h:s/value"}, nil
		}
		return nil, nil
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{parentProp, prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{"h:s/value"}, excludes.SubRef(parentProp, prop))
	// The same property as a top-level reference facet, and the reversed key, are unaffected.
	assert.Nil(t, excludes.Ref(prop))
	assert.Nil(t, excludes.SubRef(prop, parentProp))
}

func TestPrefilterExcludeFullPathsToAndDirectDeduplicated(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()
	calls := 0
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		calls++
		if id == value {
			return []string{"h:d/a", "h:d/b"}, nil
		}
		return nil, nil
	}
	// The same value appears as both a To and a Direct value, and resolves to two paths.
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: []search.ToValue{{ID: value}}, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	// The value is resolved once per occurrence (To and Direct), but each path is kept only once.
	assert.Equal(t, 2, calls)
	assert.Equal(t, []string{"h:d/a", "h:d/b"}, excludes.Ref(prop))
}

func TestPrefilterExcludeFullPathsSkipsNonRefAndPathless(t *testing.T) {
	t.Parallel()

	hasProp := identifier.New()
	rootProp := identifier.New()
	rootValue := identifier.New()
	resolve := func(_ context.Context, _ identifier.Identifier) ([]string, errors.E) {
		// Every value is a hierarchy root here, so it resolves to no path.
		return nil, nil
	}
	session := prefilterSession([]search.Filter{
		{ //nolint:exhaustruct
			// A non-reference prefilter creates no hierarchy redundancy and is ignored.
			ID:   newFilterID(),
			Prop: []identifier.Identifier{hasProp},
			Has:  &search.HasFilter{Props: nil},
		},
		{ //nolint:exhaustruct
			// A reference prefilter whose value has no path contributes nothing.
			ID:   newFilterID(),
			Prop: []identifier.Identifier{rootProp},
			Ref:  &search.RefFilter{To: []search.ToValue{{ID: rootValue}}, Direct: nil, Missing: false},
		},
	})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, excludes.Ref(hasProp))
	assert.Nil(t, excludes.Ref(rootProp))
}

func TestPrefilterExcludeFullPathsResolverError(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()
	filterID := newFilterID()
	sentinel := errors.Base("resolve failed")
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		if id == value {
			return nil, errors.WithStack(sentinel)
		}
		return nil, nil
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   filterID,
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false},
	}})

	_, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.Error(t, errE)
	assert.ErrorIs(t, errE, sentinel)
	// The error carries the value that failed to resolve and the prefilter it came from.
	assert.Equal(t, value.String(), errors.Details(errE)["id"])
	assert.Equal(t, filterID.String(), errors.Details(errE)["filter"])
}
