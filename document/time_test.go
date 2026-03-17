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
		{"2025", document.TimePrecisionGigaYears},
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
