package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

func TestRangeFloatValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid gte lt", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		lt := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := 1.0
		lte := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("both gt and gte", func(t *testing.T) {
		t.Parallel()
		gt := 1.0
		gte := 1.0
		lt := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:        &gt,
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "both greater than and greater than or equal are set")
	})

	t.Run("both lt and lte", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		lt := 2.0
		lte := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
			LessThanOrEqual:    &lte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "both less than and less than or equal are set")
	})

	t.Run("no lower bound", func(t *testing.T) {
		t.Parallel()
		lt := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			LessThan: &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})

	t.Run("no upper bound", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "less than bound is required")
	})

	t.Run("empty range", func(t *testing.T) {
		t.Parallel()
		r := internalSearch.RangeFloat{}
		errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})
}

func TestRangeIntValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid gte lt", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		lt := int64(2)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := int64(1)
		lte := int64(2)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("both gt and gte", func(t *testing.T) {
		t.Parallel()
		gt := int64(1)
		gte := int64(1)
		lt := int64(2)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			GreaterThan:        &gt,
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "both greater than and greater than or equal are set")
	})

	t.Run("both lt and lte", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		lt := int64(2)
		lte := int64(2)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
			LessThanOrEqual:    &lte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "both less than and less than or equal are set")
	})

	t.Run("no lower bound", func(t *testing.T) {
		t.Parallel()
		lt := int64(2)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			LessThan: &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})

	t.Run("no upper bound", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		r := internalSearch.RangeInt{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "less than bound is required")
	})

	t.Run("empty range", func(t *testing.T) {
		t.Parallel()
		r := internalSearch.RangeInt{}
		errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})
}
