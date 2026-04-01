package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		swapped, errE := r.Validate()
		assert.NoError(t, errE) //nolint:testifylint
		assert.False(t, swapped)
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := 1.0
		lte := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		swapped, errE := r.Validate()
		assert.NoError(t, errE) //nolint:testifylint
		assert.False(t, swapped)
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
		_, errE := r.Validate()
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
		_, errE := r.Validate()
		assert.EqualError(t, errE, "both less than and less than or equal are set")
	})

	t.Run("no lower bound", func(t *testing.T) {
		t.Parallel()
		lt := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			LessThan: &lt,
		}
		_, errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})

	t.Run("no upper bound", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
		}
		_, errE := r.Validate()
		assert.EqualError(t, errE, "less than bound is required")
	})

	t.Run("empty range", func(t *testing.T) {
		t.Parallel()
		r := internalSearch.RangeFloat{}
		_, errE := r.Validate()
		assert.EqualError(t, errE, "greater than bound is required")
	})

	t.Run("equal bounds both closed", func(t *testing.T) {
		t.Parallel()
		gte := 5.0
		lte := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThanOrEqual:    &lte,
		}
		_, errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is equal to upper bound")
	})

	t.Run("equal bounds gte lt", func(t *testing.T) {
		t.Parallel()
		gte := 5.0
		lt := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		_, errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is equal to upper bound")
	})

	t.Run("inverted gte lt swaps to lte gt", func(t *testing.T) {
		t.Parallel()
		gte := 3.0
		lt := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		swapped, errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, swapped)
		assert.Nil(t, r.GreaterThanOrEqual)
		assert.Nil(t, r.LessThan)
		assert.Equal(t, 1.0, *r.GreaterThan)     //nolint:testifylint
		assert.Equal(t, 3.0, *r.LessThanOrEqual) //nolint:testifylint
	})

	t.Run("inverted gt lte swaps to lt gte", func(t *testing.T) {
		t.Parallel()
		gt := 3.0
		lte := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		swapped, errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, swapped)
		assert.Nil(t, r.GreaterThan)
		assert.Nil(t, r.LessThanOrEqual)
		assert.Equal(t, 1.0, *r.GreaterThanOrEqual) //nolint:testifylint
		assert.Equal(t, 3.0, *r.LessThan)           //nolint:testifylint
	})

	t.Run("inverted gt lt swaps to gt lt", func(t *testing.T) {
		t.Parallel()
		gt := 3.0
		lt := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		swapped, errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, swapped)
		assert.Equal(t, 1.0, *r.GreaterThan) //nolint:testifylint
		assert.Equal(t, 3.0, *r.LessThan)    //nolint:testifylint
	})

	t.Run("floating point precision issue", func(t *testing.T) {
		t.Parallel()
		// Simulating the exact error from the bug report:
		// min value (7.652448E8) > max value (7.652447999999999E8).
		gte := 7.652448e8
		lt := 7.652447999999999e8
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		swapped, errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, swapped)
		// After swap, lt becomes GreaterThan and gte becomes LessThanOrEqual.
		assert.Equal(t, 7.652447999999999e8, *r.GreaterThan) //nolint:testifylint
		assert.Equal(t, 7.652448e8, *r.LessThanOrEqual)      //nolint:testifylint
	})
}
