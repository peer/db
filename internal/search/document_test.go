package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/internal/search"
)

func TestRangeFloatValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid gte lt", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		lt := 2.0
		r := search.RangeFloat{
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := 1.0
		lte := 2.0
		r := search.RangeFloat{
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
		r := search.RangeFloat{
			GreaterThan:        &gt,
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "both greater than and greater than or equal")
	})

	t.Run("both lt and lte", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		lt := 2.0
		lte := 2.0
		r := search.RangeFloat{
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
			LessThanOrEqual:    &lte,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "both less than and less than or equal")
	})

	t.Run("no lower bound", func(t *testing.T) {
		t.Parallel()
		lt := 2.0
		r := search.RangeFloat{ //nolint:exhaustruct
			LessThan: &lt,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "greater than bound is required")
	})

	t.Run("no upper bound", func(t *testing.T) {
		t.Parallel()
		gte := 1.0
		r := search.RangeFloat{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "less than bound is required")
	})

	t.Run("empty range", func(t *testing.T) {
		t.Parallel()
		r := search.RangeFloat{} //nolint:exhaustruct
		errE := r.Validate()
		assert.Error(t, errE)
	})
}

func TestRangeIntValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid gte lt", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		lt := int64(2)
		r := search.RangeInt{
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("valid gt lte", func(t *testing.T) {
		t.Parallel()
		gt := int64(1)
		lte := int64(2)
		r := search.RangeInt{
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
		r := search.RangeInt{
			GreaterThan:        &gt,
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "both greater than and greater than or equal")
	})

	t.Run("both lt and lte", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		lt := int64(2)
		lte := int64(2)
		r := search.RangeInt{
			GreaterThanOrEqual: &gte,
			LessThan:           &lt,
			LessThanOrEqual:    &lte,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "both less than and less than or equal")
	})

	t.Run("no lower bound", func(t *testing.T) {
		t.Parallel()
		lt := int64(2)
		r := search.RangeInt{ //nolint:exhaustruct
			LessThan: &lt,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "greater than bound is required")
	})

	t.Run("no upper bound", func(t *testing.T) {
		t.Parallel()
		gte := int64(1)
		r := search.RangeInt{ //nolint:exhaustruct
			GreaterThanOrEqual: &gte,
		}
		errE := r.Validate()
		assert.Error(t, errE)
		assert.Contains(t, errE.Error(), "less than bound is required")
	})

	t.Run("empty range", func(t *testing.T) {
		t.Parallel()
		r := search.RangeInt{} //nolint:exhaustruct
		errE := r.Validate()
		assert.Error(t, errE)
	})
}
