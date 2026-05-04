package document_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

func TestTimeMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ts   string
		unix int64
	}{
		{`"2006-12-04 12:34:45"`, 1165235685},
		{`"0206-12-04 12:34:45"`, -55637321115},
		{`"0001-12-04 12:34:45"`, -62106434715},
		{`"20006-12-04 12:34:45"`, 569190371685},
		{`"0000-12-04 12:34:45"`, -62137970715},
		{`"-0001-12-04 12:34:45"`, -62169593115},
		{`"-0206-12-04 12:34:45"`, -68638706715},
		{`"-2006-12-04 12:34:45"`, -125441263515},
		{`"-20006-12-04 12:34:45"`, -693466399515},
		{`"-239999999-01-01 00:00:00"`, -7573730615596800},
	}
	for _, test := range tests {
		t.Run(test.ts, func(t *testing.T) {
			t.Parallel()

			var ts document.Time
			in := []byte(test.ts)
			errE := x.UnmarshalWithoutUnknownFields(in, &ts)
			require.NoError(t, errE, "% -+#.1v", errE)
			tt, errE := ts.Time(0, time.UTC)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, test.unix, tt.Unix())
			out, errE := x.MarshalWithoutEscapeHTML(ts)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, in, out)
		})
	}
}

func TestTimeValidation(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		ts        string
		precision document.TimePrecision
	}{
		{"1000000000", document.TimePrecisionGigaYears},
		{"2025", document.TimePrecisionYear},
		{"2025-03-00", document.TimePrecisionMonth},
		{"2025-03-15", document.TimePrecisionDay},
		{"2025-03-15 10:00", document.TimePrecisionHour},
		{"2025-03-15 10:00", document.TimePrecisionMinute},
		{"2025-03-15 10:30", document.TimePrecisionMinute},
		{"2025-03-15 10:30:45", document.TimePrecisionSecond},
		{"2025-03-15 10:00:45", document.TimePrecisionSecond},
		{"2025-03-15 10:30:45.123", document.TimePrecisionMillisecond},
		{"2025-03-15 10:30:45.123456", document.TimePrecisionMicrosecond},
		{"2025-03-15 10:30:45.123456789", document.TimePrecisionNanosecond},
	}
	for _, tc := range validCases {
		t.Run(tc.ts+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Time(tc.ts).Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}

	invalidCases := []struct {
		ts        string
		precision document.TimePrecision
		errMsg    string
	}{
		{"2025-03-15", document.TimePrecisionYear, "month not allowed for precision"},
		{"2025", document.TimePrecisionMonth, "month required for precision"},
		{"2025-03-00", document.TimePrecisionDay, "day required for precision"},
		{"2025-03-15", document.TimePrecisionHour, "hours and minutes required for precision"},
		{"2025-03-15 10:30", document.TimePrecisionHour, "minutes must be zero for hour precision"},
		{"2025-03-15 10:30", document.TimePrecisionSecond, "seconds required for precision"},
		{"2025-03-15 10:30:45", document.TimePrecisionMillisecond, "subseconds required for precision"},
		{"2025-03-15 10:30:45.123", document.TimePrecisionMicrosecond, "subseconds length does not match precision"},
		{"2025-03-15 10:30:45.123456", document.TimePrecisionNanosecond, "subseconds length does not match precision"},
		{"2025-13-01", document.TimePrecisionDay, "month out of range"},
		{"2025-03-32", document.TimePrecisionDay, "day out of range"},
		{"2025-03-15 25:00", document.TimePrecisionHour, "hours out of range"},
		{"2025-03-15 10:60", document.TimePrecisionMinute, "minutes out of range"},
		{"2025-03-15 10:30:60", document.TimePrecisionSecond, "seconds out of range"},
		{"not-a-time", document.TimePrecisionYear, "unable to parse time"},
	}
	for _, tc := range invalidCases {
		t.Run(tc.ts+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Time(tc.ts).Validate(tc.precision)
			assert.EqualError(t, errE, tc.errMsg)
		})
	}
}

func TestTimePrecisionMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		precision document.TimePrecision
		json      string
	}{
		{document.TimePrecisionGigaYears, `"G"`},
		{document.TimePrecisionHundredMegaYears, `"100M"`},
		{document.TimePrecisionTenMegaYears, `"10M"`},
		{document.TimePrecisionMegaYears, `"M"`},
		{document.TimePrecisionHundredKiloYears, `"100k"`},
		{document.TimePrecisionTenKiloYears, `"10k"`},
		{document.TimePrecisionKiloYears, `"k"`},
		{document.TimePrecisionHundredYears, `"100y"`},
		{document.TimePrecisionTenYears, `"10y"`},
		{document.TimePrecisionYear, `"y"`},
		{document.TimePrecisionMonth, `"m"`},
		{document.TimePrecisionDay, `"d"`},
		{document.TimePrecisionHour, `"h"`},
		{document.TimePrecisionMinute, `"min"`},
		{document.TimePrecisionSecond, `"s"`},
		{document.TimePrecisionMillisecond, `"ms"`},
		{document.TimePrecisionMicrosecond, `"us"`},
		{document.TimePrecisionNanosecond, `"ns"`},
	}
	for _, test := range tests {
		t.Run(test.json, func(t *testing.T) {
			t.Parallel()

			out, errE := x.MarshalWithoutEscapeHTML(test.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, []byte(test.json), out)

			var p document.TimePrecision
			errE = x.UnmarshalWithoutUnknownFields([]byte(test.json), &p)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, test.precision, p)
		})
	}
}

