package document

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// Timestamp represents a point in time.
//
// It generally operates with two additional pieces of information
// which are not part of the timestamp itself:
//
//   - [TimePrecision]: precision of the timestamp
//   - [time.Location]: location (timezone) of the timestamp
//
// It is represented as a string to preserve the original format
// as provided by the user. The format is RFC 3339 compatible with
// the following changes:
//
//   - year component can have more than 4 digits and can have a negative sign
//   - supports milliseconds, microseconds and nanoseconds with exactly 3, 6, or
//     9 decimal fraction digits, respectively
//   - day component can be zero for timestamps used with month precision,
//     but month component cannot be zero
//   - timestamp can contain just the part of the format when used with precision
//     which does not require other parts, parts are in order: a) year, b) month + day,
//     c) hours + minutes, d) seconds, e) milliseconds, f) microseconds, and g) nanoseconds
//   - instead of T delimiter, a space is used
//   - location (timezone) must not be present
//
//nolint:recvcheck
type Timestamp string

var timeRegex = regexp.MustCompile(`^([+-]?\d{4,})(?:-(\d{2})-(\d{2})(?: (\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{3}(?:\d{3}(?:\d{3})?)?))?)?)?)?$`)

const (
	timeIndexYear = iota + 1
	timeIndexMonth
	timeIndexDay
	timeIndexHours
	timeIndexMinutes
	timeIndexSeconds
	timeIndexSubseconds
)

// Based on isLeap from Go's time.Time from version 1.25.
func isLeap(year int) bool {
	mask := 0xf
	if year%25 != 0 {
		mask = 3
	}
	return year&mask == 0
}

// Based on daysIn from Go's time.Time from version 1.25.
func daysIn(month, year int) int {
	if month == 2 { //nolint:mnd
		if isLeap(year) {
			return 29 //nolint:mnd
		}
		return 28 //nolint:mnd
	}
	return 30 + int((month+month>>3)&1) //nolint:mnd,unconvert
}

// Time returns the time.Time representation of a Timestamp.
//
// Passing 0 for precision skips checks for precision.
//
// It location is nil, UTC is used.
func (t Timestamp) Time(precision TimePrecision, location *time.Location) (time.Time, errors.E) { //nolint:maintidx
	if location == nil {
		location = time.UTC
	}

	s := string(t)
	match := timeRegex.FindStringSubmatch(s)
	if match == nil {
		errE := errors.New("unable to parse timestamp")
		errors.Details(errE)["value"] = s
		return time.Time{}, errE
	}
	year, err := strconv.ParseInt(match[timeIndexYear], 10, 0)
	if err != nil {
		errE := errors.New("unable to parse year")
		errors.Details(errE)["value"] = s
		return time.Time{}, errE
	}
	var month, day, hours, minutes, seconds, nanoseconds int64 = -1, 0, -1, -1, -1, -1
	if match[timeIndexMonth] != "" { //nolint:nestif
		month, err = strconv.ParseInt(match[timeIndexMonth], 10, 0)
		if err != nil {
			errE := errors.New("unable to parse month")
			errors.Details(errE)["value"] = s
			return time.Time{}, errE
		}
		// Month cannot be 0.
		if month < 1 || month > 12 {
			errE := errors.New("month out of range")
			errors.Details(errE)["value"] = s
			return time.Time{}, errE
		}
		day, err = strconv.ParseInt(match[timeIndexDay], 10, 0)
		if err != nil {
			errE := errors.New("unable to parse day")
			errors.Details(errE)["value"] = s
			return time.Time{}, errE
		}
		// We support 0 for day.
		if day > int64(daysIn(int(month), int(year))) {
			errE := errors.New("day out of range")
			errors.Details(errE)["value"] = s
			return time.Time{}, errE
		}
		if match[timeIndexHours] != "" {
			hours, err = strconv.ParseInt(match[timeIndexHours], 10, 0)
			if err != nil {
				errE := errors.New("unable to parse hours")
				errors.Details(errE)["value"] = s
				return time.Time{}, errE
			}
			if hours > 23 { //nolint:mnd
				errE := errors.New("hours out of range")
				errors.Details(errE)["value"] = s
				return time.Time{}, errE
			}
			minutes, err = strconv.ParseInt(match[timeIndexMinutes], 10, 0)
			if err != nil {
				errE := errors.New("unable to parse minutes")
				errors.Details(errE)["value"] = s
				return time.Time{}, errE
			}
			if minutes > 59 { //nolint:mnd
				errE := errors.New("minutes out of range")
				errors.Details(errE)["value"] = s
				return time.Time{}, errE
			}
			if match[timeIndexSeconds] != "" {
				seconds, err = strconv.ParseInt(match[timeIndexSeconds], 10, 0)
				if err != nil {
					errE := errors.New("unable to parse seconds")
					errors.Details(errE)["value"] = s
					return time.Time{}, errE
				}
				if seconds > 59 { //nolint:mnd
					errE := errors.New("seconds out of range")
					errors.Details(errE)["value"] = s
					return time.Time{}, errE
				}
				if match[timeIndexSubseconds] != "" {
					nanoseconds, err = strconv.ParseInt(match[timeIndexSubseconds], 10, 0)
					if err != nil {
						errE := errors.New("unable to parse subseconds")
						errors.Details(errE)["value"] = s
						return time.Time{}, errE
					}
					switch len(match[timeIndexSubseconds]) {
					case 3: //nolint:mnd
						nanoseconds *= 1000 * 1000 //nolint:mnd
					case 6: //nolint:mnd
						nanoseconds *= 1000
					case 9: //nolint:mnd
					default:
						// This should not be possible.
						errE := errors.New("unexpected subseconds length")
						errors.Details(errE)["value"] = s
						panic(errE)
					}
				}
			}
		}
	}

	if precision != 0 { //nolint:nestif
		// Determine which parts the precision requires.
		needsMonth := precision >= TimePrecisionMonth
		needsDay := precision >= TimePrecisionDay
		needsHours := precision >= TimePrecisionHour
		needsMinutes := precision >= TimePrecisionMinute
		needsSeconds := precision >= TimePrecisionSecond
		needsSubseconds := precision >= TimePrecisionMillisecond

		// Validate that present/absent parts match precision.
		var errE errors.E
		if (month != -1) != needsMonth {
			if needsMonth {
				errE = errors.New("month required for precision")
			} else {
				errE = errors.New("month not allowed for precision")
			}
		} else if (day != 0) != needsDay {
			if needsDay {
				errE = errors.New("day required for precision")
			} else {
				errE = errors.New("day not allowed for precision")
			}
		} else if (hours != -1) != needsHours {
			if needsHours {
				errE = errors.New("hours and minutes required for precision")
			} else {
				errE = errors.New("hours and minutes not allowed for precision")
			}
		} else if hours != -1 && !needsMinutes && minutes != 0 {
			errE = errors.New("minutes must be zero for hour precision")
		} else if (seconds != -1) != needsSeconds {
			if needsSeconds {
				errE = errors.New("seconds required for precision")
			} else {
				errE = errors.New("seconds not allowed for precision")
			}
		} else if (nanoseconds != -1) != needsSubseconds {
			if needsSubseconds {
				errE = errors.New("subseconds required for precision")
			} else {
				errE = errors.New("subseconds not allowed for precision")
			}
		} else if nanoseconds != -1 {
			var requiredSubsecondsLen int
			switch precision { //nolint:exhaustive
			case TimePrecisionMillisecond:
				requiredSubsecondsLen = 3
			case TimePrecisionMicrosecond:
				requiredSubsecondsLen = 6
			case TimePrecisionNanosecond:
				requiredSubsecondsLen = 9
			default:
				errE = errors.New("invalid precision")
			}
			if errE == nil && len(match[timeIndexSubseconds]) != requiredSubsecondsLen {
				errE = errors.New("subseconds length does not match precision")
			}
		}

		if errE != nil {
			errors.Details(errE)["value"] = s
			errors.Details(errE)["precision"] = precision.String()
			return time.Time{}, errE
		}
	}

	// Replace absent parts with defaults for time.Date.
	if month == -1 {
		month = 1
	}
	if day == 0 {
		day = 1
	}
	if hours == -1 {
		hours, minutes = 0, 0
	}
	if seconds == -1 {
		seconds = 0
	}
	if nanoseconds == -1 {
		nanoseconds = 0
	}

	return time.Date(int(year), time.Month(month), int(day), int(hours), int(minutes), int(seconds), int(nanoseconds), location), nil
}

