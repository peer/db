package search_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/search"
)

func utc(year int, month time.Month, day, hour, minute int) float64 {
	return x.TimeToFloat64(time.Date(year, month, day, hour, minute, 0, 0, time.UTC))
}

func TestTimePrecisionForValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    float64
		expected document.TimePrecision
	}{
		{1000.5, document.TimePrecisionSecond},
		{1000, document.TimePrecisionSecond},
		{utc(1970, time.January, 1, 0, 17), document.TimePrecisionMinute},
		{utc(1970, time.January, 1, 5, 0), document.TimePrecisionHour},
		{utc(2026, time.June, 17, 0, 0), document.TimePrecisionDay},
		{utc(2026, time.February, 1, 0, 0), document.TimePrecisionMonth},
		{utc(2021, time.January, 1, 0, 0), document.TimePrecisionYear},
		// Four-digit years are never classified coarser than a year, even at coarser
		// calendar boundaries.
		{utc(1990, time.January, 1, 0, 0), document.TimePrecisionYear},
		{utc(2000, time.January, 1, 0, 0), document.TimePrecisionYear},
		// Five-digit and larger years use the year divisibility walk.
		{utc(10000, time.January, 1, 0, 0), document.TimePrecisionTenKiloYears},
		{utc(12000, time.January, 1, 0, 0), document.TimePrecisionKiloYears},
		{utc(1_000_000, time.January, 1, 0, 0), document.TimePrecisionMegaYears},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, search.TestingTimePrecisionForValue(tt.value), "value %v", tt.value)
	}
}

func TestTimeStepDown(t *testing.T) {
	t.Parallel()

	yearSpan := float64(60 * 60 * 24 * 365)

	tests := []struct {
		name     string
		value    float64
		span     float64
		expected float64
	}{
		// A day-precision value steps down one day.
		{"day", utc(2026, time.June, 18, 0, 0), yearSpan, utc(2026, time.June, 17, 0, 0)},
		// 2000-01-01 classifies as a year (four-digit years never classify coarser),
		// so it steps down exactly one year.
		{"year boundary", utc(2000, time.January, 1, 0, 0), 26 * yearSpan, utc(1999, time.January, 1, 0, 0)},
		// Deep-time values use the year divisibility walk and step a full precision window.
		{"deep time", utc(10000, time.January, 1, 0, 0), 100_000 * yearSpan, utc(0, time.January, 1, 0, 0)},
		// A coarse deep-time step is refined to not exceed the span.
		{"deep time refined", utc(10000, time.January, 1, 0, 0), 5_000 * yearSpan, utc(9000, time.January, 1, 0, 0)},
		// A day-precision value with a span of a minute refines down to a minute step.
		{"refined to minute", utc(2026, time.June, 17, 0, 0), 60, utc(2026, time.June, 16, 23, 59)},
		// Second-precision values step down one second.
		{"second", 1000, 8000, 999},
		{"fractional second", 1000.5, 8000, 999.5},
		{"negative second", -500, 1000, -501},
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.expected, search.TestingTimeStepDown(tt.value, tt.span), 1e-9, "%s", tt.name)
	}
}

func TestAmountStepDown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    float64
		span     float64
		expected float64
	}{
		// 10 is a multiple of 10 and the span allows a step of 10.
		{"round ten", 10, 80, 0},
		// The span does not allow a step of 10, so it refines to 1.
		{"refined", 10, 5, 9},
		{"hundred refined", 100, 10, 90},
		{"integer", 25, 100, 24},
		{"decimal", 9.5, 90, 9.4},
		{"negative", -500, 1000, -600},
		// Zero carries no decimal precision of its own, so the step is the largest
		// power of ten not exceeding span, capped at one.
		{"zero", 0, 100, -1},
		{"zero small span", 0, 0.01, -0.01},
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.expected, search.TestingAmountStepDown(tt.value, tt.span), 1e-9, "%s", tt.name)
	}
}
