// Package document provides shared document types used across packages.
package document

import (
	"bytes"
	"fmt"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// TimePrecision represents the precision of a time value.
//
//nolint:recvcheck
type TimePrecision int

const (
	// TimePrecisionGigaYears represents a time precision of giga-years (1 billion years).
	TimePrecisionGigaYears TimePrecision = iota + 1
	// TimePrecisionHundredMegaYears represents a time precision of 100 million years.
	TimePrecisionHundredMegaYears
	// TimePrecisionTenMegaYears represents a time precision of 10 million years.
	TimePrecisionTenMegaYears
	// TimePrecisionMegaYears represents a time precision of 1 million years (mega-years).
	TimePrecisionMegaYears
	// TimePrecisionHundredKiloYears represents a time precision of 100 thousand years.
	TimePrecisionHundredKiloYears
	// TimePrecisionTenKiloYears represents a time precision of 10 thousand years.
	TimePrecisionTenKiloYears
	// TimePrecisionKiloYears represents a time precision of 1 thousand years (kilo-years).
	TimePrecisionKiloYears
	// TimePrecisionHundredYears represents a time precision of 100 years (centuries).
	TimePrecisionHundredYears
	// TimePrecisionTenYears represents a time precision of 10 years (decades).
	TimePrecisionTenYears
	// TimePrecisionYear represents a time precision of 1 year.
	TimePrecisionYear
	// TimePrecisionMonth represents a time precision of 1 month.
	TimePrecisionMonth
	// TimePrecisionDay represents a time precision of 1 day.
	TimePrecisionDay
	// TimePrecisionHour represents a time precision of 1 hour.
	TimePrecisionHour
	// TimePrecisionMinute represents a time precision of 1 minute.
	TimePrecisionMinute
	// TimePrecisionSecond represents a time precision of 1 second.
	TimePrecisionSecond
	// TimePrecisionMillisecond represents a time precision of 1 millisecond.
	TimePrecisionMillisecond
	// TimePrecisionMicrosecond represents a time precision of 1 microsecond.
	TimePrecisionMicrosecond
	// TimePrecisionNanosecond represents a time precision of 1 nanosecond.
	TimePrecisionNanosecond
)

// MarshalText implements encoding.TextMarshaler for TimePrecision.
func (p TimePrecision) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// MarshalJSON implements json.Marshaler for TimePrecision.
func (p TimePrecision) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(p.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

// String returns the string representation of TimePrecision.
func (p TimePrecision) String() string {
	switch p {
	case TimePrecisionGigaYears:
		return "G"
	case TimePrecisionHundredMegaYears:
		return "100M"
	case TimePrecisionTenMegaYears:
		return "10M"
	case TimePrecisionMegaYears:
		return "M"
	case TimePrecisionHundredKiloYears:
		return "100k"
	case TimePrecisionTenKiloYears:
		return "10k"
	case TimePrecisionKiloYears:
		return "k"
	case TimePrecisionHundredYears:
		return "100y"
	case TimePrecisionTenYears:
		return "10y"
	case TimePrecisionYear:
		return "y"
	case TimePrecisionMonth:
		return "m"
	case TimePrecisionDay:
		return "d"
	case TimePrecisionHour:
		return "h"
	case TimePrecisionMinute:
		return "min"
	case TimePrecisionSecond:
		return "s"
	case TimePrecisionMillisecond:
		return "ms"
	case TimePrecisionMicrosecond:
		return "us"
	case TimePrecisionNanosecond:
		return "ns"
	default:
		return fmt.Sprintf("[%d]", p)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for TimePrecision.
func (p *TimePrecision) UnmarshalText(text []byte) error {
	s := string(text)

	switch s {
	case "G":
		*p = TimePrecisionGigaYears
	case "100M":
		*p = TimePrecisionHundredMegaYears
	case "10M":
		*p = TimePrecisionTenMegaYears
	case "M":
		*p = TimePrecisionMegaYears
	case "100k":
		*p = TimePrecisionHundredKiloYears
	case "10k":
		*p = TimePrecisionTenKiloYears
	case "k":
		*p = TimePrecisionKiloYears
	case "100y":
		*p = TimePrecisionHundredYears
	case "10y":
		*p = TimePrecisionTenYears
	case "y":
		*p = TimePrecisionYear
	case "m":
		*p = TimePrecisionMonth
	case "d":
		*p = TimePrecisionDay
	case "h":
		*p = TimePrecisionHour
	case "min":
		*p = TimePrecisionMinute
	case "s":
		*p = TimePrecisionSecond
	case "ms":
		*p = TimePrecisionMillisecond
	case "us":
		*p = TimePrecisionMicrosecond
	case "ns":
		*p = TimePrecisionNanosecond
	default:
		errE := errors.New("unknown time precision")
		errors.Details(errE)["value"] = s
		return errE
	}

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for TimePrecision.
func (p *TimePrecision) UnmarshalJSON(b []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(b, &s)
	if errE != nil {
		return errE
	}
	return p.UnmarshalText([]byte(s))
}

// AddTimePrecision returns the time moved by steps precision windows from t (e.g., one
// window forward for steps of 1 or one window back for steps of -1). If the natural step
// does not survive the float64 round-trip via x.TimeToFloat64 it widens to the next
// coarser precision.
//
//nolint:cyclop,mnd
func AddTimePrecision(t time.Time, precision TimePrecision, steps int) time.Time {
	var stepped time.Time
	switch precision {
	case TimePrecisionGigaYears:
		stepped = t.AddDate(steps*1_000_000_000, 0, 0)
	case TimePrecisionHundredMegaYears:
		stepped = t.AddDate(steps*100_000_000, 0, 0)
	case TimePrecisionTenMegaYears:
		stepped = t.AddDate(steps*10_000_000, 0, 0)
	case TimePrecisionMegaYears:
		stepped = t.AddDate(steps*1_000_000, 0, 0)
	case TimePrecisionHundredKiloYears:
		stepped = t.AddDate(steps*100_000, 0, 0)
	case TimePrecisionTenKiloYears:
		stepped = t.AddDate(steps*10_000, 0, 0)
	case TimePrecisionKiloYears:
		stepped = t.AddDate(steps*1_000, 0, 0)
	case TimePrecisionHundredYears:
		stepped = t.AddDate(steps*100, 0, 0)
	case TimePrecisionTenYears:
		stepped = t.AddDate(steps*10, 0, 0)
	case TimePrecisionYear:
		stepped = t.AddDate(steps, 0, 0)
	case TimePrecisionMonth:
		stepped = t.AddDate(0, steps, 0)
	case TimePrecisionDay:
		stepped = t.AddDate(0, 0, steps)
	case TimePrecisionHour:
		stepped = t.Add(time.Duration(steps) * time.Hour)
	case TimePrecisionMinute:
		stepped = t.Add(time.Duration(steps) * time.Minute)
	case TimePrecisionSecond:
		stepped = t.Add(time.Duration(steps) * time.Second)
	case TimePrecisionMillisecond:
		stepped = t.Add(time.Duration(steps) * time.Millisecond)
	case TimePrecisionMicrosecond:
		stepped = t.Add(time.Duration(steps) * time.Microsecond)
	case TimePrecisionNanosecond:
		stepped = t.Add(time.Duration(steps) * time.Nanosecond)
	default:
		errE := errors.New("unknown precision")
		errors.Details(errE)["precision"] = precision
		panic(errE)
	}

	if x.TimeToFloat64(stepped) == x.TimeToFloat64(t) {
		if precision == TimePrecisionGigaYears {
			// Nothing left to widen to.
			// This should not happen.
			errE := errors.New("unsupported precision")
			errors.Details(errE)["t"] = t
			errors.Details(errE)["precision"] = precision
			panic(errE)
		}
		return AddTimePrecision(t, precision-1, steps)
	}
	return stepped
}
