package document

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	internalDocument "gitlab.com/peerdb/peerdb/internal/document"
)

// Time represents a point in time.
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
type Time string

var timeRegex = regexp.MustCompile(`^(-?\d{4,})(?:-(\d{2})-(\d{2})(?: (\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{3}(?:\d{3}(?:\d{3})?)?))?)?)?)?$`)

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

// Float64 returns the float64 representation of a Time,
// as seconds since the Unix epoch.
//
// Passing 0 for precision skips checks for precision.
//
// It location is nil, UTC is used.
func (t Time) Float64(precision TimePrecision, location *time.Location) (float64, errors.E) {
	tm, errE := t.Time(precision, location)
	if errE != nil {
		return 0, errE
	}
	return x.TimeToFloat64(tm), nil
}

// Time returns the time.Time representation of a Time.
//
// Passing 0 for precision skips checks for precision.
//
// It location is nil, UTC is used.
func (t Time) Time(precision TimePrecision, location *time.Location) (time.Time, errors.E) { //nolint:maintidx
	if location == nil {
		location = time.UTC
	}

	s := string(t)
	match := timeRegex.FindStringSubmatch(s)
	if match == nil {
		errE := errors.New("unable to parse time")
		errors.Details(errE)["value"] = s
		return time.Time{}, errE
	}
	year, err := strconv.ParseInt(match[timeIndexYear], 10, 0)
	if err != nil {
		errE := errors.WithMessage(err, "unable to parse year")
		errors.Details(errE)["value"] = s
		return time.Time{}, errE
	}
	var month, day, hours, minutes, seconds, nanoseconds int64 = -1, 0, -1, -1, -1, -1
	if match[timeIndexMonth] != "" { //nolint:nestif
		month, err = strconv.ParseInt(match[timeIndexMonth], 10, 0)
		if err != nil {
			errE := errors.WithMessage(err, "unable to parse month")
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
			errE := errors.WithMessage(err, "unable to parse day")
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
				errE := errors.WithMessage(err, "unable to parse hours")
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
				errE := errors.WithMessage(err, "unable to parse minutes")
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
					errE := errors.WithMessage(err, "unable to parse seconds")
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
						errE := errors.WithMessage(err, "unable to parse subseconds")
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
		if mult := yearPrecisionMultiple(precision); year%mult != 0 {
			errE = errors.New("year not rounded to precision")
		} else if (month != -1) != needsMonth {
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
			switch internalDocument.TimePrecision(precision) { //nolint:exhaustive,unconvert
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

// Validate checks if the time is valid for the given precision and returns an error if it is not.
//
// Passing 0 for precision skips checks for precision and just checks the format.
func (t Time) Validate(precision TimePrecision) errors.E {
	_, errE := t.Time(precision, time.UTC)
	return errE
}

// WindowStartFloat64 returns the lower edge that this bound contributes to a
// half-open indexed range, as float64 seconds since the unix epoch. When the
// bound is closed (default, isOpen=false) this is the start of the precision
// window; when open (isOpen=true) the precision window is excluded and the
// edge advances to the end of the window.
func (t Time) WindowStartFloat64(precision TimePrecision, isOpen bool) (float64, errors.E) {
	if isOpen {
		return t.windowEndFloat64(precision)
	}
	return t.windowStartFloat64(precision)
}

// WindowEndFloat64 returns the upper edge that this bound contributes to a
// half-open indexed range, as float64 seconds since the unix epoch. When the
// bound is closed (default, isOpen=false) this is the end of the precision
// window; when open (isOpen=true) the precision window is excluded and the
// edge retreats to the start of the window.
func (t Time) WindowEndFloat64(precision TimePrecision, isOpen bool) (float64, errors.E) {
	if isOpen {
		return t.windowStartFloat64(precision)
	}
	return t.windowEndFloat64(precision)
}

// windowStartFloat64 returns the start of the precision window represented
// by t as float64 seconds since the unix epoch.
func (t Time) windowStartFloat64(precision TimePrecision) (float64, errors.E) {
	return t.Float64(precision, time.UTC)
}

// windowEndFloat64 returns the end of the precision window represented by
// t as float64 seconds since the unix epoch.
//
// If the natural step for the requested precision is below the float64
// resolution at t's magnitude (i.e. it rounds back to t through
// x.TimeToFloat64), the function falls back to the next coarser precision.
// Within Go's representable time range (year ~-291 billion to ~+291 billion)
// this widening is enough: at the extremes the float64 ULP is ~1024 s, so
// sub-hour precisions widen up to hour, and hour-and-above always survive.
func (t Time) windowEndFloat64(precision TimePrecision) (float64, errors.E) {
	parsed, errE := t.Time(precision, time.UTC)
	if errE != nil {
		return 0, errE
	}
	return x.TimeToFloat64(addTimePrecision(parsed, precision)), nil
}

// addTimePrecision returns the time at the end of the precision window
// starting at t. If the natural step does not survive the float64
// round-trip via x.TimeToFloat64 it widens to the next coarser precision.
//
//nolint:cyclop
func addTimePrecision(t time.Time, precision TimePrecision) time.Time {
	var stepped time.Time
	switch precision {
	case TimePrecisionGigaYears:
		stepped = t.AddDate(1_000_000_000, 0, 0) //nolint:mnd
	case TimePrecisionHundredMegaYears:
		stepped = t.AddDate(100_000_000, 0, 0) //nolint:mnd
	case TimePrecisionTenMegaYears:
		stepped = t.AddDate(10_000_000, 0, 0) //nolint:mnd
	case TimePrecisionMegaYears:
		stepped = t.AddDate(1_000_000, 0, 0) //nolint:mnd
	case TimePrecisionHundredKiloYears:
		stepped = t.AddDate(100_000, 0, 0) //nolint:mnd
	case TimePrecisionTenKiloYears:
		stepped = t.AddDate(10_000, 0, 0) //nolint:mnd
	case TimePrecisionKiloYears:
		stepped = t.AddDate(1_000, 0, 0) //nolint:mnd
	case TimePrecisionHundredYears:
		stepped = t.AddDate(100, 0, 0) //nolint:mnd
	case TimePrecisionTenYears:
		stepped = t.AddDate(10, 0, 0) //nolint:mnd
	case TimePrecisionYear:
		stepped = t.AddDate(1, 0, 0)
	case TimePrecisionMonth:
		stepped = t.AddDate(0, 1, 0)
	case TimePrecisionDay:
		stepped = t.AddDate(0, 0, 1)
	case TimePrecisionHour:
		stepped = t.Add(time.Hour)
	case TimePrecisionMinute:
		stepped = t.Add(time.Minute)
	case TimePrecisionSecond:
		stepped = t.Add(time.Second)
	case TimePrecisionMillisecond:
		stepped = t.Add(time.Millisecond)
	case TimePrecisionMicrosecond:
		stepped = t.Add(time.Microsecond)
	case TimePrecisionNanosecond:
		stepped = t.Add(time.Nanosecond)
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
		return addTimePrecision(t, precision-1)
	}
	return stepped
}

// MarshalText implements encoding.TextMarshaler for Time.
func (t Time) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// MarshalJSON implements json.Marshaler for Time.
func (t Time) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(t.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

// String returns the string representation of Time.
func (t Time) String() string {
	return string(t)
}

// UnmarshalText implements encoding.TextUnmarshaler for Time.
func (t *Time) UnmarshalText(text []byte) error {
	ts := Time(text)

	// We use special value 0 for precision which skips checking precision.
	errE := ts.Validate(0)
	if errE != nil {
		return errE
	}

	*t = ts

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for Time.
func (t *Time) UnmarshalJSON(data []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(data, &s)
	if errE != nil {
		return errE
	}
	return t.UnmarshalText([]byte(s))
}

// TimePrecision represents the precision level of a timestamp.
type TimePrecision = internalDocument.TimePrecision

const (
	// TimePrecisionGigaYears represents a time precision of giga-years (1 billion years).
	TimePrecisionGigaYears = internalDocument.TimePrecisionGigaYears
	// TimePrecisionHundredMegaYears represents a time precision of 100 million years.
	TimePrecisionHundredMegaYears = internalDocument.TimePrecisionHundredMegaYears
	// TimePrecisionTenMegaYears represents a time precision of 10 million years.
	TimePrecisionTenMegaYears = internalDocument.TimePrecisionTenMegaYears
	// TimePrecisionMegaYears represents a time precision of 1 million years (mega-years).
	TimePrecisionMegaYears = internalDocument.TimePrecisionMegaYears
	// TimePrecisionHundredKiloYears represents a time precision of 100 thousand years.
	TimePrecisionHundredKiloYears = internalDocument.TimePrecisionHundredKiloYears
	// TimePrecisionTenKiloYears represents a time precision of 10 thousand years.
	TimePrecisionTenKiloYears = internalDocument.TimePrecisionTenKiloYears
	// TimePrecisionKiloYears represents a time precision of 1 thousand years (kilo-years).
	TimePrecisionKiloYears = internalDocument.TimePrecisionKiloYears
	// TimePrecisionHundredYears represents a time precision of 100 years (centuries).
	TimePrecisionHundredYears = internalDocument.TimePrecisionHundredYears
	// TimePrecisionTenYears represents a time precision of 10 years (decades).
	TimePrecisionTenYears = internalDocument.TimePrecisionTenYears
	// TimePrecisionYear represents a time precision of 1 year.
	TimePrecisionYear = internalDocument.TimePrecisionYear
	// TimePrecisionMonth represents a time precision of 1 month.
	TimePrecisionMonth = internalDocument.TimePrecisionMonth
	// TimePrecisionDay represents a time precision of 1 day.
	TimePrecisionDay = internalDocument.TimePrecisionDay
	// TimePrecisionHour represents a time precision of 1 hour.
	TimePrecisionHour = internalDocument.TimePrecisionHour
	// TimePrecisionMinute represents a time precision of 1 minute.
	TimePrecisionMinute = internalDocument.TimePrecisionMinute
	// TimePrecisionSecond represents a time precision of 1 second.
	TimePrecisionSecond = internalDocument.TimePrecisionSecond
	// TimePrecisionMillisecond represents a time precision of 1 millisecond.
	TimePrecisionMillisecond = internalDocument.TimePrecisionMillisecond
	// TimePrecisionMicrosecond represents a time precision of 1 microsecond.
	TimePrecisionMicrosecond = internalDocument.TimePrecisionMicrosecond
	// TimePrecisionNanosecond represents a time precision of 1 nanosecond.
	TimePrecisionNanosecond = internalDocument.TimePrecisionNanosecond
)

// yearPrecisionMultiple returns the factor by which the year must be divisible
// for precisions coarser than a single year. Returns 1 for TimePrecisionYear and finer.
func yearPrecisionMultiple(precision internalDocument.TimePrecision) int64 {
	switch precision { //nolint:exhaustive
	case TimePrecisionGigaYears:
		return 1_000_000_000 //nolint:mnd
	case TimePrecisionHundredMegaYears:
		return 100_000_000 //nolint:mnd
	case TimePrecisionTenMegaYears:
		return 10_000_000 //nolint:mnd
	case TimePrecisionMegaYears:
		return 1_000_000 //nolint:mnd
	case TimePrecisionHundredKiloYears:
		return 100_000 //nolint:mnd
	case TimePrecisionTenKiloYears:
		return 10_000 //nolint:mnd
	case TimePrecisionKiloYears:
		return 1_000 //nolint:mnd
	case TimePrecisionHundredYears:
		return 100 //nolint:mnd
	case TimePrecisionTenYears:
		return 10 //nolint:mnd
	default:
		return 1
	}
}

// NewTime formats a time.Time into a Time string
// with only the parts required by the given precision.
//
// The timestamp is formatted in the provided location (timezone).
// If location is nil, UTC is used.
func NewTime(t time.Time, precision TimePrecision, location *time.Location) Time {
	if location == nil {
		location = time.UTC
	}

	t = t.In(location)
	w := 4
	year, month, day := t.Date()
	// Truncate year to the required precision multiple (e.g. decade -> nearest 10).
	// Go's integer division truncates toward zero, which is consistent with the
	// year%multiple==0 divisibility check used in Time().
	if mult := int(yearPrecisionMultiple(precision)); mult != 1 {
		year = (year / mult) * mult
	}
	if year < 0 {
		// An extra character for the minus sign.
		w = 5
	}
	switch {
	case precision >= TimePrecisionNanosecond:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%09d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		))
	case precision >= TimePrecisionMicrosecond:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%06d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000, //nolint:mnd
		))
	case precision >= TimePrecisionMillisecond:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d.%03d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1_000_000, //nolint:mnd
		))
	case precision >= TimePrecisionSecond:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d:%02d",
			w, year, month, day, t.Hour(), t.Minute(), t.Second(),
		))
	case precision >= TimePrecisionMinute:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:%02d",
			w, year, month, day, t.Hour(), t.Minute(),
		))
	case precision >= TimePrecisionHour:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d %02d:00",
			w, year, month, day, t.Hour(),
		))
	case precision >= TimePrecisionDay:
		return Time(fmt.Sprintf(
			"%0*d-%02d-%02d",
			w, year, month, day,
		))
	case precision >= TimePrecisionMonth:
		return Time(fmt.Sprintf(
			"%0*d-%02d-00",
			w, year, month,
		))
	default:
		return Time(fmt.Sprintf(
			"%0*d",
			w, year,
		))
	}
}
