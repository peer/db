package search

import (
	"math"
	"time"

	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
	internalDocument "gitlab.com/peerdb/peerdb/internal/document"
)

// secondsPerYear is the approximate number of seconds in a year used when comparing
// a precision step with the histogram span. Exact-year math is unnecessary here. We only
// need the right order of magnitude (mirrors SECONDS_PER_YEAR in src/utils.ts).
const secondsPerYear = 60 * 60 * 24 * 365

// timePrecisionForValue picks the coarsest precision a single float64 unix-second timestamp
// could plausibly be carrying, mirroring timePrecisionForValue in src/utils.ts. Anything with
// a fractional second part (beyond a small float64 tolerance) is classified as a second and
// finer precisions are never returned. Otherwise divisibility by 60/3600/86400 determines
// min/h/d, and the calendar fields (and year divisibility) determine the coarser tiers.
// Years within the four-digit range are never classified coarser than a year, mirroring
// inferYearPrecision in src/partials/input/InputTime.format.ts: a value like 2000-01-01
// comes from a year-precision claim in human-scale history, not a millennium one.
func timePrecisionForValue(seconds float64) document.TimePrecision {
	// Tolerate small float64 rounding error when classifying "is this an integer number of
	// seconds?". For unix seconds in the human-relevant range the ULP is well under this threshold.
	if math.Abs(seconds-math.Round(seconds)) >= 1e-6 { //nolint:mnd
		return document.TimePrecisionSecond
	}
	sec := int64(math.Round(seconds))
	if sec%60 != 0 {
		return document.TimePrecisionSecond
	}
	if sec%(60*60) != 0 { //nolint:mnd
		return document.TimePrecisionMinute
	}
	if sec%(60*60*24) != 0 { //nolint:mnd
		return document.TimePrecisionHour
	}
	// Calendar units (months, years) do not have a fixed second count, so we
	// switch to inspecting the date components.
	t := time.Unix(sec, 0).UTC()
	if t.Day() > 1 {
		return document.TimePrecisionDay
	}
	if t.Month() > time.January {
		return document.TimePrecisionMonth
	}
	year := t.Year()
	if year > -10_000 && year < 10_000 {
		return document.TimePrecisionYear
	}
	switch {
	case year%10 != 0:
		return document.TimePrecisionYear
	case year%100 != 0:
		return document.TimePrecisionTenYears
	case year%1_000 != 0:
		return document.TimePrecisionHundredYears
	case year%10_000 != 0:
		return document.TimePrecisionKiloYears
	case year%100_000 != 0:
		return document.TimePrecisionTenKiloYears
	case year%1_000_000 != 0:
		return document.TimePrecisionHundredKiloYears
	case year%10_000_000 != 0:
		return document.TimePrecisionMegaYears
	case year%100_000_000 != 0:
		return document.TimePrecisionTenMegaYears
	case year%1_000_000_000 != 0:
		return document.TimePrecisionHundredMegaYears
	default:
		return document.TimePrecisionGigaYears
	}
}

// approxTimePrecisionSeconds returns the approximate length of one precision window in
// seconds. It is used only to compare a precision step with the histogram span, so the
// right order of magnitude is enough.
//
//nolint:mnd
func approxTimePrecisionSeconds(precision document.TimePrecision) float64 {
	switch precision {
	case document.TimePrecisionGigaYears:
		return 1e9 * secondsPerYear
	case document.TimePrecisionHundredMegaYears:
		return 1e8 * secondsPerYear
	case document.TimePrecisionTenMegaYears:
		return 1e7 * secondsPerYear
	case document.TimePrecisionMegaYears:
		return 1e6 * secondsPerYear
	case document.TimePrecisionHundredKiloYears:
		return 1e5 * secondsPerYear
	case document.TimePrecisionTenKiloYears:
		return 1e4 * secondsPerYear
	case document.TimePrecisionKiloYears:
		return 1e3 * secondsPerYear
	case document.TimePrecisionHundredYears:
		return 1e2 * secondsPerYear
	case document.TimePrecisionTenYears:
		return 1e1 * secondsPerYear
	case document.TimePrecisionYear:
		return secondsPerYear
	case document.TimePrecisionMonth:
		return 60 * 60 * 24 * 30
	case document.TimePrecisionDay:
		return 60 * 60 * 24
	case document.TimePrecisionHour:
		return 60 * 60
	case document.TimePrecisionMinute:
		return 60
	case document.TimePrecisionSecond:
		return 1
	case document.TimePrecisionMillisecond:
		return 1e-3
	case document.TimePrecisionMicrosecond:
		return 1e-6
	case document.TimePrecisionNanosecond:
		return 1e-9
	default:
		return 1
	}
}

// timeStepDown returns the timestamp lowered by one window of the precision the value appears
// to be carrying, so that a lowered histogram start still renders as a reasonable timestamp
// (a one ULP step would render as something like 1999-12-31T23:59:59.999...). The guessed
// precision is refined to a finer one until the step does not exceed span: a deep-time value
// at a coarse calendar boundary must not blow up a much narrower histogram.
func timeStepDown(v, span float64) float64 {
	// Outside the int64 range of seconds time.Unix overflows.
	if v <= -9.2e18 || v >= 9.2e18 {
		return math.Nextafter(v, math.Inf(-1))
	}
	precision := timePrecisionForValue(v)
	for precision < document.TimePrecisionSecond && approxTimePrecisionSeconds(precision) > span {
		precision++
	}
	// Calendar arithmetic has to happen in UTC. In a local timezone AddDate would pick up
	// historical UTC offset changes (e.g., local mean time used before timezones).
	stepped := x.TimeToFloat64(internalDocument.AddTimePrecision(x.TimeFromFloat64(v).UTC(), precision, -1))
	if stepped >= v {
		// The step did not survive the float64 resolution of the value.
		return math.Nextafter(v, math.Inf(-1))
	}
	return stepped
}

// amountStepDown returns the amount lowered by one step of the decimal precision the value
// appears to be carrying: the largest power of ten the value is an integer multiple of,
// refined to a finer one until the step does not exceed span (so a round value like 100 in
// a histogram spanning [100, 110] steps down by 10, not by 100). A zero value carries no
// decimal precision of its own, so it steps down by the largest power of ten not exceeding
// span, capped at one.
//
//nolint:mnd
func amountStepDown(v, span float64) float64 {
	const minStep = 1e-12
	if v == 0 {
		step := 1.0
		for step > span && step > minStep {
			step /= 10
		}
		return -step
	}
	for step := math.Pow(10, math.Floor(math.Log10(math.Abs(v)))); step >= minStep; step /= 10 {
		if step > span {
			continue
		}
		q := v / step
		if math.Abs(q-math.Round(q)) < 1e-9 {
			stepped := v - step
			if stepped < v {
				return stepped
			}
			// The step did not survive the float64 resolution of the value.
			break
		}
	}
	return math.Nextafter(v, math.Inf(-1))
}
