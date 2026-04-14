package export_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

//nolint:gochecknoglobals
var (
	docID1 = identifier.From("test", "doc1")
	docID2 = identifier.From("test", "doc2")
	docID3 = identifier.From("test", "doc3")
	langEN = identifier.From("test", "lang", "en")
	langSL = identifier.From("test", "lang", "sl")
)

func ptrID(id identifier.Identifier) *identifier.Identifier {
	return &id
}

func ref(id identifier.Identifier) document.Reference {
	return document.Reference{ID: id}
}

func makeNameCache() *export.NameCache {
	nc := export.NewNameCache(func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	})
	nc.Names[internalCore.NamePropID] = "NAME"
	nc.Names[internalCore.MnemonicPropID] = "MNEMONIC"
	nc.Names[internalCore.DescriptionPropID] = "DESCRIPTION"
	nc.Names[internalCore.InLanguagePropID] = "IN_LANGUAGE"
	nc.Names[internalCore.InstanceOfPropID] = "INSTANCE_OF"
	nc.Names[internalCore.FieldsPropID] = "FIELDS"
	nc.Names[internalCore.FieldPropID] = "FIELD"
	nc.Names[internalCore.CardinalityPropID] = "CARDINALITY"
	return nc
}

func TestColumnKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "A", export.ColumnKey([]string{"A"}))
	assert.Equal(t, "A\x00B", export.ColumnKey([]string{"A", "B"}))
	assert.Equal(t, "A\x00B\x00C", export.ColumnKey([]string{"A", "B", "C"}))
}

func TestColumnCSVName(t *testing.T) {
	t.Parallel()

	c := export.Column{Path: []string{"FIELDS", "FIELD", "CARDINALITY"}, IsHas: false}
	assert.Equal(t, "FIELDS.FIELD.CARDINALITY", c.CSVName())

	c2 := export.Column{Path: []string{"NAME"}, IsHas: false}
	assert.Equal(t, "NAME", c2.CSVName())
}

func TestResolveID_Mnemonic(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"NAME": internalCore.NamePropID}
	id, errE := export.ResolveID("NAME", mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, internalCore.NamePropID, id)
}

func TestResolveID_ValidID(t *testing.T) {
	t.Parallel()

	id, errE := export.ResolveID(docID1.String(), map[string]identifier.Identifier{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, docID1, id)
}

func TestResolveID_Invalid(t *testing.T) {
	t.Parallel()

	_, errE := export.ResolveID("not_valid_at_all", map[string]identifier.Identifier{})
	require.Error(t, errE)
	assert.Contains(t, errE.Error(), "invalid identifier")
}

func TestResolveIDs(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"NAME": internalCore.NamePropID}
	ids, errE := export.ResolveIDs([]string{"NAME", docID1.String()}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, ids, 2)
	assert.Equal(t, internalCore.NamePropID, ids[0])
	assert.Equal(t, docID1, ids[1])
}

func TestResolveIDs_Error(t *testing.T) {
	t.Parallel()

	_, errE := export.ResolveIDs([]string{"invalid_mnemonic"}, map[string]identifier.Identifier{})
	require.Error(t, errE)
}

func seg(id identifier.Identifier) export.PathSegment {
	return export.PathSegment{ID: ptrID(id), Recursive: false}
}

func wildSeg() export.PathSegment {
	return export.PathSegment{ID: nil, Recursive: false}
}

func recSeg() export.PathSegment {
	return export.PathSegment{ID: nil, Recursive: true}
}

// allSpecs is the ** spec: match everything at all depths.
func allSpecs() []export.PropertySpec {
	return []export.PropertySpec{{Segments: []export.PathSegment{recSeg()}}}
}

func TestParsePropertySpecs_Empty(t *testing.T) {
	t.Parallel()

	// No --property flags: defaults to **.
	specs, errE := export.ParsePropertySpecs(nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 1)
	assert.Nil(t, specs[0].Segments[0].ID)
	assert.True(t, specs[0].Segments[0].Recursive)
}

func TestParsePropertySpecs_Wildcard(t *testing.T) {
	t.Parallel()

	specs, errE := export.ParsePropertySpecs([]string{"*"}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 1)
	assert.Nil(t, specs[0].Segments[0].ID)
	assert.False(t, specs[0].Segments[0].Recursive)
}

func TestParsePropertySpecs_RecursiveWildcard(t *testing.T) {
	t.Parallel()

	specs, errE := export.ParsePropertySpecs([]string{"**"}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 1)
	assert.Nil(t, specs[0].Segments[0].ID)
	assert.True(t, specs[0].Segments[0].Recursive)
}

func TestParsePropertySpecs_Simple(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"NAME": internalCore.NamePropID}
	specs, errE := export.ParsePropertySpecs([]string{"NAME"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 1)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.Equal(t, internalCore.NamePropID, *specs[0].Segments[0].ID)
}

func TestParsePropertySpecs_SubWildcard(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"NAME": internalCore.NamePropID}
	specs, errE := export.ParsePropertySpecs([]string{"NAME.*"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 2)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.Equal(t, internalCore.NamePropID, *specs[0].Segments[0].ID)
	assert.Nil(t, specs[0].Segments[1].ID)
}

func TestParsePropertySpecs_SubSpecific(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NAME":        internalCore.NamePropID,
		"IN_LANGUAGE": internalCore.InLanguagePropID,
	}
	specs, errE := export.ParsePropertySpecs([]string{"NAME.IN_LANGUAGE"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 2)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.Equal(t, internalCore.NamePropID, *specs[0].Segments[0].ID)
	require.NotNil(t, specs[0].Segments[1].ID)
	assert.Equal(t, internalCore.InLanguagePropID, *specs[0].Segments[1].ID)
}