// Validate checks if the timestamp is valid for the given precision and returns an error if it is not.
//
// Passing 0 for precision skips checks for precision and just checks the format.
func (t Timestamp) Validate(precision TimePrecision) errors.E {
	_, errE := t.Time(precision, time.UTC)
	return errE
}

// MarshalText implements encoding.TextMarshaler for Timestamp.
func (t Timestamp) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// MarshalJSON implements json.Marshaler for Timestamp.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(t.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

// String returns the string representation of Timestamp.
func (t Timestamp) String() string {
	return string(t)
}

// UnmarshalText implements encoding.TextUnmarshaler for Timestamp.
func (t *Timestamp) UnmarshalText(text []byte) error {
	timestamp := Timestamp(text)

	// We use special value 0 for precision which skips checking precision.
	errE := timestamp.Validate(0)
	if errE != nil {
		return errE
	}

	*t = timestamp

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for Timestamp.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(data, &s)
	if errE != nil {
		return errE
	}
	return t.UnmarshalText([]byte(s))
}

// TimePrecision represents the precision level of a timestamp.
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

// NewTimestamp formats a time.Time into a Timestamp string
// with only the parts required by the given precision.
//
// The timestamp is formatted in the provided location (timezone).
// If location is nil, UTC is used.
func NewTimestamp(t time.Time, precision TimePrecision, location *time.Location) Timestamp {
	if location == nil {
		location = time.UTC
	}

	t = t.In(location)
	w := 4
	year, month, day := t.Date()
	if year < 0 {
		// An extra character for the minus sign.
		w = 5
	}
	switch {
	case precision >= TimePrecisionNanosecond:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%09d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		))
	case precision >= TimePrecisionMicrosecond:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%06d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000, //nolint:mnd
		))
	case precision >= TimePrecisionMillisecond:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%03d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1_000_000, //nolint:mnd
		))
	case precision >= TimePrecisionSecond:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(),
		))
	case precision >= TimePrecisionMinute:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d",
			w, year, month, day, t.Hour(), t.Minute(),
		))
	case precision >= TimePrecisionHour:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:00",
			w, year, month, day, t.Hour(),
		))
	case precision >= TimePrecisionDay:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-%02d",
			w, year, month, day,
		))
	case precision >= TimePrecisionMonth:
		return Timestamp(fmt.Sprintf(
			"%0*d-%02d-00",
			w, year, month,
		))
	default:
		return Timestamp(fmt.Sprintf(
			"%0*d",
			w, year,
		))
	}
}
