package search_test

import (
	"math"
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
		errE := r.Validate()
		assert.NoError(t, errE) //nolint:testifylint
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := 1.0
		lte := 2.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		errE := r.Validate()
		assert.NoError(t, errE) //nolint:testifylint
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

	t.Run("equal bounds both closed (gte lte) is accepted (single-point range)", func(t *testing.T) {
		t.Parallel()
		gte := 5.0
		lte := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThanOrEqual:    &lte,
		}
		errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
		// Bounds unchanged.
		require.NotNil(t, r.GreaterThanOrEqual)
		require.NotNil(t, r.LessThanOrEqual)
		assert.Equal(t, 5.0, *r.GreaterThanOrEqual)  //nolint:testifylint
		assert.Equal(t, 5.0, *r.LessThanOrEqual)     //nolint:testifylint
	})

	t.Run("equal bounds gte lt is rejected", func(t *testing.T) {
		t.Parallel()
		gte := 5.0
		lt := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "equal bounds with at least one strict bound")
	})

	t.Run("equal bounds gt lte is rejected", func(t *testing.T) {
		t.Parallel()
		gt := 5.0
		lte := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "equal bounds with at least one strict bound")
	})

	t.Run("equal bounds gt lt is rejected", func(t *testing.T) {
		t.Parallel()
		gt := 5.0
		lt := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "equal bounds with at least one strict bound")
	})

	t.Run("inverted gte lt is rejected", func(t *testing.T) {
		t.Parallel()
		gte := 3.0
		lt := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is greater than upper bound")
	})

	t.Run("inverted gt lte is rejected", func(t *testing.T) {
		t.Parallel()
		gt := 3.0
		lte := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan:     &gt,
			LessThanOrEqual: &lte,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is greater than upper bound")
	})

	t.Run("inverted gt lt is rejected", func(t *testing.T) {
		t.Parallel()
		gt := 3.0
		lt := 1.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is greater than upper bound")
	})

	t.Run("floating point precision issue is rejected", func(t *testing.T) {
		t.Parallel()
		// Simulating the exact error from the bug report:
		// min value (7.652448E8) > max value (7.652447999999999E8).
		gte := 7.652448e8
		lt := 7.652447999999999e8
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is greater than upper bound")
	})

	t.Run("NaN lower is rejected", func(t *testing.T) {
		t.Parallel()
		gte := math.NaN()
		lt := 10.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is not a finite number")
	})

	t.Run("NaN upper is rejected", func(t *testing.T) {
		t.Parallel()
		gte := 0.0
		lt := math.NaN()
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "upper bound is not a finite number")
	})

	t.Run("negative infinity lower is rejected", func(t *testing.T) {
		t.Parallel()
		gte := math.Inf(-1)
		lt := 10.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is not a finite number")
	})

	t.Run("positive infinity upper is rejected", func(t *testing.T) {
		t.Parallel()
		gte := 0.0
		lt := math.Inf(1)
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "upper bound is not a finite number")
	})

	t.Run("strict bounds within one ULP are rejected", func(t *testing.T) {
		t.Parallel()
		// gt: 5.0, lt: 5.0 advanced by one ULP - strict bounds 1 ULP apart.
		gt := 5.0
		lt := math.Nextafter(5.0, math.Inf(1))
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "strict bounds within one ULP of each other")
	})

	t.Run("strict bounds two ULPs apart are accepted", func(t *testing.T) {
		t.Parallel()
		gt := 5.0
		lt := math.Nextafter(math.Nextafter(5.0, math.Inf(1)), math.Inf(1))
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		errE := r.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("inverted strict bounds are rejected", func(t *testing.T) {
		t.Parallel()
		gt := math.Nextafter(5.0, math.Inf(1))
		lt := 5.0
		r := internalSearch.RangeFloat{ //nolint:exhaustruct
			GreaterThan: &gt,
			LessThan:    &lt,
		}
		errE := r.Validate()
		assert.EqualError(t, errE, "lower bound is greater than upper bound")
	})
}