func TestParsePropertySpecs_ThreeLevels(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"FIELDS":      internalCore.FieldsPropID,
		"FIELD":       internalCore.FieldPropID,
		"CARDINALITY": internalCore.CardinalityPropID,
	}
	specs, errE := export.ParsePropertySpecs([]string{"FIELDS.FIELD.CARDINALITY"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 3)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.Equal(t, internalCore.FieldsPropID, *specs[0].Segments[0].ID)
	require.NotNil(t, specs[0].Segments[1].ID)
	assert.Equal(t, internalCore.FieldPropID, *specs[0].Segments[1].ID)
	require.NotNil(t, specs[0].Segments[2].ID)
	assert.Equal(t, internalCore.CardinalityPropID, *specs[0].Segments[2].ID)
}

func TestParsePropertySpecs_WildcardMiddle(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NAME":        internalCore.NamePropID,
		"CARDINALITY": internalCore.CardinalityPropID,
	}
	specs, errE := export.ParsePropertySpecs([]string{"NAME.*.CARDINALITY"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 3)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.Nil(t, specs[0].Segments[1].ID)
	require.NotNil(t, specs[0].Segments[2].ID)
}

func TestParsePropertySpecs_LeadingWildcard(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"IN_LANGUAGE": internalCore.InLanguagePropID}
	specs, errE := export.ParsePropertySpecs([]string{"*.IN_LANGUAGE"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 2)
	assert.Nil(t, specs[0].Segments[0].ID)
	require.NotNil(t, specs[0].Segments[1].ID)
	assert.Equal(t, internalCore.InLanguagePropID, *specs[0].Segments[1].ID)
}

func TestParsePropertySpecs_RecursiveMiddle(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NAME":        internalCore.NamePropID,
		"CARDINALITY": internalCore.CardinalityPropID,
	}
	specs, errE := export.ParsePropertySpecs([]string{"NAME.**.CARDINALITY"}, mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, specs, 1)
	require.Len(t, specs[0].Segments, 3)
	require.NotNil(t, specs[0].Segments[0].ID)
	assert.True(t, specs[0].Segments[1].Recursive)
	assert.Nil(t, specs[0].Segments[1].ID)
	require.NotNil(t, specs[0].Segments[2].ID)
}

func TestParsePropertySpecs_Error(t *testing.T) {
	t.Parallel()

	_, errE := export.ParsePropertySpecs([]string{"invalid_prop"}, map[string]identifier.Identifier{})
	require.Error(t, errE)
}

func TestParsePropertySpecs_SubPropError(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{"NAME": internalCore.NamePropID}
	_, errE := export.ParsePropertySpecs([]string{"NAME.invalid_sub"}, mnemonics)
	require.Error(t, errE)
}

func TestMatchAtDepth_BareWildcard(t *testing.T) {
	t.Parallel()

	// * matches any property at this level, but no children.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{wildSeg()}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.NotNil(t, result.ChildSpecs)
	assert.Empty(t, result.ChildSpecs)
}

func TestMatchAtDepth_BareRecursiveWildcard(t *testing.T) {
	t.Parallel()

	// ** matches any property at any depth.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{recSeg()}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.NotNil(t, result.ChildSpecs)
	assert.NotEmpty(t, result.ChildSpecs)

	// Children also match anything.
	result2 := export.MatchAtDepth(internalCore.DescriptionPropID, result.ChildSpecs)
	assert.True(t, result2.Matched)
}

func TestMatchAtDepth_RecursiveWithTrailing(t *testing.T) {
	t.Parallel()

	// **.FOO: FOO at any depth.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{recSeg(), seg(internalCore.InLanguagePropID)}}}

	// Non-matching prop: not matched (value not emitted), but childSpecs non-empty for traversal.
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.False(t, result.Matched)
	require.NotEmpty(t, result.ChildSpecs)

	// Matching prop: matched (value emitted).
	result2 := export.MatchAtDepth(internalCore.InLanguagePropID, specs)
	assert.True(t, result2.Matched)
}

func TestMatchAtDepth_RecursiveMiddle(t *testing.T) {
	t.Parallel()

	// NAME.**.CARDINALITY: CARDINALITY at any depth under NAME.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.NamePropID),
		recSeg(),
		seg(internalCore.CardinalityPropID),
	}}}

	// Level 0: NAME matches (concrete match on first segment).
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.NotEmpty(t, result.ChildSpecs)

	// Level 1: FIELD not matched (value not emitted), but childSpecs propagates **.
	result2 := export.MatchAtDepth(internalCore.FieldPropID, result.ChildSpecs)
	assert.False(t, result2.Matched)
	require.NotEmpty(t, result2.ChildSpecs)

	// Level 2: CARDINALITY matches via ** + consuming CARDINALITY.
	result3 := export.MatchAtDepth(internalCore.CardinalityPropID, result2.ChildSpecs)
	assert.True(t, result3.Matched)

	// Level 2: something else not matched, but childSpecs keeps propagating.
	result4 := export.MatchAtDepth(internalCore.DescriptionPropID, result2.ChildSpecs)
	assert.False(t, result4.Matched)
	require.NotEmpty(t, result4.ChildSpecs)
}

func TestMatchAtDepth_ExactMatch_NoChildren(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID)}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.NotNil(t, result.ChildSpecs)
	assert.Empty(t, result.ChildSpecs)
}

func TestMatchAtDepth_NoMatch(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.DescriptionPropID)}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.False(t, result.Matched)
}

func TestMatchAtDepth_WithSubWildcard(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID), wildSeg()}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.Len(t, result.ChildSpecs, 1)
	require.Len(t, result.ChildSpecs[0].Segments, 1)
	assert.Nil(t, result.ChildSpecs[0].Segments[0].ID)
}

func TestMatchAtDepth_WithSubSpecific(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID), seg(internalCore.InLanguagePropID)}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.Len(t, result.ChildSpecs, 1)
	require.Len(t, result.ChildSpecs[0].Segments, 1)
	require.NotNil(t, result.ChildSpecs[0].Segments[0].ID)
	assert.Equal(t, internalCore.InLanguagePropID, *result.ChildSpecs[0].Segments[0].ID)
}