func TestNewTime(t *testing.T) {
	t.Parallel()

	newYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     time.Time
		precision document.TimePrecision
		location  *time.Location
		expected  string
	}{
		// nil location defaults to UTC.
		{
			name:      "nil location uses UTC",
			input:     time.Date(2025, 3, 15, 10, 30, 45, 0, time.UTC),
			precision: document.TimePrecisionSecond,
			location:  nil,
			expected:  "2025-03-15 10:30:45",
		},
		// Various precisions with UTC.
		{
			name:      "year precision",
			input:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionYear,
			location:  time.UTC,
			expected:  "2025",
		},
		{
			// year 2025 truncated to nearest billion (toward zero) = year 0.
			name:      "giga-years precision",
			input:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionGigaYears,
			location:  time.UTC,
			expected:  "0000",
		},
		{
			name:      "month precision",
			input:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionMonth,
			location:  time.UTC,
			expected:  "2025-06-00",
		},
		{
			name:      "day precision",
			input:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionDay,
			location:  time.UTC,
			expected:  "2025-06-15",
		},
		{
			name:      "hour precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 0, time.UTC),
			precision: document.TimePrecisionHour,
			location:  time.UTC,
			expected:  "2025-06-15 12:00",
		},
		{
			name:      "minute precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 0, time.UTC),
			precision: document.TimePrecisionMinute,
			location:  time.UTC,
			expected:  "2025-06-15 12:30",
		},
		{
			name:      "second precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 123456789, time.UTC),
			precision: document.TimePrecisionSecond,
			location:  time.UTC,
			expected:  "2025-06-15 12:30:45",
		},
		{
			name:      "millisecond precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 123456789, time.UTC),
			precision: document.TimePrecisionMillisecond,
			location:  time.UTC,
			expected:  "2025-06-15 12:30:45.123",
		},
		{
			name:      "microsecond precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 123456789, time.UTC),
			precision: document.TimePrecisionMicrosecond,
			location:  time.UTC,
			expected:  "2025-06-15 12:30:45.123456",
		},
		{
			name:      "nanosecond precision",
			input:     time.Date(2025, 6, 15, 12, 30, 45, 123456789, time.UTC),
			precision: document.TimePrecisionNanosecond,
			location:  time.UTC,
			expected:  "2025-06-15 12:30:45.123456789",
		},
		// Non-UTC timezone: New York is UTC-5 in January.
		{
			name:      "New York second (UTC input)",
			input:     time.Date(2025, 1, 15, 15, 30, 45, 0, time.UTC), // 10:30:45 EST
			precision: document.TimePrecisionSecond,
			location:  newYork,
			expected:  "2025-01-15 10:30:45",
		},
		{
			name:      "New York nanosecond (UTC input)",
			input:     time.Date(2025, 1, 15, 15, 30, 45, 123456789, time.UTC), // 10:30:45.123456789 EST
			precision: document.TimePrecisionNanosecond,
			location:  newYork,
			expected:  "2025-01-15 10:30:45.123456789",
		},
		// Tokyo is UTC+9.
		{
			name:      "Tokyo second (UTC input)",
			input:     time.Date(2025, 3, 15, 0, 30, 45, 0, time.UTC), // 09:30:45 JST
			precision: document.TimePrecisionSecond,
			location:  tokyo,
			expected:  "2025-03-15 09:30:45",
		},
		// Timezone crossing midnight: same instant, different local date.
		{
			// 2025-03-16 03:30 UTC = 2025-03-15 23:30 EDT (UTC-4, after DST started Mar 9).
			name:      "New York crosses midnight (day precision)",
			input:     time.Date(2025, 3, 16, 3, 30, 0, 0, time.UTC),
			precision: document.TimePrecisionDay,
			location:  newYork,
			expected:  "2025-03-15",
		},
		{
			// 2025-03-15 22:00 UTC = 2025-03-16 07:00 JST (UTC+9).
			name:      "Tokyo crosses midnight (day precision)",
			input:     time.Date(2025, 3, 15, 22, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionDay,
			location:  tokyo,
			expected:  "2025-03-16",
		},
		// Negative year.
		{
			name:      "negative year",
			input:     time.Date(-2025, 3, 15, 10, 30, 45, 0, time.UTC),
			precision: document.TimePrecisionSecond,
			location:  time.UTC,
			expected:  "-2025-03-15 10:30:45",
		},
		{
			name:      "negative year precision only",
			input:     time.Date(-10000, 6, 1, 0, 0, 0, 0, time.UTC),
			precision: document.TimePrecisionYear,
			location:  time.UTC,
			expected:  "-10000",
		},
		// Input time in a non-UTC location.
		{
			name:      "input already in New York timezone",
			input:     time.Date(2025, 1, 15, 10, 30, 45, 0, newYork),
			precision: document.TimePrecisionSecond,
			location:  newYork,
			expected:  "2025-01-15 10:30:45",
		},
		// nil vs UTC equivalence: 2025-03-15 is after DST (Mar 9), so NY is UTC-4 (EDT).
		// 10:30:45 EDT = 14:30:45 UTC.
		{
			name:      "nil same as UTC for non-UTC input",
			input:     time.Date(2025, 3, 15, 10, 30, 45, 0, newYork),
			precision: document.TimePrecisionSecond,
			location:  nil,
			expected:  "2025-03-15 14:30:45",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := document.NewTime(tc.input, tc.precision, tc.location)
			assert.Equal(t, document.Time(tc.expected), ts)
		})
	}
}

