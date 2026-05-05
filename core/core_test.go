//nolint:exhaustruct
package core_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/transform"
)

func TestClasses(t *testing.T) {
	t.Parallel()

	// Build mnemonics from properties.
	props, errE := core.Properties()
	require.NoError(t, errE, "% -+#.1v", errE)
	allDocs := make([]any, 0, len(props))
	allDocs = append(allDocs, props...)
	mnemonics, errE := transform.Mnemonics(t.Context(), allDocs)
	require.NoError(t, errE, "% -+#.1v", errE)

	docs, errE := core.Classes(mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, docs)
}

func TestProperties(t *testing.T) {
	t.Parallel()

	docs, errE := core.Properties()
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, docs)
}

func TestVocabularies(t *testing.T) {
	t.Parallel()

	docs, errE := core.Vocabularies()
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
	assert.EqualError(t, errE, "unknown precision")

	errE = core.Time{Precision: document.TimePrecisionGigaYears - 1}.Validate()
	assert.EqualError(t, errE, "unknown precision")
}

func TestAmountValidateFloat32(t *testing.T) {
	t.Parallel()

	// Valid: finite values.
	errE := core.Amount[float32]{Amount: 1.0, Precision: 0.1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: infinite amount.
	errE = core.Amount[float32]{Amount: float32(math.Inf(1)), Precision: 0.1}.Validate()
	assert.EqualError(t, errE, "amount must be a finite number")

	// Invalid: NaN amount.
	errE = core.Amount[float32]{Amount: float32(math.NaN()), Precision: 0.1}.Validate()
	assert.EqualError(t, errE, "amount must be a finite number")

	// Invalid: infinite precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: float32(math.Inf(-1))}.Validate()
	assert.EqualError(t, errE, "precision must be a finite number")

	// Invalid: NaN precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: float32(math.NaN())}.Validate()
	assert.EqualError(t, errE, "precision must be a finite number")

	// Invalid: negative precision.
	errE = core.Amount[float32]{Amount: 1.0, Precision: -0.1}.Validate()
	assert.EqualError(t, errE, "precision must be positive")
}

func TestAmountValidateFloat64(t *testing.T) {
	t.Parallel()

	// Valid: finite values.
	errE := core.Amount[float64]{Amount: 1.0, Precision: 0.1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: infinite amount.
	errE = core.Amount[float64]{Amount: math.Inf(1), Precision: 0.1}.Validate()
	assert.EqualError(t, errE, "amount must be a finite number")

	// Invalid: NaN amount.
	errE = core.Amount[float64]{Amount: math.NaN(), Precision: 0.1}.Validate()
	assert.EqualError(t, errE, "amount must be a finite number")

	// Invalid: infinite precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: math.Inf(-1)}.Validate()
	assert.EqualError(t, errE, "precision must be a finite number")

	// Invalid: NaN precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: math.NaN()}.Validate()
	assert.EqualError(t, errE, "precision must be a finite number")

	// Invalid: negative precision.
	errE = core.Amount[float64]{Amount: 1.0, Precision: -0.1}.Validate()
	assert.EqualError(t, errE, "precision must be positive")
}

func TestAmountValidateInt(t *testing.T) {
	t.Parallel()

	// Integer amounts are always valid.
	errE := core.Amount[int]{Amount: 42, Precision: 1}.Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: negative precision.
	errE = core.Amount[int]{Amount: 42, Precision: -1}.Validate()
	assert.EqualError(t, errE, "precision must be positive")
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
	errE = (&core.Interval[core.Amount[int]]{ToIsOpen: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{ToIsUnknown: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	errE = (&core.Interval[core.Amount[int]]{ToIsNone: true}).Validate()
	assert.NoError(t, errE, "% -+#.1v", errE) //nolint:testifylint

	// Invalid: multiple FromIs* set simultaneously.
	errE = (&core.Interval[core.Amount[int]]{FromIsOpen: true, FromIsUnknown: true}).Validate()
	assert.EqualError(t, errE, "only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")

	errE = (&core.Interval[core.Amount[int]]{FromIsOpen: true, FromIsNone: true}).Validate()
	assert.EqualError(t, errE, "only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")

	errE = (&core.Interval[core.Amount[int]]{FromIsUnknown: true, FromIsNone: true}).Validate()
	assert.EqualError(t, errE, "only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")

	// Invalid: From set with FromIsUnknown or FromIsNone.
	errE = (&core.Interval[core.Amount[int]]{From: &from, FromIsUnknown: true}).Validate()
	assert.EqualError(t, errE, "From must not be set when FromIsUnknown or FromIsNone is true")

	errE = (&core.Interval[core.Amount[int]]{From: &from, FromIsNone: true}).Validate()
	assert.EqualError(t, errE, "From must not be set when FromIsUnknown or FromIsNone is true")

	// Invalid: multiple ToIs* set simultaneously.
	errE = (&core.Interval[core.Amount[int]]{ToIsOpen: true, ToIsUnknown: true}).Validate()
	assert.EqualError(t, errE, "only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")

	errE = (&core.Interval[core.Amount[int]]{ToIsOpen: true, ToIsNone: true}).Validate()
	assert.EqualError(t, errE, "only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")

	errE = (&core.Interval[core.Amount[int]]{ToIsUnknown: true, ToIsNone: true}).Validate()
	assert.EqualError(t, errE, "only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")

	// Invalid: To set with ToIsUnknown or ToIsNone.
	errE = (&core.Interval[core.Amount[int]]{To: &to, ToIsUnknown: true}).Validate()
	assert.EqualError(t, errE, "To must not be set when ToIsUnknown or ToIsNone is true")

	errE = (&core.Interval[core.Amount[int]]{To: &to, ToIsNone: true}).Validate()
	assert.EqualError(t, errE, "To must not be set when ToIsUnknown or ToIsNone is true")

	// Bound validation is propagated for From.
	invalidFrom := core.Amount[float64]{Amount: math.Inf(1)}
	errE = (&core.Interval[core.Amount[float64]]{From: &invalidFrom}).Validate()
	assert.EqualError(t, errE, "amount must be a finite number")

	// Bound validation is propagated for To.
	invalidTo := core.Amount[float64]{Amount: math.NaN()}
	errE = (&core.Interval[core.Amount[float64]]{To: &invalidTo}).Validate()
	assert.EqualError(t, errE, "amount must be a finite number")
}
