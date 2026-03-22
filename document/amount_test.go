package document_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

func TestAmountFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		amount    string
		precision float64
		expected  float64
	}{
		// No precision check (precision=0).
		{"integer zero precision", "42", 0, 42},
		{"negative zero precision", "-42", 0, -42},
		{"decimal zero precision", "3.14", 0, 3.14},
		{"comma separator zero precision", "3,14", 0, 3.14},
		{"large integer zero precision", "123456789", 0, 123456789},

		// Precision >= 1 (no decimal digits expected).
		{"integer precision 1", "42", 1, 42},
		{"zero precision 1", "0", 1, 0},
		{"negative precision 1", "-100", 1, -100},
		{"precision 10", "120", 10, 120},
		{"precision 100", "1200", 100, 1200},
		{"precision 1000", "5000", 1000, 5000},
		{"precision 60 seconds", "180", 60, 180},
		{"precision 60 zero", "0", 60, 0},
		{"precision 3600 seconds", "7200", 3600, 7200},
		{"precision 3600 zero", "0", 3600, 0},

		// Precision < 1 (decimal digits expected).
		{"precision 0.1", "3.1", 0.1, 3.1},
		{"precision 0.01", "3.14", 0.01, 3.14},
		{"precision 0.001", "3.142", 0.001, 3.142},
		{"precision 0.5 half", "3.5", 0.5, 3.5},
		{"precision 0.5 whole", "4.0", 0.5, 4.0},
		{"negative with precision 0.01", "-1.50", 0.01, -1.5},
		{"zero with precision 0.01", "0.00", 0.01, 0},

		// Comma as decimal separator with precision.
		{"comma precision 0.01", "3,14", 0.01, 3.14},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, errE := document.Amount(tc.amount).Float64(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.InDelta(t, tc.expected, result, 1e-10)
		})
	}
}

func TestAmountFloat64Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		amount    string
		precision float64
		errMsg    string
	}{
		// Invalid format.
		{"empty string", "", 0, "unable to parse amount"},
		{"letters", "abc", 0, "unable to parse amount"},
		{"spaces", "1 2", 0, "unable to parse amount"},
		{"double dot", "1.2.3", 0, "unable to parse amount"},
		{"trailing dot", "1.", 0, "unable to parse amount"},
		{"leading dot", ".1", 0, "unable to parse amount"},
		{"plus sign", "+1", 0, "unable to parse amount"},

		// Invalid precision values.
		{"negative precision", "1", -1, "precision must be a finite positive number"},
		{"zero negative precision", "1", -0.5, "precision must be a finite positive number"},
		{"infinity precision", "1", math.Inf(1), "precision must be a finite positive number"},
		{"negative infinity precision", "1", math.Inf(-1), "precision must be a finite positive number"},
		{"NaN precision", "1", math.NaN(), "precision must be a finite positive number"},

		// Decimal digit count mismatch.
		{"no decimals when expected", "3", 0.1, "number of decimal digits does not match precision"},
		{"too many decimals", "3.14", 0.1, "number of decimal digits does not match precision"},
		{"too few decimals", "3.1", 0.01, "number of decimal digits does not match precision"},
		{"decimals with integer precision", "3.14", 1, "number of decimal digits does not match precision"},
		{"decimals with precision 10", "3.1", 10, "number of decimal digits does not match precision"},

		// Not rounded to precision.
		{"not rounded to 10", "123", 10, "amount is not rounded to precision"},
		{"not rounded to 100", "150", 100, "amount is not rounded to precision"},
		{"not rounded to 0.5", "3.3", 0.5, "amount is not rounded to precision"},
		{"not rounded to 60", "90", 60, "amount is not rounded to precision"},
		{"not rounded to 3600", "1800", 3600, "amount is not rounded to precision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, errE := document.Amount(tc.amount).Float64(tc.precision)
			assert.EqualError(t, errE, tc.errMsg)
		})
	}
}

func TestAmountValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		errE := document.Amount("42").Validate(1)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("valid no precision", func(t *testing.T) {
		t.Parallel()

		errE := document.Amount("3.14").Validate(0)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("invalid format", func(t *testing.T) {
		t.Parallel()

		errE := document.Amount("abc").Validate(0)
		assert.EqualError(t, errE, "unable to parse amount")
	})

	t.Run("invalid precision", func(t *testing.T) {
		t.Parallel()

		errE := document.Amount("3.14").Validate(1)
		assert.EqualError(t, errE, "number of decimal digits does not match precision")
	})
}

func TestAmountMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   document.Amount
		expected string
	}{
		{"integer", document.Amount("42"), `"42"`},
		{"negative", document.Amount("-42"), `"-42"`},
		{"decimal", document.Amount("3.14"), `"3.14"`},
		{"comma decimal", document.Amount("3,14"), `"3,14"`},
		{"zero", document.Amount("0"), `"0"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, errE := x.MarshalWithoutEscapeHTML(tc.amount)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, []byte(tc.expected), out)
		})
	}
}

func TestAmountUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		json     string
		expected document.Amount
	}{
		{"integer", `"42"`, document.Amount("42")},
		{"negative", `"-42"`, document.Amount("-42")},
		{"decimal", `"3.14"`, document.Amount("3.14")},
		{"comma", `"3,14"`, document.Amount("3,14")},
		{"zero", `"0"`, document.Amount("0")},
		{"large", `"123456789"`, document.Amount("123456789")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var a document.Amount
			errE := x.UnmarshalWithoutUnknownFields([]byte(tc.json), &a)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tc.expected, a)
		})
	}
}

func TestAmountUnmarshalJSONInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		json   string
		errMsg string
	}{
		{"invalid format", `"abc"`, "unable to parse amount"},
		{"empty string", `""`, "unable to parse amount"},
		{"not a string", `42`, "json: cannot unmarshal number into Go value of type string"},
		{"null", `null`, "unable to parse amount"},
		{"leading dot", `".5"`, "unable to parse amount"},
		{"plus sign", `"+1"`, "unable to parse amount"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var a document.Amount
			err := x.UnmarshalWithoutUnknownFields([]byte(tc.json), &a)
			assert.EqualError(t, err, tc.errMsg)
		})
	}
}

func TestAmountMarshalRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []string{"42", "-42", "3.14", "0", "100", "-1.50"}
	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			t.Parallel()

			a := document.Amount(s)
			out, errE := x.MarshalWithoutEscapeHTML(a)
			require.NoError(t, errE, "% -+#.1v", errE)

			var a2 document.Amount
			errE = x.UnmarshalWithoutUnknownFields(out, &a2)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, a, a2)
		})
	}
}

func TestAmountMarshalText(t *testing.T) {
	t.Parallel()

	a := document.Amount("3.14")
	b, err := a.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("3.14"), b)
}

func TestAmountUnmarshalText(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		var a document.Amount
		err := a.UnmarshalText([]byte("3.14"))
		require.NoError(t, err)
		assert.Equal(t, document.Amount("3.14"), a)
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		var a document.Amount
		err := a.UnmarshalText([]byte("abc"))
		assert.EqualError(t, err, "unable to parse amount")
	})
}

func TestAmountString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "42", document.Amount("42").String())
	assert.Equal(t, "-3.14", document.Amount("-3.14").String())
	assert.Equal(t, "3,14", document.Amount("3,14").String())
}

func TestNewAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     float64
		precision float64
		expected  document.Amount
	}{
		// Integer precision.
		{"integer precision 1", 42, 1, document.Amount("42")},
		{"rounding precision 1", 42.7, 1, document.Amount("43")},
		{"precision 10", 123, 10, document.Amount("120")},
		{"precision 100", 1250, 100, document.Amount("1300")},
		{"precision 1000", 1500, 1000, document.Amount("2000")},
		{"precision 60", 90, 60, document.Amount("120")},
		{"precision 60 exact", 180, 60, document.Amount("180")},
		{"precision 60 rounding", 100, 60, document.Amount("120")},
		{"precision 3600", 5000, 3600, document.Amount("3600")},
		{"precision 3600 exact", 7200, 3600, document.Amount("7200")},
		{"precision 3600 rounding", 5400, 3600, document.Amount("7200")},

		// Decimal precision.
		{"precision 0.1", 3.14, 0.1, document.Amount("3.1")},
		{"precision 0.01", 3.14159, 0.01, document.Amount("3.14")},
		{"precision 0.001", 3.14159, 0.001, document.Amount("3.142")},
		{"precision 0.5", 3.3, 0.5, document.Amount("3.5")},
		{"precision 0.5 exact", 4.0, 0.5, document.Amount("4.0")},

		// Negative values.
		{"negative precision 1", -42, 1, document.Amount("-42")},
		{"negative precision 0.01", -3.14159, 0.01, document.Amount("-3.14")},
		{"negative rounding", -3.3, 0.5, document.Amount("-3.5")},

		// Zero.
		{"zero precision 1", 0, 1, document.Amount("0")},
		{"zero precision 0.01", 0, 0.01, document.Amount("0.00")},

		// Negative zero.
		{"negative zero precision 1", math.Copysign(0, -1), 1, document.Amount("0")},
		{"negative zero precision 0.01", math.Copysign(0, -1), 0.01, document.Amount("0.00")},
		{"small negative rounds to zero", -0.001, 0.01, document.Amount("0.00")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := document.NewAmount(tc.value, tc.precision)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNewAmountRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     float64
		precision float64
	}{
		{"integer", 42, 1},
		{"decimal", 3.14, 0.01},
		{"negative", -100, 10},
		{"negative decimal", -3.5, 0.5},
		{"zero", 0, 1},
		{"large", 123456, 1},
		{"small precision", 1.23456, 0.00001},
		{"seconds precision 60", 90, 60},
		{"seconds precision 3600", 5000, 3600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := document.NewAmount(tc.value, tc.precision)

			// Validate passes.
			errE := a.Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)

			// Float64 round-trips.
			result, errE := a.Float64(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)

			expected := math.Round(tc.value/tc.precision) * tc.precision
			assert.InDelta(t, expected, result, 1e-10)
		})
	}
}

func TestNewAmountJSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     float64
		precision float64
	}{
		{"integer", 42, 1},
		{"decimal", 3.14, 0.01},
		{"negative", -50, 10},
		{"zero", 0, 0.001},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := document.NewAmount(tc.value, tc.precision)

			// Marshal to JSON.
			out, errE := x.MarshalWithoutEscapeHTML(a)
			require.NoError(t, errE, "% -+#.1v", errE)

			// Unmarshal from JSON.
			var a2 document.Amount
			errE = x.UnmarshalWithoutUnknownFields(out, &a2)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, a, a2)

			// Float64 still works after round-trip.
			result, errE := a2.Float64(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
			expected := math.Round(tc.value/tc.precision) * tc.precision
			assert.InDelta(t, expected, result, 1e-10)
		})
	}
}