func TestMatchAtDepth_LeadingWildcard(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{wildSeg(), seg(internalCore.InLanguagePropID)}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.Len(t, result.ChildSpecs, 1)
}

func TestMatchAtDepth_ThreeSegments(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.FieldsPropID),
		seg(internalCore.FieldPropID),
		seg(internalCore.CardinalityPropID),
	}}}
	result := export.MatchAtDepth(internalCore.FieldsPropID, specs)
	assert.True(t, result.Matched)
	require.Len(t, result.ChildSpecs, 1)
	require.Len(t, result.ChildSpecs[0].Segments, 2)

	// Second level.
	result2 := export.MatchAtDepth(internalCore.FieldPropID, result.ChildSpecs)
	assert.True(t, result2.Matched)
	require.Len(t, result2.ChildSpecs, 1)
	require.Len(t, result2.ChildSpecs[0].Segments, 1)

	// Third level.
	result3 := export.MatchAtDepth(internalCore.CardinalityPropID, result2.ChildSpecs)
	assert.True(t, result3.Matched)
	require.NotNil(t, result3.ChildSpecs)
	assert.Empty(t, result3.ChildSpecs)

	// Non-matching at third level.
	result4 := export.MatchAtDepth(internalCore.NamePropID, result2.ChildSpecs)
	assert.False(t, result4.Matched)
}

func TestMatchAtDepth_WildcardMiddle(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.NamePropID),
		wildSeg(),
		seg(internalCore.CardinalityPropID),
	}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.True(t, result.Matched)
	require.Len(t, result.ChildSpecs, 1)

	// Any property matches at second level.
	result2 := export.MatchAtDepth(internalCore.FieldPropID, result.ChildSpecs)
	assert.True(t, result2.Matched)
	require.Len(t, result2.ChildSpecs, 1)

	// Only CARDINALITY matches at third level.
	result3 := export.MatchAtDepth(internalCore.CardinalityPropID, result2.ChildSpecs)
	assert.True(t, result3.Matched)
	result4 := export.MatchAtDepth(internalCore.NamePropID, result2.ChildSpecs)
	assert.False(t, result4.Matched)
}

func TestMatchAtDepth_DifferentPropNoMatch(t *testing.T) {
	t.Parallel()

	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.DescriptionPropID), wildSeg()}}}
	result := export.MatchAtDepth(internalCore.NamePropID, specs)
	assert.False(t, result.Matched)
}

func TestClaimValue_AllTypes(t *testing.T) {
	t.Parallel()

	amount := document.Amount("42")
	ti := document.Time("2024-01-01 00:00:00")

	tests := []struct {
		name  string
		claim document.Claim
		want  string
	}{
		{
			"identifier",
			&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				Value:     "Q42",
			},
			"Q42",
		},
		{
			"string",
			&document.StringClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				String:    "hello",
			},
			"hello",
		},
		{
			"html",
			&document.HTMLClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				HTML:      "<b>bold</b>",
			},
			"<b>bold</b>",
		},
		{
			"amount",
			&document.AmountClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				Amount:    amount,
				Precision: 1,
			},
			"42",
		},
		{
			"time",
			&document.TimeClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				Time:      ti,
				Precision: document.TimePrecisionSecond,
			},
			"2024-01-01 00:00:00",
		},
		{
			"link",
			&document.LinkClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				IRI:       "https://example.com",
			},
			"https://example.com",
		},
		{
			"reference",
			&document.ReferenceClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
				To:        ref(docID1),
			},
			docID1.String(),
		},
		{
			"none",
			&document.NoneClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
			},
			export.NoneValue,
		},
		{
			"unknown",
			&document.UnknownClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
			},
			export.UnknownValue,
		},
		{
			"has",
			&document.HasClaim{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:      ref(internalCore.NamePropID),
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, export.ClaimValue(tt.claim))
		})
	}
}

func ptrAmount(s string) *document.Amount {
	a := document.Amount(s)
	return &a
}

func ptrFloat(f float64) *float64 {
	return &f
}

func ptrTime(s string) *document.Time {
	t := document.Time(s)
	return &t
}

func ptrTimePrecision(p document.TimePrecision) *document.TimePrecision {
	return &p
}

func TestFormatAmountInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		c    *document.AmountIntervalClaim
		want string
	}{
		{
			"closed",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrAmount("1"),
				FromPrecision: ptrFloat(1),
				To:            ptrAmount("10"),
				ToPrecision:   ptrFloat(1),
			},
			"[1, 10]",
		},
		{
			"closed open",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrAmount("1"),
				FromPrecision: ptrFloat(1),
				To:            ptrAmount("10"),
				ToPrecision:   ptrFloat(1),
				ToIsOpen:      true,
			},
			"[1, 10)",
		},
		{
			"open closed",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrAmount("1"),
				FromPrecision: ptrFloat(1),
				FromIsOpen:    true,
				To:            ptrAmount("10"),
				ToPrecision:   ptrFloat(1),
			},
			"(1, 10]",
		},
		{
			"to none",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrAmount("1"),
				FromPrecision: ptrFloat(1),
				ToIsNone:      true,
			},
			"[1, " + export.NoneValue + "]",
		},
		{
			"from unknown",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				FromIsUnknown: true,
				To:            ptrAmount("5"),
				ToPrecision:   ptrFloat(1),
			},
			"[" + export.UnknownValue + ", 5]",
		},
		{
			"to unknown",
			&document.AmountIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrAmount("1"),
				FromPrecision: ptrFloat(1),
				ToIsUnknown:   true,
			},
			"[1, " + export.UnknownValue + "]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errE := tt.c.Validate()
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tt.want, export.ClaimValue(tt.c))
		})
	}
}

func TestFormatTimeInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		c    *document.TimeIntervalClaim
		want string
	}{
		{
			"closed",
			&document.TimeIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrTime("2020-01-01 00:00:00"),
				FromPrecision: ptrTimePrecision(document.TimePrecisionSecond),
				To:            ptrTime("2025-01-01 00:00:00"),
				ToPrecision:   ptrTimePrecision(document.TimePrecisionSecond),
			},
			"[2020-01-01 00:00:00, 2025-01-01 00:00:00]",
		},
		{
			"open closed",
			&document.TimeIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrTime("2020-01-01 00:00:00"),
				FromPrecision: ptrTimePrecision(document.TimePrecisionSecond),
				FromIsOpen:    true,
				To:            ptrTime("2025-01-01 00:00:00"),
				ToPrecision:   ptrTimePrecision(document.TimePrecisionSecond),
			},
			"(2020-01-01 00:00:00, 2025-01-01 00:00:00]",
		},
		{
			"from none to none",
			&document.TimeIntervalClaim{ //nolint:exhaustruct
				CoreClaim:  document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:       ref(internalCore.NamePropID),
				FromIsNone: true,
				ToIsNone:   true,
			},
			"[" + export.NoneValue + ", " + export.NoneValue + "]",
		},
		{
			"from unknown",
			&document.TimeIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				FromIsUnknown: true,
				To:            ptrTime("2025-01-01 00:00:00"),
				ToPrecision:   ptrTimePrecision(document.TimePrecisionSecond),
			},
			"[" + export.UnknownValue + ", 2025-01-01 00:00:00]",
		},
		{
			"to unknown",
			&document.TimeIntervalClaim{ //nolint:exhaustruct
				CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
				Prop:          ref(internalCore.NamePropID),
				From:          ptrTime("2020-01-01 00:00:00"),
				FromPrecision: ptrTimePrecision(document.TimePrecisionSecond),
				ToIsUnknown:   true,
			},
			"[2020-01-01 00:00:00, " + export.UnknownValue + "]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errE := tt.c.Validate()
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tt.want, export.ClaimValue(tt.c))
		})
	}
}

func TestFormatTimeBound_Unknown(t *testing.T) {
	t.Parallel()

	c := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
		Prop:          ref(internalCore.NamePropID),
		FromIsUnknown: true,
		To:            ptrTime("2025-01-01 00:00:00"),
		ToPrecision:   ptrTimePrecision(document.TimePrecisionSecond),
	}
	errE := c.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "["+export.UnknownValue+", 2025-01-01 00:00:00]", export.ClaimValue(c))
}

func TestFormatTimeBound_ToUnknown(t *testing.T) {
	t.Parallel()

	c := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil},
		Prop:          ref(internalCore.NamePropID),
		From:          ptrTime("2020-01-01 00:00:00"),
		FromPrecision: ptrTimePrecision(document.TimePrecisionSecond),
		ToIsUnknown:   true,
	}
	errE := c.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "[2020-01-01 00:00:00, "+export.UnknownValue+"]", export.ClaimValue(c))
}

func TestFormatAmountBound_NilVal(t *testing.T) {
	t.Parallel()

	assert.Empty(t, export.FormatAmountBound(nil, false, false))
}

func TestFormatTimeBound_NilVal(t *testing.T) {
	t.Parallel()

	assert.Empty(t, export.FormatTimeBound(nil, false, false))
}

func TestBuildMnemonicMap(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.MnemonicPropID), String: "TEST_PROP"},
		},
	})
	m := export.BuildMnemonicMap([]*document.D{doc})
	assert.Equal(t, docID1, m["TEST_PROP"])
}

func TestBuildMnemonicMap_NoMnemonic(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "just a name"},
		},
	})
	m := export.BuildMnemonicMap([]*document.D{doc})
	assert.Empty(t, m)
}

func TestDisplayNameFromDoc_Mnemonic(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.MnemonicPropID), String: "MY_MNEMONIC"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "My Name"},
		},
	})
	assert.Equal(t, "MY_MNEMONIC", export.DisplayNameFromDoc(doc))
}

func TestDisplayNameFromDoc_NameFallback(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "My Name"},
		},
	})
	assert.Equal(t, "My Name", export.DisplayNameFromDoc(doc))
}

func TestDisplayNameFromDoc_IDFallback(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, nil)
	assert.Equal(t, docID1.String(), export.DisplayNameFromDoc(doc))
}

func TestNameCache_Preload(t *testing.T) {
	t.Parallel()

	nc := export.NewNameCache(func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	})
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.MnemonicPropID), String: "CACHED"},
		},
	})
	nc.Preload([]*document.D{doc})
	assert.Equal(t, "CACHED", nc.DisplayName(t.Context(), docID1))
}

func TestNameCache_PreloadSkipExisting(t *testing.T) {
	t.Parallel()

	nc := export.NewNameCache(func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	})
	nc.Names[docID1] = "EXISTING"
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.MnemonicPropID), String: "NEW"},
		},
	})
	nc.Preload([]*document.D{doc})
	assert.Equal(t, "EXISTING", nc.Names[docID1])
}

func TestNameCache_FetchFallback(t *testing.T) {
	t.Parallel()

	nc := export.NewNameCache(func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if id == docID1 {
			return makeDoc(docID1, &document.ClaimTypes{
				String: []document.StringClaim{
					{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Fetched Name"},
				},
			}), nil
		}
		return nil, nil //nolint:nilnil
	})
	assert.Equal(t, "Fetched Name", nc.DisplayName(t.Context(), docID1))
	assert.Equal(t, "Fetched Name", nc.DisplayName(t.Context(), docID1))
}

func TestNameCache_FetchError(t *testing.T) {
	t.Parallel()

	nc := export.NewNameCache(func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, errors.New("not found")
	})
	assert.Equal(t, docID1.String(), nc.DisplayName(t.Context(), docID1))
}