func TestYearPrecisionValidation(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		ts        string
		precision document.TimePrecision
	}{
		// Each precision requires the year to be divisible by the appropriate multiple.
		{"0000", document.TimePrecisionGigaYears},
		{"1000000000", document.TimePrecisionGigaYears},
		{"-1000000000", document.TimePrecisionGigaYears},
		{"0000", document.TimePrecisionHundredMegaYears},
		{"100000000", document.TimePrecisionHundredMegaYears},
		{"-100000000", document.TimePrecisionHundredMegaYears},
		{"0000", document.TimePrecisionTenMegaYears},
		{"10000000", document.TimePrecisionTenMegaYears},
		{"0000", document.TimePrecisionMegaYears},
		{"1000000", document.TimePrecisionMegaYears},
		{"-1000000", document.TimePrecisionMegaYears},
		{"0000", document.TimePrecisionHundredKiloYears},
		{"100000", document.TimePrecisionHundredKiloYears},
		{"0000", document.TimePrecisionTenKiloYears},
		{"10000", document.TimePrecisionTenKiloYears},
		{"0000", document.TimePrecisionKiloYears},
		{"1000", document.TimePrecisionKiloYears},
		{"-1000", document.TimePrecisionKiloYears},
		{"0000", document.TimePrecisionHundredYears},
		{"1900", document.TimePrecisionHundredYears},
		{"-1900", document.TimePrecisionHundredYears},
		{"0000", document.TimePrecisionTenYears},
		{"1920", document.TimePrecisionTenYears},
		{"1910", document.TimePrecisionTenYears},
		{"-1920", document.TimePrecisionTenYears},
		// Year precision accepts any year.
		{"2025", document.TimePrecisionYear},
		{"1925", document.TimePrecisionYear},
		{"-1925", document.TimePrecisionYear},
	}
	for _, tc := range validCases {
		t.Run(tc.ts+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Time(tc.ts).Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}

	invalidCases := []struct {
		ts        string
		precision document.TimePrecision
		errMsg    string
	}{
		// Years that are not divisible by the required multiple.
		{"2025", document.TimePrecisionGigaYears, "year not rounded to precision"},
		{"100000001", document.TimePrecisionHundredMegaYears, "year not rounded to precision"},
		{"10000001", document.TimePrecisionTenMegaYears, "year not rounded to precision"},
		{"1000001", document.TimePrecisionMegaYears, "year not rounded to precision"},
		{"100001", document.TimePrecisionHundredKiloYears, "year not rounded to precision"},
		{"10001", document.TimePrecisionTenKiloYears, "year not rounded to precision"},
		{"1001", document.TimePrecisionKiloYears, "year not rounded to precision"},
		{"1925", document.TimePrecisionHundredYears, "year not rounded to precision"},
		{"1925", document.TimePrecisionTenYears, "year not rounded to precision"},
		{"-1925", document.TimePrecisionTenYears, "year not rounded to precision"},
		{"1921", document.TimePrecisionTenYears, "year not rounded to precision"},
		{"1919", document.TimePrecisionTenYears, "year not rounded to precision"},
	}
	for _, tc := range invalidCases {
		t.Run(tc.ts+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Time(tc.ts).Validate(tc.precision)
			assert.EqualError(t, errE, tc.errMsg)
		})
	}
}

func TestNewTimeYearTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputYear int
		precision document.TimePrecision
		expected  string
	}{
		// Decade precision: truncate toward zero.
		{"decade 1925->1920", 1925, document.TimePrecisionTenYears, "1920"},
		{"decade 1920->1920", 1920, document.TimePrecisionTenYears, "1920"},
		{"decade 1929->1920", 1929, document.TimePrecisionTenYears, "1920"},
		{"decade 1930->1930", 1930, document.TimePrecisionTenYears, "1930"},
		{"decade -1925->-1920", -1925, document.TimePrecisionTenYears, "-1920"},
		{"decade -1920->-1920", -1920, document.TimePrecisionTenYears, "-1920"},
		{"decade -1929->-1920", -1929, document.TimePrecisionTenYears, "-1920"},
		{"decade -1930->-1930", -1930, document.TimePrecisionTenYears, "-1930"},
		{"decade 5->0", 5, document.TimePrecisionTenYears, "0000"},
		{"decade -5->0", -5, document.TimePrecisionTenYears, "0000"},
		// Century precision.
		{"century 1925->1900", 1925, document.TimePrecisionHundredYears, "1900"},
		{"century 1900->1900", 1900, document.TimePrecisionHundredYears, "1900"},
		{"century 1999->1900", 1999, document.TimePrecisionHundredYears, "1900"},
		{"century -1925->-1900", -1925, document.TimePrecisionHundredYears, "-1900"},
		// Kilo-year precision.
		{"kilo 1500->1000", 1500, document.TimePrecisionKiloYears, "1000"},
		{"kilo 2025->2000", 2025, document.TimePrecisionKiloYears, "2000"},
		{"kilo -1500->-1000", -1500, document.TimePrecisionKiloYears, "-1000"},
		// Mega-year precision.
		{"mega 2025->0", 2025, document.TimePrecisionMegaYears, "0000"},
		{"mega 1500000->1000000", 1500000, document.TimePrecisionMegaYears, "1000000"},
		// Giga-year precision.
		{"giga 2025->0", 2025, document.TimePrecisionGigaYears, "0000"},
		{"giga 1500000000->1000000000", 1500000000, document.TimePrecisionGigaYears, "1000000000"},
		{"giga -1500000000->-1000000000", -1500000000, document.TimePrecisionGigaYears, "-1000000000"},
		// Year precision: no truncation.
		{"year 1925->1925", 1925, document.TimePrecisionYear, "1925"},
		{"year -1925->-1925", -1925, document.TimePrecisionYear, "-1925"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := time.Date(tc.inputYear, 6, 15, 12, 30, 0, 0, time.UTC)
			ts := document.NewTime(input, tc.precision, time.UTC)
			assert.Equal(t, document.Time(tc.expected), ts)

			// Validate that the produced timestamp passes validation for its precision.
			errE := ts.Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}
}

func TestNewTimeRoundTrip(t *testing.T) {
	t.Parallel()

	newYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     time.Time
		precision document.TimePrecision
		location  *time.Location
	}{
		// UTC, all high precisions (exact round-trip).
		{"UTC second", time.Date(2025, 3, 15, 10, 30, 45, 0, time.UTC), document.TimePrecisionSecond, time.UTC},
		{"UTC millisecond", time.Date(2025, 3, 15, 10, 30, 45, 123000000, time.UTC), document.TimePrecisionMillisecond, time.UTC},
		{"UTC microsecond", time.Date(2025, 3, 15, 10, 30, 45, 123456000, time.UTC), document.TimePrecisionMicrosecond, time.UTC},
		{"UTC nanosecond", time.Date(2025, 3, 15, 10, 30, 45, 123456789, time.UTC), document.TimePrecisionNanosecond, time.UTC},
		// Non-UTC timezones (exact round-trip).
		{"New York second", time.Date(2025, 1, 15, 15, 30, 45, 0, time.UTC), document.TimePrecisionSecond, newYork},
		{"New York nanosecond", time.Date(2025, 1, 15, 15, 30, 45, 123456789, time.UTC), document.TimePrecisionNanosecond, newYork},
		{"Tokyo second", time.Date(2025, 3, 15, 0, 30, 45, 0, time.UTC), document.TimePrecisionSecond, tokyo},
		{"Tokyo nanosecond", time.Date(2025, 3, 15, 0, 30, 45, 123456789, time.UTC), document.TimePrecisionNanosecond, tokyo},
		// Input already in a non-UTC location.
		{"input in New York timezone", time.Date(2025, 1, 15, 10, 30, 45, 0, newYork), document.TimePrecisionSecond, newYork},
		// Nil location (UTC).
		{"nil location second", time.Date(2025, 3, 15, 10, 30, 45, 0, time.UTC), document.TimePrecisionSecond, nil},
		// Crossing midnight.
		{"New York crosses midnight", time.Date(2025, 3, 16, 3, 30, 45, 0, time.UTC), document.TimePrecisionSecond, newYork},
		// Negative year.
		{"negative year nanosecond", time.Date(-2025, 3, 15, 10, 30, 45, 123456789, time.UTC), document.TimePrecisionNanosecond, time.UTC},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := document.NewTime(tc.input, tc.precision, tc.location)
			errE := ts.Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)

			// Pass tc.location directly (nil is allowed and defaults to UTC).
			t2, errE := ts.Time(tc.precision, tc.location)
			require.NoError(t, errE, "% -+#.1v", errE)

			// Round-trip: the parsed time must represent the same instant.
			assert.True(t, tc.input.Equal(t2), "expected %v, got %v", tc.input, t2)
		})
	}
}

