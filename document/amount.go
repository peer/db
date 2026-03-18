package document

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// Amount represents a numeric amount.
//
// It generally operates with an additional piece of information
// which is not part of the amount itself:
//
//   - precision: the rounding precision of the amount
//
// It is represented as a string to preserve the original format
// as provided by the user. The format is a decimal number with
// an optional sign and an optional decimal part separated by
// a dot or comma.
//
//nolint:recvcheck
type Amount string

var amountRegex = regexp.MustCompile(`^(-?\d+)(?:[.,](\d+))?$`)

// Float64 returns the float64 representation of an Amount,
// rounded to the given precision.
//
// Passing 0 for precision skips checks for precision.
func (a Amount) Float64(precision float64) (float64, errors.E) {
	s := string(a)
	match := amountRegex.FindStringSubmatch(s)
	if match == nil {
		errE := errors.New("unable to parse amount")
		errors.Details(errE)["value"] = s
		return 0, errE
	}

	// Build canonical decimal string with dot separator.
	numStr := match[1]
	if match[2] != "" {
		numStr += "." + match[2]
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		errE := errors.New("unable to parse amount as float64")
		errors.Details(errE)["value"] = s
		return 0, errE
	}

	if math.IsInf(value, 0) || math.IsNaN(value) {
		errE := errors.New("amount must be a finite number")
		errors.Details(errE)["value"] = s
		return 0, errE
	}

	if precision != 0 {
		if math.IsInf(precision, 0) || math.IsNaN(precision) || precision <= 0 {
			errE := errors.New("precision must be a finite positive number")
			errors.Details(errE)["value"] = s
			errors.Details(errE)["precision"] = precision
			return 0, errE
		}

		// Check number of decimal digits matches ceil(-log10(precision)).
		expectedDecimals := 0
		if precision < 1 {
			expectedDecimals = int(math.Ceil(-math.Log10(precision)))
		}
		actualDecimals := len(match[2])
		if actualDecimals != expectedDecimals {
			errE := errors.New("number of decimal digits does not match precision")
			errors.Details(errE)["value"] = s
			errors.Details(errE)["precision"] = precision
			errors.Details(errE)["expected"] = expectedDecimals
			errors.Details(errE)["got"] = actualDecimals
			return 0, errE
		}

		// Check that the value is rounded to the given precision.
		rounded := math.Round(value/precision) * precision
		// Format both to the same number of decimal digits for comparison.
		v := fmt.Sprintf("%.*f", expectedDecimals, value)
		r := fmt.Sprintf("%.*f", expectedDecimals, rounded)
		if v != r {
			errE := errors.New("amount is not rounded to precision")
			errors.Details(errE)["value"] = s
			errors.Details(errE)["precision"] = precision
			errors.Details(errE)["parsed"] = v
			errors.Details(errE)["rounded"] = r
			return 0, errE
		}

		value = rounded
	}

	return value, nil
}

// Validate checks if the amount is valid for the given precision and returns an error if it is not.
//
// Passing 0 for precision skips checks for precision and just checks the format.
func (a Amount) Validate(precision float64) errors.E {
	_, errE := a.Float64(precision)
	return errE
}

// MarshalText implements encoding.TextMarshaler for Amount.
func (a Amount) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

// MarshalJSON implements json.Marshaler for Amount.
func (a Amount) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(a.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

// String returns the string representation of Amount.
func (a Amount) String() string {
	return string(a)
}

// UnmarshalText implements encoding.TextUnmarshaler for Amount.
func (a *Amount) UnmarshalText(text []byte) error {
	amount := Amount(text)

	// We use special value 0 for precision which skips checking precision.
	errE := amount.Validate(0)
	if errE != nil {
		return errE
	}

	*a = amount

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for Amount.
func (a *Amount) UnmarshalJSON(data []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(data, &s)
	if errE != nil {
		return errE
	}
	return a.UnmarshalText([]byte(s))
}

// NewAmount formats a float64 into an Amount string
// rounded to the given precision.
func NewAmount(value, precision float64) Amount {
	rounded := math.Round(value/precision) * precision
	decimals := 0
	if precision < 1 {
		decimals = int(math.Ceil(-math.Log10(precision)))
	}
	s := fmt.Sprintf("%.*f", decimals, rounded)
	// Remove negative zero.
	if rounded == 0 {
		s = strings.TrimPrefix(s, "-")
	}
	return Amount(s)
}