func makeDoc(id identifier.Identifier, claims *document.ClaimTypes) *document.D {
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

func TestProcessCSVDocument_StringClaims(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Alice"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Bob"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Equal(t, docID1.String(), row.ID)
	nameKey := export.ColumnKey([]string{"NAME"})
	assert.Equal(t, []string{"Alice", "Bob"}, row.Values[nameKey])
}

func TestProcessCSVDocument_LowConfidenceFiltered(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "kept"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.1, Sub: nil}, Prop: ref(internalCore.DescriptionPropID), String: "filtered"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Equal(t, []string{"kept"}, row.Values[export.ColumnKey([]string{"NAME"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"DESCRIPTION"})])
}

func TestProcessCSVDocument_NoneClaim(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		None: []document.NoneClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Equal(t, []string{export.NoneValue}, row.Values[export.ColumnKey([]string{"NAME"})])
}

func TestProcessCSVDocument_UnknownClaim(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		Unknown: []document.UnknownClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Equal(t, []string{export.UnknownValue}, row.Values[export.ColumnKey([]string{"NAME"})])
}

func TestProcessCSVDocument_SimpleHasClaim(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	hasKey := export.ColumnKey([]string{export.HasColumn})
	assert.Equal(t, []string{"NAME"}, row.Values[hasKey])
	assert.True(t, colSet[hasKey].IsHas)
}

func TestProcessCSVDocument_HasClaimWithSubClaims(t *testing.T) {
	t.Parallel()

	subClaims := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID2)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	key := export.ColumnKey([]string{"FIELDS", "CARDINALITY"})
	assert.Equal(t, []string{docID2.String()}, row.Values[key])
}

func TestProcessCSVDocument_HasClaimRecursiveSubClaims(t *testing.T) {
	t.Parallel()

	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID3)},
		},
	}
	fieldsClaims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldSub}, Prop: ref(internalCore.FieldPropID)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldsClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	key := export.ColumnKey([]string{"FIELDS", "FIELD", "CARDINALITY"})
	assert.Equal(t, []string{docID3.String()}, row.Values[key])
}

func TestProcessCSVDocument_SubClaimsOnRegularClaim(t *testing.T) {
	t.Parallel()

	// Also tests that ** (recursive wildcard) discovers sub-claims.
	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Equal(t, []string{"hello"}, row.Values[export.ColumnKey([]string{"NAME"})])
	assert.Equal(t, []string{langEN.String()}, row.Values[export.ColumnKey([]string{"NAME", "IN_LANGUAGE"})])
	// Verify the sub-column was registered.
	subKey := export.ColumnKey([]string{"NAME", "IN_LANGUAGE"})
	require.Contains(t, colSet, subKey)
	assert.False(t, colSet[subKey].IsHas)
}

func TestProcessCSVDocument_PropertySpecExcludesSubClaims(t *testing.T) {
	t.Parallel()

	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID)}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{"hello"}, row.Values[export.ColumnKey([]string{"NAME"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"NAME", "IN_LANGUAGE"})])
}

func TestProcessCSVDocument_PropertySpecIncludesSubWildcard(t *testing.T) {
	t.Parallel()

	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID), wildSeg()}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{langEN.String()}, row.Values[export.ColumnKey([]string{"NAME", "IN_LANGUAGE"})])
}

func TestProcessCSVDocument_PropertySpecFiltersProperties(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Alice"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.DescriptionPropID), String: "A description"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID)}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{"Alice"}, row.Values[export.ColumnKey([]string{"NAME"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"DESCRIPTION"})])
}

func TestProcessCSVDocument_EmptyDoc(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, nil)
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Empty(t, row.Values)
	assert.Empty(t, colSet)
}

func TestProcessCSVDocument_SpecFilterSubClaim(t *testing.T) {
	t.Parallel()

	subClaims := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID2)},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.FieldPropID), To: ref(docID3)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.FieldsPropID), seg(internalCore.CardinalityPropID)}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{docID2.String()}, row.Values[export.ColumnKey([]string{"FIELDS", "CARDINALITY"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD"})])
}

func TestProcessCSVDocument_LowConfidenceSubClaimFiltered(t *testing.T) {
	t.Parallel()

	subClaims := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID2)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	row := export.ProcessCSVDocument(t.Context(), doc, allSpecs(), names, colSet)

	assert.Empty(t, row.Values[export.ColumnKey([]string{"FIELDS", "CARDINALITY"})])
}

func TestProcessCSVDocument_ThreeLevelSpec(t *testing.T) {
	t.Parallel()

	// FIELDS (has) -> FIELD (has) -> CARDINALITY (ref to docID3).
	cardSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID3)},
		},
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "should be excluded"},
		},
	}
	fieldSub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: cardSub}, Prop: ref(internalCore.FieldPropID)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldSub}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	// Only request FIELDS.FIELD.CARDINALITY -- NAME sub-claim should be excluded.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.FieldsPropID),
		seg(internalCore.FieldPropID),
		seg(internalCore.CardinalityPropID),
	}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{docID3.String()}, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "CARDINALITY"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "NAME"})])
}

// makeFieldsDoc builds a document with FIELDS (has) -> FIELD (has) -> CARDINALITY (ref to docID3) + NAME (string "fieldname").
func makeFieldsDoc() *document.D {
	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID3)},
		},
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "fieldname"},
		},
	}
	fieldsClaims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldSub}, Prop: ref(internalCore.FieldPropID)},
		},
	}
	return makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldsClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
}

func TestProcessCSVDocument_WildcardMiddleSpec(t *testing.T) {
	t.Parallel()

	doc := makeFieldsDoc()
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	// FIELDS.*.CARDINALITY -- should match FIELDS.FIELD.CARDINALITY but not FIELDS.FIELD.NAME.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.FieldsPropID),
		wildSeg(),
		seg(internalCore.CardinalityPropID),
	}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{docID3.String()}, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "CARDINALITY"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "NAME"})])
}

