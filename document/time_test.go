package document_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

func TestTimestampMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		timestamp string
		unix      int64
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
		t.Run(test.timestamp, func(t *testing.T) {
			t.Parallel()

			var timestamp document.Timestamp
			in := []byte(test.timestamp)
			errE := x.UnmarshalWithoutUnknownFields(in, &timestamp)
			require.NoError(t, errE, "% -+#.1v", errE)
			tt, errE := timestamp.Time(0, time.UTC)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, test.unix, tt.Unix())
			out, errE := x.MarshalWithoutEscapeHTML(timestamp)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, in, out)
		})
	}
}

func TestTimestampValidation(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		timestamp string
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
		t.Run(tc.timestamp+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Timestamp(tc.timestamp).Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}

	invalidCases := []struct {
		timestamp string
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
		{"not-a-timestamp", document.TimePrecisionYear, "unable to parse timestamp"},
	}
	for _, tc := range invalidCases {
		t.Run(tc.timestamp+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Timestamp(tc.timestamp).Validate(tc.precision)
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

func TestNewTimestamp(t *testing.T) {
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

			ts := document.NewTimestamp(tc.input, tc.precision, tc.location)
			assert.Equal(t, document.Timestamp(tc.expected), ts)
		})
	}
}

func TestYearPrecisionValidation(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		timestamp string
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
		t.Run(tc.timestamp+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Timestamp(tc.timestamp).Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}

	invalidCases := []struct {
		timestamp string
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
		t.Run(tc.timestamp+"/"+tc.precision.String(), func(t *testing.T) {
			t.Parallel()

			errE := document.Timestamp(tc.timestamp).Validate(tc.precision)
			assert.EqualError(t, errE, tc.errMsg)
		})
	}
}

func TestNewTimestampYearTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputYear int
		precision document.TimePrecision
		expected  string
	}{
		// Decade precision: truncate toward zero.
		{"decade 1925→1920", 1925, document.TimePrecisionTenYears, "1920"},
		{"decade 1920→1920", 1920, document.TimePrecisionTenYears, "1920"},
		{"decade 1929→1920", 1929, document.TimePrecisionTenYears, "1920"},
		{"decade 1930→1930", 1930, document.TimePrecisionTenYears, "1930"},
		{"decade -1925→-1920", -1925, document.TimePrecisionTenYears, "-1920"},
		{"decade -1920→-1920", -1920, document.TimePrecisionTenYears, "-1920"},
		{"decade -1929→-1920", -1929, document.TimePrecisionTenYears, "-1920"},
		{"decade -1930→-1930", -1930, document.TimePrecisionTenYears, "-1930"},
		{"decade 5→0", 5, document.TimePrecisionTenYears, "0000"},
		{"decade -5→0", -5, document.TimePrecisionTenYears, "0000"},
		// Century precision.
		{"century 1925→1900", 1925, document.TimePrecisionHundredYears, "1900"},
		{"century 1900→1900", 1900, document.TimePrecisionHundredYears, "1900"},
		{"century 1999→1900", 1999, document.TimePrecisionHundredYears, "1900"},
		{"century -1925→-1900", -1925, document.TimePrecisionHundredYears, "-1900"},
		// Kilo-year precision.
		{"kilo 1500→1000", 1500, document.TimePrecisionKiloYears, "1000"},
		{"kilo 2025→2000", 2025, document.TimePrecisionKiloYears, "2000"},
		{"kilo -1500→-1000", -1500, document.TimePrecisionKiloYears, "-1000"},
		// Mega-year precision.
		{"mega 2025→0", 2025, document.TimePrecisionMegaYears, "0000"},
		{"mega 1500000→1000000", 1500000, document.TimePrecisionMegaYears, "1000000"},
		// Giga-year precision.
		{"giga 2025→0", 2025, document.TimePrecisionGigaYears, "0000"},
		{"giga 1500000000→1000000000", 1500000000, document.TimePrecisionGigaYears, "1000000000"},
		{"giga -1500000000→-1000000000", -1500000000, document.TimePrecisionGigaYears, "-1000000000"},
		// Year precision: no truncation.
		{"year 1925→1925", 1925, document.TimePrecisionYear, "1925"},
		{"year -1925→-1925", -1925, document.TimePrecisionYear, "-1925"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := time.Date(tc.inputYear, 6, 15, 12, 30, 0, 0, time.UTC)
			ts := document.NewTimestamp(input, tc.precision, time.UTC)
			assert.Equal(t, document.Timestamp(tc.expected), ts)

			// Validate that the produced timestamp passes validation for its precision.
			errE := ts.Validate(tc.precision)
			require.NoError(t, errE, "% -+#.1v", errE)
		})
	}
}

func TestNewTimestampRoundTrip(t *testing.T) {
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

			ts := document.NewTimestamp(tc.input, tc.precision, tc.location)
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