// TestTimeMarshalText tests Time.MarshalText.
func TestTimeMarshalText(t *testing.T) {
	t.Parallel()

	ts := document.Time("2025-03-17 10:30:00")
	text, err := ts.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("2025-03-17 10:30:00"), text)

	var ts2 document.Time
	err = ts2.UnmarshalText(text)
	require.NoError(t, err)
	assert.Equal(t, ts, ts2)
}

// TestTimePrecisionMarshalText tests TimePrecision.MarshalText.
func TestTimePrecisionMarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		precision document.TimePrecision
		expected  string
	}{
		{document.TimePrecisionYear, "y"},
		{document.TimePrecisionDay, "d"},
		{document.TimePrecisionNanosecond, "ns"},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			t.Parallel()

			text, err := test.precision.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, []byte(test.expected), text)
		})
	}
}

// TestTimeLeapYear tests leap year handling in Time.Time via isLeap and daysIn.
func TestTimeLeapYear(t *testing.T) {
	t.Parallel()

	// Feb 29 in a 400-year leap (year 2000).
	ts := document.Time("2000-02-29")
	errE := ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a regular quadrennial leap year (2004).
	ts = document.Time("2004-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a century non-leap year (1900).
	ts = document.Time("1900-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "day out of range")

	// Feb 28 in a non-leap year is valid.
	ts = document.Time("2001-02-28")
	errE = ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a non-leap year (2001) is invalid.
	ts = document.Time("2001-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "day out of range")
}

// TestTimeValidateExtraCases tests additional Time.Validate branches.
func TestTimeValidateExtraCases(t *testing.T) {
	t.Parallel()

	// "day not allowed for precision": month precision with non-zero day.
	errE := document.Time("2025-03-15").Validate(document.TimePrecisionMonth)
	assert.EqualError(t, errE, "day not allowed for precision")

	// "hours and minutes not allowed for precision": day precision with hours present.
	errE = document.Time("2025-03-15 10:00").Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "hours and minutes not allowed for precision")

	// "seconds not allowed for precision": minute precision with seconds present.
	errE = document.Time("2025-03-15 10:30:45").Validate(document.TimePrecisionMinute)
	assert.EqualError(t, errE, "seconds not allowed for precision")

	// "subseconds not allowed for precision": second precision with subseconds present.
	errE = document.Time("2025-03-15 10:30:45.123").Validate(document.TimePrecisionSecond)
	assert.EqualError(t, errE, "subseconds not allowed for precision")
}

// TestTimePrecisionStringDefault tests TimePrecision.String for an unknown precision.
func TestTimePrecisionStringDefault(t *testing.T) {
	t.Parallel()

	p := document.TimePrecision(999)
	s := p.String()
	assert.Equal(t, "[999]", s)
}

// TestTimePrecisionUnmarshalTextUnknown tests TimePrecision.UnmarshalText with an unknown string.
func TestTimePrecisionUnmarshalTextUnknown(t *testing.T) {
	t.Parallel()

	var p document.TimePrecision
	errE := p.UnmarshalText([]byte("xyz"))
	assert.EqualError(t, errE, "unknown time precision")
}

// TestTimeTimeNilLocation tests that Time.Time with a nil location defaults to UTC.
func TestTimeTimeNilLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ts        string
		precision document.TimePrecision
	}{
		{"year", "2025", document.TimePrecisionYear},
		{"month", "2025-06-00", document.TimePrecisionMonth},
		{"day", "2025-06-15", document.TimePrecisionDay},
		{"hour", "2025-06-15 12:00", document.TimePrecisionHour},
		{"minute", "2025-06-15 12:30", document.TimePrecisionMinute},
		{"second", "2025-06-15 12:30:45", document.TimePrecisionSecond},
		{"millisecond", "2025-06-15 12:30:45.123", document.TimePrecisionMillisecond},
		{"microsecond", "2025-06-15 12:30:45.123456", document.TimePrecisionMicrosecond},
		{"nanosecond", "2025-06-15 12:30:45.123456789", document.TimePrecisionNanosecond},
		{"negative year", "-2025-03-15 10:30:45", document.TimePrecisionSecond},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := document.Time(tc.ts)

			tNil, errE := ts.Time(tc.precision, nil)
			require.NoError(t, errE, "% -+#.1v", errE)

			tUTC, errE := ts.Time(tc.precision, time.UTC)
			require.NoError(t, errE, "% -+#.1v", errE)

			// nil location must behave identically to time.UTC.
			assert.True(t, tNil.Equal(tUTC), "expected %v, got %v", tUTC, tNil)
			assert.Equal(t, tUTC.Location(), tNil.Location())
		})
	}
}

