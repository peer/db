//nolint:exhaustruct
package core_test

import (
	"math"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
)

func TestClasses(t *testing.T) {
	t.Parallel()

	docs, errE := core.Classes(zerolog.Nop())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, docs)
}

func TestProperties(t *testing.T) {
	t.Parallel()

	docs, errE := core.Properties(zerolog.Nop())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, docs)
}

func TestVocabularies(t *testing.T) {
	t.Parallel()

	docs, errE := core.Vocabularies(zerolog.Nop())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, docs)
}

func TestTimeValidate(t *testing.T) {
	t.Parallel()

	// Valid: boundary precision values.
	errE := core.Time{Precision: document.TimePrecisionGigaYears}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = core.Time{Precision: document.TimePrecisionYear}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = core.Time{Precision: document.TimePrecisionNanosecond}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: out-of-range precision.
	errE = core.Time{Precision: document.TimePrecisionNanosecond + 1}.Validate()
	assert.Error(t, errE)

	errE = core.Time{Precision: document.TimePrecisionGigaYears - 1}.Validate()
	assert.Error(t, errE)
}

func TestAmountValidateFloat32(t *testing.T) {
	t.Parallel()

	// Valid: finite values.
	errE := core.Amount[float32]{Amount: 1.0, Precision: 0.1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: infinite amount.
	errE = core.Amount[float32]{Amount: float32(math.Inf(1)), Precision: 0.1}.Validate()
	assert.Error(t, errE)

	// Invalid: NaN amount.
	errE = core.Amount[float32]{Amount: float32(math.NaN()), Precision: 0.1}.Validate()
	assert.Error(t, errE)

	// Invalid: infinite precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: float32(math.Inf(-1))}.Validate()
	assert.Error(t, errE)

	// Invalid: NaN precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: float32(math.NaN())}.Validate()
	assert.Error(t, errE)

	// Invalid: negative precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: -0.1}.Validate()
	assert.EqualError(t, errE, "Precision must be positive")
}

func TestAmountValidateFloat64(t *testing.T) {
	t.Parallel()

	// Valid: finite values.
	errE := core.Amount[float64]{Amount: 1.0, Precision: 0.1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: infinite amount.
	errE = core.Amount[float64]{Amount: math.Inf(1), Precision: 0.1}.Validate()
	assert.Error(t, errE)

	// Invalid: NaN amount.
	errE = core.Amount[float64]{Amount: math.NaN(), Precision: 0.1}.Validate()
	assert.Error(t, errE)

	// Invalid: infinite precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: math.Inf(-1)}.Validate()
	assert.Error(t, errE)

	// Invalid: NaN precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: math.NaN()}.Validate()
	assert.Error(t, errE)

	// Invalid: negative precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: -0.1}.Validate()
	assert.EqualError(t, errE, "Precision must be positive")
}

func TestAmountValidateInt(t *testing.T) {
	t.Parallel()

	// Integer amounts are always valid.
	errE := core.Amount[int]{Amount: 42, Precision: 1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: negative precision.
	errE = core.Amount[int]{Amount: 42, Precision: -1}.Validate()
	assert.EqualError(t, errE, "Precision must be positive")
}

func TestIntervalValidate(t *testing.T) {
	t.Parallel()

	from := core.Amount[int]{Amount: 1, Precision: 1}
	to := core.Amount[int]{Amount: 10, Precision: 1}

	// Valid: empty interval.
	errE := (&core.Interval[core.Amount[int]]{}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Valid: interval with both bounds.
	errE = (&core.Interval[core.Amount[int]]{From: &from, To: &to}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Valid: each From flag individually.
	errE = (&core.Interval[core.Amount[int]]{FromIsOpen: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{FromIsUnknown: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{FromIsNone: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Valid: each To flag individually.
	errE = (&core.Interval[core.Amount[int]]{ToIsClosed: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{ToIsUnknown: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{ToIsNone: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: multiple FromIs* set simultaneously.
	errE = (&core.Interval[core.Amount[int]]{FromIsOpen: true, FromIsUnknown: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{FromIsOpen: true, FromIsNone: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{FromIsUnknown: true, FromIsNone: true}).Validate()
	assert.Error(t, errE)

	// Invalid: From set with FromIsUnknown or FromIsNone.
	errE = (&core.Interval[core.Amount[int]]{From: &from, FromIsUnknown: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{From: &from, FromIsNone: true}).Validate()
	assert.Error(t, errE)

	// Invalid: multiple ToIs* set simultaneously.
	errE = (&core.Interval[core.Amount[int]]{ToIsClosed: true, ToIsUnknown: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{ToIsClosed: true, ToIsNone: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{ToIsUnknown: true, ToIsNone: true}).Validate()
	assert.Error(t, errE)

	// Invalid: To set with ToIsUnknown or ToIsNone.
	errE = (&core.Interval[core.Amount[int]]{To: &to, ToIsUnknown: true}).Validate()
	assert.Error(t, errE)

	errE = (&core.Interval[core.Amount[int]]{To: &to, ToIsNone: true}).Validate()
	assert.Error(t, errE)

	// Bound validation is propagated for From.
	invalidFrom := core.Amount[float64]{Amount: math.Inf(1)}
	errE = (&core.Interval[core.Amount[float64]]{From: &invalidFrom}).Validate()
	assert.Error(t, errE)

	// Bound validation is propagated for To.
	invalidTo := core.Amount[float64]{Amount: math.NaN()}
	errE = (&core.Interval[core.Amount[float64]]{To: &invalidTo}).Validate()
	assert.Error(t, errE)
}
