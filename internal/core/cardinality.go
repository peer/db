package core

import (
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
)

// ParseCardinalityTag parses a cardinality tag string and returns min and max
// values.
//
// Supported formats:
//   - "1" - exactly one (min=1, max=1)
//   - "1.." - one or more (min=1, max=-1 for unbounded)
//   - "0..1" - zero or one (min=0, max=1)
//   - "0.." - zero or more (min=0, max=-1 for unbounded)
//   - "2..5" - between 2 and 5 (min=2, max=5)
//
// If the tag is empty, returns (0, -1, nil) where -1 means unbounded. Malformed
// inputs return a non-nil error.
func ParseCardinalityTag(cardinality string) (int, int, errors.E) {
	if cardinality == "" {
		return 0, -1, nil
	}

	if strings.Contains(cardinality, "..") {
		parts := strings.Split(cardinality, "..")
		if len(parts) != 2 { //nolint:mnd
			errE := errors.New("invalid cardinality format")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}
		minStr := strings.TrimSpace(parts[0])
		if minStr == "" {
			errE := errors.New("cardinality min value is empty")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}
		minCardinality, err := strconv.Atoi(minStr)
		if err != nil {
			errE := errors.New("cardinality min value is not a valid integer")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errors.WrapWith(err, errE)
		}
		if minCardinality < 0 {
			errE := errors.New("cardinality min value cannot be negative")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}

		maxCardinality := -1
		maxStr := strings.TrimSpace(parts[1])
		if maxStr != "" {
			maxCardinality, err = strconv.Atoi(maxStr)
			if err != nil {
				errE := errors.New("cardinality max value is not a valid integer")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errors.WrapWith(err, errE)
			}
			if maxCardinality <= 0 {
				errE := errors.New("cardinality max value cannot be negative or zero")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errE
			}
			if maxCardinality < minCardinality {
				errE := errors.New("cardinality max value cannot be less than min")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errE
			}
		}

		return minCardinality, maxCardinality, nil
	}

	val, err := strconv.Atoi(strings.TrimSpace(cardinality))
	if err != nil {
		errE := errors.New("cardinality value is not a valid integer")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errors.WrapWith(err, errE)
	}
	if val <= 0 {
		errE := errors.New("cardinality value cannot be negative or zero")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errE
	}

	return val, val, nil
}