func TestProcessCSVDocument_RecursiveWildcardMiddleSpec(t *testing.T) {
	t.Parallel()

	doc := makeFieldsDoc()
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	// FIELDS.**.CARDINALITY -- should find CARDINALITY at any depth under FIELDS.
	specs := []export.PropertySpec{{Segments: []export.PathSegment{
		seg(internalCore.FieldsPropID),
		recSeg(),
		seg(internalCore.CardinalityPropID),
	}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{docID3.String()}, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "CARDINALITY"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"FIELDS", "FIELD", "NAME"})])
}

func TestProcessCSVDocument_SingleWildcardNoSubs(t *testing.T) {
	t.Parallel()

	// * should match all properties but NOT include sub-claims.
	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	colSet := make(map[string]export.Column)
	specs := []export.PropertySpec{{Segments: []export.PathSegment{wildSeg()}}}
	row := export.ProcessCSVDocument(t.Context(), doc, specs, names, colSet)

	assert.Equal(t, []string{"hello"}, row.Values[export.ColumnKey([]string{"NAME"})])
	assert.Empty(t, row.Values[export.ColumnKey([]string{"NAME", "IN_LANGUAGE"})])
}

func TestProcessJSONDocument_StringClaims(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Alice"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "Bob"},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	assert.Equal(t, docID1.String(), entry.ID)
	require.Len(t, entry.Entries["NAME"], 2)
	assert.Equal(t, "Alice", entry.Entries["NAME"][0])
	assert.Equal(t, "Bob", entry.Entries["NAME"][1])
}

func TestProcessJSONDocument_HasClaimWithSubClaims(t *testing.T) {
	t.Parallel()

	subClaims := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID2)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries["FIELDS"], 1)
	nested, ok := entry.Entries["FIELDS"][0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, docID2.String(), nested["CARDINALITY"])
}

func TestProcessJSONDocument_HasClaimRecursiveSubClaims(t *testing.T) {
	t.Parallel()

	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.CardinalityPropID), To: ref(docID3)},
		},
	}
	fieldsClaims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldSub}, Prop: ref(internalCore.FieldPropID)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: fieldsClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries["FIELDS"], 1)
	fieldVal, ok := entry.Entries["FIELDS"][0].(map[string]any)
	require.True(t, ok)
	nested, ok := fieldVal["FIELD"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, docID3.String(), nested["CARDINALITY"])
}

func TestProcessJSONDocument_PropertySpecExcludesSubClaims(t *testing.T) {
	t.Parallel()

	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID)}}}
	entry := export.ProcessJSONDocument(t.Context(), doc, specs, names)

	require.Len(t, entry.Entries["NAME"], 1)
	// With no sub-specs, entry is a plain string, not a map.
	assert.Equal(t, "hello", entry.Entries["NAME"][0])
}

func TestProcessJSONDocument_PropertySpecIncludesSubWildcard(t *testing.T) {
	t.Parallel()

	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID), wildSeg()}}}
	entry := export.ProcessJSONDocument(t.Context(), doc, specs, names)

	require.Len(t, entry.Entries["NAME"], 1)
	entryObj, ok := entry.Entries["NAME"][0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, langEN.String(), entryObj["IN_LANGUAGE"])
}

func TestProcessJSONDocument_SimpleHasClaim(t *testing.T) {
	t.Parallel()

	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries[export.HasColumn], 1)
	assert.Equal(t, "NAME", entry.Entries[export.HasColumn][0])
}

func TestProcessJSONDocument_MultipleWithSameProp(t *testing.T) {
	t.Parallel()

	subClaims := &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "first"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "second"},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries["FIELDS"], 1)
	nested, ok := entry.Entries["FIELDS"][0].(map[string]any)
	require.True(t, ok)
	nameVals, ok := nested["NAME"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"first", "second"}, nameVals)
}

func TestProcessJSONDocument_RecursiveWildcardAll(t *testing.T) {
	t.Parallel()

	// ** discovers everything including sub-claims at all depths.
	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries["NAME"], 1)
	nameObj, ok := entry.Entries["NAME"][0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", nameObj["value"])
	assert.Equal(t, langEN.String(), nameObj["IN_LANGUAGE"])
}

func TestProcessJSONDocument_LowConfidenceSubClaimFiltered(t *testing.T) {
	t.Parallel()

	// Sub-claims with low confidence should be excluded from JSON output.
	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	// NAME should be a simple string, not an object (sub-claim was filtered).
	require.Len(t, entry.Entries["NAME"], 1)
	assert.Equal(t, "hello", entry.Entries["NAME"][0])
}

func TestProcessJSONDocument_EmptySubClaimsNoEntry(t *testing.T) {
	t.Parallel()

	// HasClaim with sub-claims that contain only another HasClaim with no further content
	// should not produce spurious entries.
	innerSub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.FieldPropID)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: innerSub}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	// FIELDS should have no entries because the nested HasClaim has no value.
	assert.Empty(t, entry.Entries["FIELDS"])
}

func TestProcessJSONDocument_AppendSubValueMerging(t *testing.T) {
	t.Parallel()

	// Three sub-claims with the same property should be collected into an array.
	subClaims := &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "first"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "second"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "third"},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: subClaims}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	names := makeNameCache()
	entry := export.ProcessJSONDocument(t.Context(), doc, allSpecs(), names)

	require.Len(t, entry.Entries["FIELDS"], 1)
	nested, ok := entry.Entries["FIELDS"][0].(map[string]any)
	require.True(t, ok)
	nameVals, ok := nested["NAME"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"first", "second", "third"}, nameVals)
}