// TestTimeUnmarshalErrors tests error paths in Time.UnmarshalText and UnmarshalJSON.
func TestTimeUnmarshalErrors(t *testing.T) {
	t.Parallel()

	t.Run("unmarshal_text_invalid", func(t *testing.T) {
		t.Parallel()
		var ts document.Time
		err := ts.UnmarshalText([]byte("not-a-time"))
		assert.EqualError(t, err, "unable to parse time")
	})

	t.Run("unmarshal_json_non_string", func(t *testing.T) {
		t.Parallel()
		var ts document.Time
		err := ts.UnmarshalJSON([]byte("123"))
		assert.EqualError(t, err, "json: cannot unmarshal number into Go value of type string")
	})
}

// TestTimePrecisionUnmarshalJSONBadJSON tests TimePrecision.UnmarshalJSON with non-string JSON.
func TestTimePrecisionUnmarshalJSONBadJSON(t *testing.T) {
	t.Parallel()

	var p document.TimePrecision
	err := p.UnmarshalJSON([]byte("123"))
	assert.EqualError(t, err, "json: cannot unmarshal number into Go value of type string")
}

// TestTimeWindowEndFloat64 covers the natural-step behavior of
// Time.WindowEndFloat64 at modern timestamps for every supported precision.
func TestTimeWindowEndFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ts          document.Time
		precision   document.TimePrecision
		expectedEnd time.Time
	}{
		{"year", "2024", document.TimePrecisionYear, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"month", "2024-01-00", document.TimePrecisionMonth, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"day", "2024-01-01", document.TimePrecisionDay, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"hour", "2024-01-01 00:00", document.TimePrecisionHour, time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)},
		{"minute", "2024-01-01 00:00", document.TimePrecisionMinute, time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC)},
		{"second", "2024-01-01 00:00:00", document.TimePrecisionSecond, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"ten years", "2020", document.TimePrecisionTenYears, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"hundred years", "2000", document.TimePrecisionHundredYears, time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"millisecond", "2024-01-01 00:00:00.000", document.TimePrecisionMillisecond, time.Date(2024, 1, 1, 0, 0, 0, 1_000_000, time.UTC)},
		{"microsecond", "2024-01-01 00:00:00.000000", document.TimePrecisionMicrosecond, time.Date(2024, 1, 1, 0, 0, 0, 1_000, time.UTC)},
		// Nanosecond is widened to microsecond because float64 cannot
		// distinguish 1 ns at modern unix timestamps.
		{"nanosecond", "2024-01-01 00:00:00.000000000", document.TimePrecisionNanosecond, time.Date(2024, 1, 1, 0, 0, 0, 1_000, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			endF, errE := tt.ts.WindowEndFloat64(tt.precision, false)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, x.TimeToFloat64(tt.expectedEnd), endF)  //nolint:testifylint
			// WindowStartFloat64 should match the parsed ts (= start of window).
			parsed, errE := tt.ts.Time(tt.precision, time.UTC)
			require.NoError(t, errE, "% -+#.1v", errE)
			startF, errE := tt.ts.WindowStartFloat64(tt.precision, false)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, x.TimeToFloat64(parsed), startF)        //nolint:testifylint
		})
	}
}

