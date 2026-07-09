package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
)

func TestTimeMarshalJSON(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-03-15T10:30:45.123Z"`, string(b))
}

func TestTimeMarshalJSONZeroMillis(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-01-01T00:00:00.000Z"`, string(b))
}

func TestTimeUnmarshalJSON(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"2024-03-15T10:30:45.123Z"`), &st)
	require.NoError(t, err)

	expected := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	assert.True(t, expected.Equal(time.Time(st)))
}

func TestTimeUnmarshalJSONNull(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`null`), &st)
	require.NoError(t, err)
	assert.True(t, time.Time(st).IsZero())
}

func TestTimeUnmarshalJSONInvalid(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`12345`), &st)
	assert.EqualError(t, err, "Time.UnmarshalJSON: input is not a JSON string")
}

func TestTimeUnmarshalJSONBadFormat(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"not-a-date"`), &st)
	assert.EqualError(t, err, `parsing time "not-a-date" as "2006-01-02T15:04:05.000Z07:00": cannot parse "not-a-date" as "2006"`)
}

func TestTimeMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC)
	original := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(original)
	require.NoError(t, err)

	var decoded store.Time
	err = x.UnmarshalWithoutUnknownFields(b, &decoded)
	require.NoError(t, err)

	assert.True(t, time.Time(original).Equal(time.Time(decoded)))
}

func TestTimeMarshalWithTimezone(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("EST", -5*60*60)
	ts := time.Date(2024, 6, 15, 14, 30, 0, 0, loc)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-06-15T14:30:00.000-05:00"`, string(b))
}

// Pinned identifier used by Version tests so that parsing and formatting are
// fully deterministic.
const versionTestChangesetStr = "11111111111111111111AB"

func TestVersionStringWithRevision(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	v := store.Version{Changeset: cs, Revision: 7}
	assert.Equal(t, versionTestChangesetStr+"-7", v.String())
}

func TestVersionStringRevisionZero(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	v := store.Version{Changeset: cs, Revision: 0}
	// Zero revision still serializes with explicit "-0" suffix.
	assert.Equal(t, versionTestChangesetStr+"-0", v.String())
}

func TestVersionMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	for _, rev := range []int64{0, 1, 2, 100, 1<<62 - 1} {
		original := store.Version{Changeset: cs, Revision: rev}

		b, err := original.MarshalText()
		require.NoError(t, err)

		var decoded store.Version
		err = decoded.UnmarshalText(b)
		require.NoError(t, err)

		assert.Equal(t, original, decoded)
	}
}

func TestVersionFromStringWithoutDash(t *testing.T) {
	t.Parallel()

	// No "-" -> revision defaults to 0 (meaning "latest" in read paths).
	v, errE := store.VersionFromString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(0), v.Revision)
}

func TestVersionFromStringWithExplicitRevision(t *testing.T) {
	t.Parallel()

	v, errE := store.VersionFromString(versionTestChangesetStr + "-3")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(3), v.Revision)
}

func TestVersionFromStringExplicitZero(t *testing.T) {
	t.Parallel()

	v, errE := store.VersionFromString(versionTestChangesetStr + "-0")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(0), v.Revision)
}

func TestVersionFromStringEmptyRevisionAfterDash(t *testing.T) {
	t.Parallel()

	// "<cs>-" parses revisionStr as "" which fails strconv.ParseInt.
	_, errE := store.VersionFromString(versionTestChangesetStr + "-")
	require.Error(t, errE)
}

func TestVersionFromStringDoubleDash(t *testing.T) {
	t.Parallel()

	// strings.Cut splits on the first "-" so revisionStr is "-3", which parses
	// to a negative integer and is rejected.
	_, errE := store.VersionFromString(versionTestChangesetStr + "--3")
	require.Error(t, errE)
	assert.Contains(t, errE.Error(), "invalid version revision")
}

func TestVersionFromStringBadChangeset(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString("not-an-identifier")
	require.Error(t, errE)
}

func TestVersionFromStringEmpty(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString("")
	require.Error(t, errE)
}

func TestVersionFromStringOverflow(t *testing.T) {
	t.Parallel()

	// Revision larger than int64.
	_, errE := store.VersionFromString(versionTestChangesetStr + "-99999999999999999999")
	require.Error(t, errE)
}

func TestVersionFromStringNegativeRevision(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString(versionTestChangesetStr + "-" + "-1")
	require.Error(t, errE)
	// "-1" -> invalid (we reject negative).
	assert.Contains(t, errE.Error(), "invalid version revision")
}

func TestVersionUnmarshalTextRoundTripThroughMarshalText(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	original := store.Version{Changeset: cs, Revision: 42}

	b, err := original.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, versionTestChangesetStr+"-42", string(b))

	var decoded store.Version
	err = decoded.UnmarshalText(b)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}