func TestSortColumns(t *testing.T) {
	t.Parallel()

	colSet := map[string]export.Column{
		export.ColumnKey([]string{"DESCRIPTION"}):    {Path: []string{"DESCRIPTION"}, IsHas: false},
		export.ColumnKey([]string{"NAME"}):           {Path: []string{"NAME"}, IsHas: false},
		export.ColumnKey([]string{export.HasColumn}): {Path: []string{export.HasColumn}, IsHas: true},
		export.ColumnKey([]string{"CODE"}):           {Path: []string{"CODE"}, IsHas: false},
	}
	sorted := export.SortColumns(colSet)
	require.Len(t, sorted, 4)
	assert.Equal(t, "CODE", sorted[0].CSVName())
	assert.Equal(t, "DESCRIPTION", sorted[1].CSVName())
	assert.Equal(t, "NAME", sorted[2].CSVName())
	assert.Equal(t, export.HasColumn, sorted[3].CSVName())
	assert.True(t, sorted[3].IsHas)
}

func TestWriteCSVRow_Simple(t *testing.T) {
	t.Parallel()

	columns := []export.Column{
		{Path: []string{"NAME"}, IsHas: false},
		{Path: []string{"CODE"}, IsHas: false},
	}
	row := export.CSVRow{
		ID: "doc1",
		Values: map[string][]string{
			export.ColumnKey([]string{"NAME"}): {"Alice"},
			export.ColumnKey([]string{"CODE"}): {"A"},
		},
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	errE := export.WriteCSVRow(cw, columns, row)
	require.NoError(t, errE, "% -+#.1v", errE)
	cw.Flush()
	assert.Equal(t, "doc1,Alice,A\n", buf.String())
}

func TestWriteCSVRow_RepeatedValues(t *testing.T) {
	t.Parallel()

	columns := []export.Column{
		{Path: []string{"NAME"}, IsHas: false},
		{Path: []string{"CODE"}, IsHas: false},
	}
	row := export.CSVRow{
		ID: "doc1",
		Values: map[string][]string{
			export.ColumnKey([]string{"NAME"}): {"Alice", "Bob"},
			export.ColumnKey([]string{"CODE"}): {"A"},
		},
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	errE := export.WriteCSVRow(cw, columns, row)
	require.NoError(t, errE, "% -+#.1v", errE)
	cw.Flush()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "doc1,Alice,A", lines[0])
	assert.Equal(t, ",Bob,", lines[1])
}

func TestWriteCSVRow_EmptyDoc(t *testing.T) {
	t.Parallel()

	columns := []export.Column{{Path: []string{"NAME"}, IsHas: false}}
	row := export.CSVRow{ID: "doc1", Values: map[string][]string{}}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	errE := export.WriteCSVRow(cw, columns, row)
	require.NoError(t, errE, "% -+#.1v", errE)
	cw.Flush()
	assert.Equal(t, "doc1,\n", buf.String())
}

func TestWriteJSONEntry_SimpleArrays(t *testing.T) {
	t.Parallel()

	entry := export.JSONEntry{
		ID:      "doc1",
		Entries: map[string][]any{"NAME": {"Alice", "Bob"}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	errE := export.WriteJSONEntry(enc, entry)
	require.NoError(t, errE, "% -+#.1v", errE)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	assert.Equal(t, "doc1", obj["id"])
	names, ok := obj["NAME"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"Alice", "Bob"}, names)
}

func TestWriteJSONEntry_NestedObjects(t *testing.T) {
	t.Parallel()

	entry := export.JSONEntry{
		ID: "doc1",
		Entries: map[string][]any{
			"NAME": {
				map[string]any{"value": "hello", "IN_LANGUAGE": "en"},
				map[string]any{"value": "hola", "IN_LANGUAGE": "es"},
			},
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	errE := export.WriteJSONEntry(enc, entry)
	require.NoError(t, errE, "% -+#.1v", errE)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	names, ok := obj["NAME"].([]any)
	require.True(t, ok)
	require.Len(t, names, 2)
	first, ok := names[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", first["value"])
	assert.Equal(t, "en", first["IN_LANGUAGE"])
}

func TestWriteJSONEntry_HasColumnValues(t *testing.T) {
	t.Parallel()

	entry := export.JSONEntry{
		ID:      "doc1",
		Entries: map[string][]any{export.HasColumn: {"ABSTRACT_CLASS", "SETTING"}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	errE := export.WriteJSONEntry(enc, entry)
	require.NoError(t, errE, "% -+#.1v", errE)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	hasArr, ok := obj[export.HasColumn].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"ABSTRACT_CLASS", "SETTING"}, hasArr)
}

func TestWriteJSONEntry_EmptyEntriesSkipped(t *testing.T) {
	t.Parallel()

	entry := export.JSONEntry{
		ID:      "doc1",
		Entries: map[string][]any{"NAME": {}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	errE := export.WriteJSONEntry(enc, entry)
	require.NoError(t, errE, "% -+#.1v", errE)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	_, ok := obj["NAME"]
	assert.False(t, ok)
}

func TestWriteJSONEntry_HasClaimWithSubValues(t *testing.T) {
	t.Parallel()

	entry := export.JSONEntry{
		ID: "doc1",
		Entries: map[string][]any{
			"FIELDS": {map[string]any{"CARDINALITY": "one", "FIELD": "ref1"}},
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	errE := export.WriteJSONEntry(enc, entry)
	require.NoError(t, errE, "% -+#.1v", errE)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	fields, ok := obj["FIELDS"].([]any)
	require.True(t, ok)
	require.Len(t, fields, 1)
	entryObj, ok := fields[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "one", entryObj["CARDINALITY"])
	assert.Equal(t, "ref1", entryObj["FIELD"])
	_, hasValue := entryObj["value"]
	assert.False(t, hasValue)
}

func makeTestDocs() map[identifier.Identifier]*document.D {
	nameSub1 := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	nameSub2 := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langSL)},
		},
	}
	doc1 := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub1}, Prop: ref(internalCore.NamePropID), String: "byte"},
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub2}, Prop: ref(internalCore.NamePropID), String: "bajt"},
		},
		Identifier: []document.IdentifierClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.MnemonicPropID), Value: "BYTE"},
		},
	})
	doc2 := makeDoc(docID2, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.NamePropID), String: "metre"},
		},
		None: []document.NoneClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.DescriptionPropID)},
		},
		Has: []document.HasClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.FieldsPropID)},
		},
	})
	return map[identifier.Identifier]*document.D{docID1: doc1, docID2: doc2}
}