// TestTimeWindowEndFloat64MagnitudeWidening verifies that the precision
// step is widened to the next coarser precision when the natural step is
// below float64 resolution at t's magnitude.
func TestTimeWindowEndFloat64MagnitudeWidening(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ts        document.Time
		precision document.TimePrecision
		expected  time.Time
	}{
		// Near-epoch (unix ~86400, ULP ~1.9e-11 s): 1 ns survives, no widening.
		{
			"near-epoch nanosecond stays at 1 ns",
			"1970-01-02 00:00:00.000000000",
			document.TimePrecisionNanosecond,
			time.Date(1970, 1, 2, 0, 0, 0, 1, time.UTC),
		},
		{
			"near-epoch microsecond stays at 1 µs",
			"1970-01-02 00:00:00.000000",
			document.TimePrecisionMicrosecond,
			time.Date(1970, 1, 2, 0, 0, 0, 1_000, time.UTC),
		},
		// Modern (unix ~1.7e9, ULP ~3.8e-7 s): 1 ns widened to 1 µs.
		{
			"modern nanosecond widens to 1 µs",
			"2024-01-01 00:00:00.000000000",
			document.TimePrecisionNanosecond,
			time.Date(2024, 1, 1, 0, 0, 0, 1_000, time.UTC),
		},
		// Far future (unix ~3.25e10, ULP ~7 µs): 1 ns and 1 µs widen to 1 ms.
		{
			"year 3000 nanosecond widens to 1 ms",
			"3000-01-01 00:00:00.000000000",
			document.TimePrecisionNanosecond,
			time.Date(3000, 1, 1, 0, 0, 0, 1_000_000, time.UTC),
		},
		{
			"year 3000 microsecond widens to 1 ms",
			"3000-01-01 00:00:00.000000",
			document.TimePrecisionMicrosecond,
			time.Date(3000, 1, 1, 0, 0, 0, 1_000_000, time.UTC),
		},
		// Year 50 million (unix ~1.58e15, ULP ~0.25 s): 1 ms widens to 1 s.
		{
			"year 50 million millisecond widens to 1 s",
			"50000000-01-01 00:00:00.000",
			document.TimePrecisionMillisecond,
			time.Date(50_000_000, 1, 1, 0, 0, 1, 0, time.UTC),
		},
		// Year 500 million (unix ~1.58e16, ULP ~2 s): 1 s widens to 1 minute,
		// crossing the sub-second boundary.
		{
			"year 500 million second widens to 1 minute",
			"500000000-01-01 00:00:00",
			document.TimePrecisionSecond,
			time.Date(500_000_000, 1, 1, 0, 1, 0, 0, time.UTC),
		},
		// Year 290 billion (near Go's time.Time upper limit, unix ~9.15e18,
		// ULP ~1024 s): everything sub-hour widens up to hour.
		{
			"year 290 billion second widens to 1 hour",
			"290000000000-01-01 00:00:00",
			document.TimePrecisionSecond,
			time.Date(290_000_000_000, 1, 1, 1, 0, 0, 0, time.UTC),
		},
		{
			"year 290 billion hour stays at 1 hour",
			"290000000000-01-01 00:00",
			document.TimePrecisionHour,
			time.Date(290_000_000_000, 1, 1, 1, 0, 0, 0, time.UTC),
		},
		{
			"year 290 billion giga-years stays at 1 Gy",
			"290000000000",
			document.TimePrecisionGigaYears,
			time.Date(291_000_000_000, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			endF, errE := tt.ts.WindowEndFloat64(tt.precision, false)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, x.TimeToFloat64(tt.expected), endF)  //nolint:testifylint
		})
	}
}