func testGetDoc(docs map[identifier.Identifier]*document.D) export.GetDocFunc {
	return func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if doc, ok := docs[id]; ok {
			return doc, nil
		}
		return nil, nil
	}
}

func TestExportCSV_Integration(t *testing.T) {
	t.Parallel()

	docs := makeTestDocs()
	docIDs := []identifier.Identifier{docID1, docID2}
	names := makeNameCache()

	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, docIDs, allSpecs(), names, testGetDoc(docs))
	require.NoError(t, errE, "% -+#.1v", errE)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	header := records[0]
	assert.Equal(t, "id", header[0])
	assert.Contains(t, header, "NAME")
	assert.Contains(t, header, "DESCRIPTION")
	assert.Equal(t, docID1.String(), records[1][0])
	assert.Empty(t, records[2][0])
}

func TestExportCSV_WithPropertyFilter(t *testing.T) {
	t.Parallel()

	docs := makeTestDocs()
	names := makeNameCache()
	specs := []export.PropertySpec{{Segments: []export.PathSegment{seg(internalCore.NamePropID)}}}

	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, []identifier.Identifier{docID1}, specs, names, testGetDoc(docs))
	require.NoError(t, errE, "% -+#.1v", errE)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "NAME"}, records[0])
}

func TestExportJSON_Integration(t *testing.T) {
	t.Parallel()

	docs := makeTestDocs()
	docIDs := []identifier.Identifier{docID1, docID2}
	names := makeNameCache()

	var buf bytes.Buffer
	errE := export.JSON(t.Context(), &buf, docIDs, allSpecs(), names, testGetDoc(docs))
	require.NoError(t, errE, "% -+#.1v", errE)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)

	var obj1 map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &obj1))
	assert.Equal(t, docID1.String(), obj1["id"])

	nameArr, ok := obj1["NAME"].([]any)
	require.True(t, ok)
	require.Len(t, nameArr, 2)
	first, ok := nameArr[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "byte", first["value"])
	assert.Equal(t, langEN.String(), first["IN_LANGUAGE"])

	for k := range obj1 {
		assert.NotContains(t, k, "\x00")
	}

	var obj2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &obj2))
	descArr, ok := obj2["DESCRIPTION"].([]any)
	require.True(t, ok)
	assert.Equal(t, export.NoneValue, descArr[0])
	hasArr, ok := obj2[export.HasColumn].([]any)
	require.True(t, ok)
	assert.Contains(t, hasArr, "FIELDS")

	for k, v := range obj2 {
		if arr, ok := v.([]any); ok {
			assert.NotEmpty(t, arr, "empty array for key %s", k)
		}
	}
}

func TestExportJSON_NilDoc(t *testing.T) {
	t.Parallel()

	getDoc := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	}
	var buf bytes.Buffer
	errE := export.JSON(t.Context(), &buf, []identifier.Identifier{docID1}, allSpecs(), makeNameCache(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, buf.String())
}

func TestExportCSV_NilDoc(t *testing.T) {
	t.Parallel()

	getDoc := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	}
	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, []identifier.Identifier{docID1}, allSpecs(), makeNameCache(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "id\n", buf.String())
}

func TestExportCSV_MultipleDocuments(t *testing.T) {
	t.Parallel()

	docs := makeTestDocs()
	docIDs := []identifier.Identifier{docID1, docID2}
	names := makeNameCache()

	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, docIDs, allSpecs(), names, testGetDoc(docs))
	require.NoError(t, errE, "% -+#.1v", errE)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	// Should have header + doc1 (2 rows: repeated NAME) + doc2 (1 row).
	require.GreaterOrEqual(t, len(records), 4)

	// Find doc2 row.
	found := false
	for _, rec := range records[1:] {
		if rec[0] == docID2.String() {
			found = true
			break
		}
	}
	assert.True(t, found, "doc2 should appear in CSV output")
}

func TestExportCSV_GetDocError(t *testing.T) {
	t.Parallel()

	getDoc := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, errors.New("db error")
	}
	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, []identifier.Identifier{docID1}, allSpecs(), makeNameCache(), getDoc)
	require.Error(t, errE)
}

func TestExportJSON_GetDocError(t *testing.T) {
	t.Parallel()

	getDoc := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, errors.New("db error")
	}
	var buf bytes.Buffer
	errE := export.JSON(t.Context(), &buf, []identifier.Identifier{docID1}, allSpecs(), makeNameCache(), getDoc)
	require.Error(t, errE)
}

func TestExportCSV_DotSeparatedSubColumns(t *testing.T) {
	t.Parallel()

	nameSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nil}, Prop: ref(internalCore.InLanguagePropID), To: ref(langEN)},
		},
	}
	doc := makeDoc(docID1, &document.ClaimTypes{
		String: []document.StringClaim{
			{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1, Sub: nameSub}, Prop: ref(internalCore.NamePropID), String: "hello"},
		},
	})
	docs := map[identifier.Identifier]*document.D{docID1: doc}
	names := makeNameCache()

	var buf bytes.Buffer
	errE := export.CSV(t.Context(), &buf, []identifier.Identifier{docID1}, allSpecs(), names, testGetDoc(docs))
	require.NoError(t, errE, "% -+#.1v", errE)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)
	assert.Contains(t, records[0], "NAME.IN_LANGUAGE")
}
