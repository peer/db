package transform_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/transform"
)

type SimpleDoc struct {
	ID   []string `documentid:""`
	Name string   `              property:"NAME"`
}

type DocWithIdentifier struct {
	ID    []string `documentid:""`
	Code  string   `              property:"CODE"  type:"id"`
	Codes []string `              property:"CODES" type:"id"`
}

type DocWithHTML struct {
	ID          []string  `documentid:""`
	Description string    `              property:"DESCRIPTION" type:"html"`
	Notes       []string  `              property:"NOTES"       type:"html"`
	HTMLText    core.HTML `              property:"HTML"`
}

type DocWithRawHTMLComplex struct {
	ID             []string     `documentid:""`
	RawDescription string       `              property:"RAW_DESCRIPTION" type:"rawhtml"`
	RawNotes       []string     `              property:"RAW_NOTES"       type:"rawhtml"`
	RawHTMLText    core.RawHTML `              property:"RAW_HTML"`
}

type DocWithIRI struct {
	ID       []string `documentid:""`
	Homepage string   `              property:"HOMEPAGE"  type:"iri"`
	Links    []string `              property:"LINKS"     type:"iri"`
	PlainIRI core.IRI `              property:"PLAIN_IRI"`
}

type DocWithRef struct {
	ID       []string   `documentid:""`
	Parent   core.Ref   `              property:"PARENT"`
	Children []core.Ref `              property:"CHILDREN"`
}

type DocWithTime struct {
	ID        []string    `documentid:""`
	Created   core.Time   `              property:"CREATED"`
	Modified  []core.Time `              property:"MODIFIED"`
	Published core.Time   `              property:"PUBLISHED"`
}

type DocWithInterval struct {
	ID     []string                 `documentid:""`
	Period core.Interval[core.Time] `              property:"PERIOD"`
}

type DocWithAmount struct {
	ID     []string `documentid:""`
	Width  float64  `              precision:"0.01" property:"WIDTH"`
	Height int      `              precision:"1"    property:"HEIGHT"`
	Count  uint     `              precision:"1"    property:"COUNT"`
}

type DocWithBool struct {
	ID        []string `documentid:""`
	Published bool     `              property:"PUBLISHED"`
	Hidden    bool     `              property:"HIDDEN"`
}

type DocWithRequired struct {
	ID    []string `                               documentid:""`
	Title string   `cardinality:"1" default:"none"               property:"TITLE"`
}

type DocWithUnknown struct {
	ID              []string `documentid:""`
	Name            string   `              property:"NAME"`
	AgeIsUnknown    bool     `              property:"AGE"    type:"unknown"`
	HeightIsUnknown bool     `              property:"HEIGHT" type:"unknown"`
}

type NestedValue struct {
	Value  string                   `                  value:""`
	Period core.Interval[core.Time] `property:"PERIOD"`
	Note   string                   `property:"NOTE"`
}

type DocWithNestedValue struct {
	ID          []string      `documentid:""`
	Title       NestedValue   `              property:"TITLE"`
	Description []NestedValue `              property:"DESCRIPTION"`
}

type NestedWithoutValue struct {
	Location core.Ref                 `property:"LOCATION"`
	Period   core.Interval[core.Time] `property:"PERIOD"`
}

type DocWithNestedNoValue struct {
	ID      []string             `documentid:""`
	Address NestedWithoutValue   `              property:"ADDRESS"`
	History []NestedWithoutValue `              property:"HISTORY"`
}

type DocWithSkippedFields struct {
	ID           []string `documentid:""`
	Name         string   `              property:"NAME"`
	Internal     string   // No property tag - should be skipped.
	SkipExplicit string   `property:"-"` // Explicit skip.
}

type DocWithPointer struct {
	ID       []string  `documentid:""`
	Optional *core.Ref `              property:"OPTIONAL"`
}

type BaseDocFields struct {
	ID []string `documentid:""`
}

type DocWithEmbedded struct {
	BaseDocFields

	Name string `property:"NAME"`
}

type MiddleFields struct {
	BaseDocFields

	Description string `property:"DESCRIPTION"`
}

type DocWithNestedEmbedded struct {
	MiddleFields

	Title string `property:"TITLE"`
}

type EmbeddedProperties struct {
	Name   string     `property:"NAME"`
	Author []core.Ref `property:"HAS_AUTHOR"`
}

type DocWithEmbeddedProperties struct {
	EmbeddedProperties

	ID    []string `documentid:""`
	Extra string   `              property:"EXTRA"`
}

func createMnemonics() map[string]identifier.Identifier {
	return map[string]identifier.Identifier{
		"NAME":            identifier.From("test", "NAME"),
		"CODE":            identifier.From("test", "CODE"),
		"CODES":           identifier.From("test", "CODES"),
		"DESCRIPTION":     identifier.From("test", "DESCRIPTION"),
		"NOTES":           identifier.From("test", "NOTES"),
		"HTML":            identifier.From("test", "HTML"),
		"RAW_DESCRIPTION": identifier.From("test", "RAW_DESCRIPTION"),
		"RAW_NOTES":       identifier.From("test", "RAW_NOTES"),
		"RAW_HTML":        identifier.From("test", "RAW_HTML"),
		"HOMEPAGE":        identifier.From("test", "HOMEPAGE"),
		"LINKS":           identifier.From("test", "LINKS"),
		"PLAIN_IRI":       identifier.From("test", "PLAIN_IRI"),
		"PARENT":          identifier.From("test", "PARENT"),
		"CHILDREN":        identifier.From("test", "CHILDREN"),
		"CREATED":         identifier.From("test", "CREATED"),
		"MODIFIED":        identifier.From("test", "MODIFIED"),
		"PUBLISHED":       identifier.From("test", "PUBLISHED"),
		"PERIOD":          identifier.From("test", "PERIOD"),
		"WIDTH":           identifier.From("test", "WIDTH"),
		"HEIGHT":          identifier.From("test", "HEIGHT"),
		"COUNT":           identifier.From("test", "COUNT"),
		"HIDDEN":          identifier.From("test", "HIDDEN"),
		"TITLE":           identifier.From("test", "TITLE"),
		"AGE":             identifier.From("test", "AGE"),
		"LOCATION":        identifier.From("test", "LOCATION"),
		"ADDRESS":         identifier.From("test", "ADDRESS"),
		"HISTORY":         identifier.From("test", "HISTORY"),
		"NOTE":            identifier.From("test", "NOTE"),
		"OPTIONAL":        identifier.From("test", "OPTIONAL"),
		"BORN":            identifier.From("test", "BORN"),
	}
}

func TestDocuments_SimpleString(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&SimpleDoc{
			ID:   []string{"test", "doc1"},
			Name: "Test Document",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)

	doc := results[0]
	assert.Equal(t, identifier.From("test", "doc1"), doc.ID)

	// Check StringClaim.
	require.Len(t, doc.Claims.String, 1)

	claim := doc.Claims.String[0]
	assert.Equal(t, "Test Document", claim.String)

	propID := mnemonics["NAME"]
	assert.Equal(t, propID, claim.Prop.ID)

	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), claim.ID)
}

func TestDocuments_IdentifierClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithIdentifier{
			ID:    []string{"test", "doc1"},
			Code:  "ABC123",
			Codes: []string{"XYZ", "DEF"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check IdentifierClaim for Code.
	require.Len(t, doc.Claims.Identifier, 3)

	// First claim is Code.
	assert.Equal(t, "ABC123", doc.Claims.Identifier[0].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODE", "0"), doc.Claims.Identifier[0].ID)

	// Next two are Codes.
	assert.Equal(t, "XYZ", doc.Claims.Identifier[1].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODES", "0"), doc.Claims.Identifier[1].ID)
	assert.Equal(t, "DEF", doc.Claims.Identifier[2].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODES", "1"), doc.Claims.Identifier[2].ID)
}

func TestDocuments_TextClaim(t *testing.T) { //nolint:dupl
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithHTML{
			ID:          []string{"test", "doc1"},
			Description: "<p>Test</p>",
			Notes:       []string{"<b>Note 1</b>", "<i>Note 2</i>"},
			HTMLText:    "HTML",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check TextClaims (Description + 2 Notes + HTMLText = 4).
	require.Len(t, doc.Claims.HTML, 4)

	// Check HTML escaping.
	assert.Equal(t, "&lt;p&gt;Test&lt;/p&gt;", doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "DESCRIPTION", "0"), doc.Claims.HTML[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "NOTES", "0"), doc.Claims.HTML[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "NOTES", "1"), doc.Claims.HTML[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.HTML[3].ID)
}

func TestDocuments_RawHTMLTextClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRawHTMLComplex{
			ID:             []string{"test", "doc1"},
			RawDescription: "<p>Test</p>",
			RawNotes:       []string{"<b>Note 1</b>", "<i>Note 2</i>"},
			RawHTMLText:    "<script>alert('xss')</script>",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check RawTextClaims: RawDescription + 2 RawNotes = 3 (RawHTMLText is skipped because sanitization strips all content).
	require.Len(t, doc.Claims.HTML, 3)

	// Check HTML is NOT escaped for rawhtml type.
	assert.Equal(t, "<p>Test</p>", doc.Claims.HTML[0].HTML)
	assert.Equal(t, "<b>Note 1</b>", doc.Claims.HTML[1].HTML)
	assert.Equal(t, "<i>Note 2</i>", doc.Claims.HTML[2].HTML)

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "RAW_DESCRIPTION", "0"), doc.Claims.HTML[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "0"), doc.Claims.HTML[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "1"), doc.Claims.HTML[2].ID)
}

func TestDocuments_RawHTMLTextClaimWithSurroundingText(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRawHTMLComplex{
			ID:             []string{"test", "doc1"},
			RawDescription: "<p>Test</p>",
			RawNotes:       []string{"<b>Note 1</b>", "<i>Note 2</i>"},
			RawHTMLText:    "hello <script>alert('xss')</script> world",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check RawTextClaims (RawDescription + 2 RawNotes + RawHTMLText = 4).
	// RawHTMLText has surrounding text so sanitization leaves a non-empty result.
	require.Len(t, doc.Claims.HTML, 4)

	// Check HTML is NOT escaped for rawhtml type.
	assert.Equal(t, "<p>Test</p>", doc.Claims.HTML[0].HTML)
	assert.Equal(t, "<b>Note 1</b>", doc.Claims.HTML[1].HTML)
	assert.Equal(t, "<i>Note 2</i>", doc.Claims.HTML[2].HTML)
	// Script tag and its content are stripped, but surrounding text remains.
	assert.Equal(t, "hello  world", doc.Claims.HTML[3].HTML)

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "RAW_DESCRIPTION", "0"), doc.Claims.HTML[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "0"), doc.Claims.HTML[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "1"), doc.Claims.HTML[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_HTML", "0"), doc.Claims.HTML[3].ID)
}

func TestDocuments_LinkClaim(t *testing.T) { //nolint:dupl
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithIRI{
			ID:       []string{"test", "doc1"},
			Homepage: "https://example.com",
			Links:    []string{"https://link1.com", "https://link2.com"},
			PlainIRI: "https://plain.com",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Homepage + 2 Links + PlainIRI.
	require.Len(t, doc.Claims.Link, 4)

	assert.Equal(t, "https://example.com", doc.Claims.Link[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "HOMEPAGE", "0"), doc.Claims.Link[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "LINKS", "0"), doc.Claims.Link[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "LINKS", "1"), doc.Claims.Link[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "PLAIN_IRI", "0"), doc.Claims.Link[3].ID)
}

func TestDocuments_ReferenceClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRef{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"parent", "id"}},
			Children: []core.Ref{
				{ID: []string{"child", "1"}},
				{ID: []string{"child", "2"}},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Parent + 2 Children = 3.
	require.Len(t, doc.Claims.Reference, 3)

	expectedParentID := identifier.From("parent", "id")
	assert.Equal(t, expectedParentID, doc.Claims.Reference[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "PARENT", "0"), doc.Claims.Reference[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CHILDREN", "0"), doc.Claims.Reference[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CHILDREN", "1"), doc.Claims.Reference[2].ID)
}

func TestDocuments_TimeClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	now := time.Now()
	later := now.Add(time.Hour)
	docs := []any{
		&DocWithTime{
			ID: []string{"test", "doc1"},
			Created: core.Time{
				Time:      now,
				Precision: document.TimePrecisionSecond,
			},
			Modified: []core.Time{
				{Time: later, Precision: document.TimePrecisionSecond},
			},
			Published: core.Time{
				Time:      now.Add(2 * time.Hour),
				Precision: document.TimePrecisionSecond,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Time, 3)

	tt, errE := doc.Claims.Time[0].Time.Time(doc.Claims.Time[0].Precision, time.UTC)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, now.UTC().Truncate(time.Second), tt)

	assert.Equal(t, document.TimePrecisionSecond, doc.Claims.Time[0].Precision)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0"), doc.Claims.Time[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "MODIFIED", "0"), doc.Claims.Time[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "PUBLISHED", "0"), doc.Claims.Time[2].ID)
}

func TestDocuments_TimeRangeClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{
				From:          &core.Time{Time: start, Precision: document.TimePrecisionDay},
				FromIsOpen:    false,
				FromIsUnknown: false,
				FromIsNone:    false,
				To:            &core.Time{Time: end, Precision: document.TimePrecisionDay},
				ToIsClosed:    false,
				ToIsUnknown:   false,
				ToIsNone:      false,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.TimeInterval, 1)

	claim := doc.Claims.TimeInterval[0]
	require.NotNil(t, claim.From)
	require.NotNil(t, claim.FromPrecision)
	fromTime, errE := claim.From.Time(*claim.FromPrecision, time.UTC)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, start, fromTime)
	require.NotNil(t, claim.To)
	require.NotNil(t, claim.ToPrecision)
	toTime, errE := claim.To.Time(*claim.ToPrecision, time.UTC)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, end, toTime)
	assert.Equal(t, identifier.From("test", "doc1", "PERIOD", "0"), claim.ID)
}

func TestDocuments_IntervalUnknown(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{
				From:          nil,
				FromIsOpen:    false,
				FromIsUnknown: true,
				FromIsNone:    false,
				To:            nil,
				ToIsClosed:    false,
				ToIsUnknown:   true,
				ToIsNone:      false,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Both-unknown interval creates a TimeIntervalClaim with unknown bounds.
	require.Len(t, doc.Claims.TimeInterval, 1)
	claim := doc.Claims.TimeInterval[0]
	assert.True(t, claim.FromIsUnknown)
	assert.Nil(t, claim.From)
	assert.True(t, claim.ToIsUnknown)
	assert.Nil(t, claim.To)
	assert.Equal(t, identifier.From("test", "doc1", "PERIOD", "0"), claim.ID)
}

func TestDocuments_TimeIntervalWithNoneBounds(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From:     &core.Time{Time: start, Precision: document.TimePrecisionDay},
				ToIsNone: true,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.TimeInterval, 1)
	claim := doc.Claims.TimeInterval[0]
	require.NotNil(t, claim.From)
	assert.Nil(t, claim.To)
	assert.True(t, claim.ToIsNone)
	assert.False(t, claim.FromIsUnknown)
}

func TestDocuments_TimeIntervalWithOpenBound(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From:       &core.Time{Time: start, Precision: document.TimePrecisionDay},
				FromIsOpen: true,
				To:         &core.Time{Time: end, Precision: document.TimePrecisionDay},
				ToIsClosed: true,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.TimeInterval, 1)
	claim := doc.Claims.TimeInterval[0]
	assert.True(t, claim.FromIsOpen)
	assert.True(t, claim.ToIsClosed)
	require.NotNil(t, claim.From)
	require.NotNil(t, claim.To)
}

func TestDocuments_TimeIntervalMissingBound(t *testing.T) {
	t.Parallel()

	// From is nil with no flags set - not a valid interval.
	mnemonics := createMnemonics()
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				To: &core.Time{Time: end, Precision: document.TimePrecisionDay},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "from" bound is not set`)
}

func TestDocuments_AmountIntervalWithUnknownBounds(t *testing.T) {
	t.Parallel()

	type DocWithAmountInterval struct {
		ID          []string                        `documentid:""`
		Cardinality core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountInterval{
			ID: []string{"test", "doc1"},
			Cardinality: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				FromIsUnknown: true,
				To:            &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.AmountInterval, 1)
	claim := doc.Claims.AmountInterval[0]
	assert.True(t, claim.FromIsUnknown)
	assert.Nil(t, claim.From)
	require.NotNil(t, claim.To)
	assert.Equal(t, document.Amount("10"), *claim.To)
}

func TestDocuments_AmountIntervalWithNoneBounds(t *testing.T) {
	t.Parallel()

	type DocWithAmountInterval struct {
		ID          []string                        `documentid:""`
		Cardinality core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountInterval{
			ID: []string{"test", "doc1"},
			Cardinality: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From:     &core.Amount[int]{Amount: 1, Precision: 1},
				ToIsNone: true,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.AmountInterval, 1)
	claim := doc.Claims.AmountInterval[0]
	require.NotNil(t, claim.From)
	assert.Equal(t, document.Amount("1"), *claim.From)
	assert.Nil(t, claim.To)
	assert.True(t, claim.ToIsNone)
}

func TestDocuments_AmountIntervalWithOpenBound(t *testing.T) {
	t.Parallel()

	type DocWithAmountInterval struct {
		ID          []string                        `documentid:""`
		Cardinality core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountInterval{
			ID: []string{"test", "doc1"},
			Cardinality: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From:       &core.Amount[int]{Amount: 1, Precision: 1},
				FromIsOpen: true,
				To:         &core.Amount[int]{Amount: 10, Precision: 1},
				ToIsClosed: true,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.AmountInterval, 1)
	claim := doc.Claims.AmountInterval[0]
	assert.True(t, claim.FromIsOpen)
	assert.True(t, claim.ToIsClosed)
	require.NotNil(t, claim.From)
	require.NotNil(t, claim.To)
}

func TestDocuments_AmountIntervalMissingBound(t *testing.T) {
	t.Parallel()

	type DocWithAmountInterval struct {
		ID          []string                        `documentid:""`
		Cardinality core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountInterval{
			ID: []string{"test", "doc1"},
			Cardinality: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From: &core.Amount[int]{Amount: 1, Precision: 1},
				// To is nil with no flags.
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "to" bound is not set`)
}

func TestDocuments_AmountClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmount{
			ID:     []string{"test", "doc1"},
			Width:  1.5,
			Height: 200,
			Count:  42,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Amount, 3)

	// Check Width (float).
	assert.Equal(t, document.Amount("1.50"), doc.Claims.Amount[0].Amount)
	assert.Equal(t, identifier.From("test", "doc1", "WIDTH", "0"), doc.Claims.Amount[0].ID)

	// Check Height (int).
	assert.Equal(t, document.Amount("200"), doc.Claims.Amount[1].Amount)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[1].ID)

	// Check Count (uint).
	assert.Equal(t, document.Amount("42"), doc.Claims.Amount[2].Amount)
	assert.Equal(t, identifier.From("test", "doc1", "COUNT", "0"), doc.Claims.Amount[2].ID)
}

func TestDocuments_CoreAmountClaim(t *testing.T) {
	t.Parallel()

	type DocWithCoreAmount struct {
		ID     []string             `documentid:""`
		Width  core.Amount[float64] `              property:"WIDTH"`
		Height core.Amount[int]     `              property:"HEIGHT"`
		Count  core.Amount[uint]    `              property:"COUNT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithCoreAmount{
			ID:     []string{"test", "doc1"},
			Width:  core.Amount[float64]{Amount: 1.5, Precision: 0.1},
			Height: core.Amount[int]{Amount: 200, Precision: 1},
			Count:  core.Amount[uint]{Amount: 42, Precision: 1},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Amount, 3)

	// Check Width (float64).
	assert.Equal(t, document.Amount("1.5"), doc.Claims.Amount[0].Amount)
	assert.Equal(t, 0.1, doc.Claims.Amount[0].Precision) //nolint:testifylint
	assert.Equal(t, identifier.From("test", "doc1", "WIDTH", "0"), doc.Claims.Amount[0].ID)

	// Check Height (int).
	assert.Equal(t, document.Amount("200"), doc.Claims.Amount[1].Amount)
	assert.Equal(t, 1.0, doc.Claims.Amount[1].Precision) //nolint:testifylint
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[1].ID)

	// Check Count (uint).
	assert.Equal(t, document.Amount("42"), doc.Claims.Amount[2].Amount)
	assert.Equal(t, 1.0, doc.Claims.Amount[2].Precision) //nolint:testifylint
	assert.Equal(t, identifier.From("test", "doc1", "COUNT", "0"), doc.Claims.Amount[2].ID)
}

func TestDocuments_CoreAmountPrecisionInfinity(t *testing.T) {
	t.Parallel()

	type DocWithInfPrecision struct {
		ID    []string             `documentid:""`
		Width core.Amount[float64] `              property:"WIDTH"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithInfPrecision{
			ID:    []string{"test", "doc1"},
			Width: core.Amount[float64]{Amount: 1.5, Precision: math.Inf(1)},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision must be finite positive number")
}

func TestDocuments_AmountRangeClaim(t *testing.T) {
	t.Parallel()

	type DocWithAmountInterval struct {
		ID          []string                        `documentid:""`
		Cardinality core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountInterval{
			ID: []string{"test", "doc1"},
			Cardinality: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From: &core.Amount[int]{Amount: 1, Precision: 1},
				To:   &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.AmountInterval, 1)

	claim := doc.Claims.AmountInterval[0]
	require.NotNil(t, claim.From)
	assert.Equal(t, document.Amount("1"), *claim.From)
	require.NotNil(t, claim.To)
	assert.Equal(t, document.Amount("10"), *claim.To)
	assert.Equal(t, identifier.From("test", "doc1", "PERIOD", "0"), claim.ID)
}

func TestDocuments_BoolNoValue(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithBool{
			ID:        []string{"test", "doc1"},
			Published: true,
			Hidden:    false,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only Published=true should create HasClaim.
	require.Len(t, doc.Claims.Has, 1)

	propID := mnemonics["PUBLISHED"]
	assert.Equal(t, propID, doc.Claims.Has[0].Prop.ID)
	assert.Equal(t, identifier.From("test", "doc1", "PUBLISHED", "0"), doc.Claims.Has[0].ID)
}

func TestDocuments_RequiredEmpty(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRequired{
			ID:    []string{"test", "doc1"},
			Title: "", // Empty but required.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create NoneClaim for empty string with required cardinality and default:"none" tag.
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_RequiredEmptySlice(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type DocWithRequiredSlice struct {
		ID     []string `                                 documentid:""`
		Titles []string `cardinality:"1.." default:"none"               property:"TITLE"`
	}

	docs := []any{
		&DocWithRequiredSlice{
			ID:     []string{"test", "doc1"},
			Titles: []string{}, // Empty slice with required.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create NoneClaim for empty slice with required tag.
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_UnknownValue(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithUnknown{
			ID:              []string{"test", "doc1"},
			Name:            "John",
			AgeIsUnknown:    true,
			HeightIsUnknown: false,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only AgeIsUnknown=true should create UnknownClaim.
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.Unknown[0].ID)

	// Should have string claim for Name.
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
}

func TestDocuments_NestedWithValue(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithNestedValue{
			ID: []string{"test", "doc1"},
			Title: NestedValue{
				Value: "Main Title",
				Period: core.Interval[core.Time]{
					From:          &core.Time{Time: start, Precision: document.TimePrecisionYear},
					FromIsOpen:    false,
					FromIsUnknown: false,
					FromIsNone:    false,
					To:            &core.Time{Time: end, Precision: document.TimePrecisionYear},
					ToIsClosed:    false,
					ToIsUnknown:   false,
					ToIsNone:      false,
				},
				Note: "Important",
			},
			Description: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 string claim with sub-claims.
	require.Len(t, doc.Claims.String, 1)

	claim := doc.Claims.String[0]
	assert.Equal(t, "Main Title", claim.String)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), claim.ID)

	// Check sub-claims.
	require.NotNil(t, claim.Sub)

	// Should have 1 TimeRange and 1 String sub-claim.
	assert.Len(t, claim.Sub.TimeInterval, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "PERIOD", "0"), claim.Sub.TimeInterval[0].ID)
	assert.Len(t, claim.Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), claim.Sub.String[0].ID)
}

func TestDocuments_NestedWithoutValue(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithNestedNoValue{
			ID: []string{"test", "doc1"},
			Address: NestedWithoutValue{
				Location: core.Ref{ID: []string{"location", "1"}},
				Period: core.Interval[core.Time]{
					From:          &core.Time{Time: start, Precision: document.TimePrecisionYear},
					FromIsOpen:    false,
					FromIsUnknown: false,
					FromIsNone:    false,
					To:            &core.Time{Time: end, Precision: document.TimePrecisionYear},
					ToIsClosed:    false,
					ToIsUnknown:   false,
					ToIsNone:      false,
				},
			},
			History: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nested struct without value field creates a HasClaim for the ADDRESS property,
	// and the Location and Period fields become sub-claims on that HasClaim.
	require.Len(t, doc.Claims.Has, 1)

	hasClaim := doc.Claims.Has[0]
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0"), hasClaim.ID)
	require.NotNil(t, hasClaim.Sub)

	// Should have 1 Relation and 1 TimeRange as sub-claims.
	assert.Len(t, hasClaim.Sub.Reference, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "LOCATION", "0"), hasClaim.Sub.Reference[0].ID)
	assert.Len(t, hasClaim.Sub.TimeInterval, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "PERIOD", "0"), hasClaim.Sub.TimeInterval[0].ID)
}

func TestDocuments_SkippedFields(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithSkippedFields{
			ID:           []string{"test", "doc1"},
			Name:         "Test",
			Internal:     "Should be skipped",
			SkipExplicit: "Also skipped",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should only have Name claim.
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "Test", doc.Claims.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
}

func TestDocuments_NilPointer(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithPointer{
			ID:       []string{"test", "doc1"},
			Optional: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil pointer should be skipped.
	assert.Empty(t, doc.Claims.Reference)
}

func TestDocuments_NonNilPointer(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	ref := core.Ref{ID: []string{"ref", "1"}}
	docs := []any{
		&DocWithPointer{
			ID:       []string{"test", "doc1"},
			Optional: &ref,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Non-nil pointer should create claim.
	require.Len(t, doc.Claims.Reference, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.Reference[0].ID)
}

func TestDocuments_EmptyRef(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRef{
			ID:       []string{"test", "doc1"},
			Parent:   core.Ref{ID: []string{}}, // Empty ID.
			Children: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// For empty ref which is not required, no claim should be made.
	assert.Equal(t, 0, doc.Claims.Size())
}

func TestDocuments_ZeroValues(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmount{
			ID:     []string{"test", "doc1"},
			Width:  0, // Zero float.
			Height: 0, // Zero int.
			Count:  0, // Zero uint.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Zero values are included as valid amounts.
	require.Len(t, doc.Claims.Amount, 3)

	// Verify all amounts are zero.
	assert.Equal(t, document.Amount("0.00"), doc.Claims.Amount[0].Amount)
	assert.Equal(t, document.Amount("0"), doc.Claims.Amount[1].Amount)
	assert.Equal(t, document.Amount("0"), doc.Claims.Amount[2].Amount)

	assert.Equal(t, identifier.From("test", "doc1", "WIDTH", "0"), doc.Claims.Amount[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "COUNT", "0"), doc.Claims.Amount[2].ID)
}

func TestDocuments_EmptyStrings(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&SimpleDoc{
			ID:   []string{"test", "doc1"},
			Name: "", // Empty string.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// For empty string which is not required, no claim should be made.
	assert.Equal(t, 0, doc.Claims.Size())
}

func TestDocuments_MultipleDocuments(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&SimpleDoc{
			ID:   []string{"test", "doc1"},
			Name: "First",
		},
		&SimpleDoc{
			ID:   []string{"test", "doc2"},
			Name: "Second",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 2)

	expectedID1 := identifier.From("test", "doc1")
	expectedID2 := identifier.From("test", "doc2")

	assert.Equal(t, expectedID1, results[0].ID)
	assert.Equal(t, expectedID2, results[1].ID)

	// Check claim IDs for both documents.
	require.Len(t, results[0].Claims.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), results[0].Claims.String[0].ID)
	require.Len(t, results[1].Claims.String, 1)
	assert.Equal(t, identifier.From("test", "doc2", "NAME", "0"), results[1].Claims.String[0].ID)
}

func TestDocuments_MissingDocumentID(t *testing.T) {
	t.Parallel()

	type DocNoID struct {
		Name string `property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocNoID{
			Name: "Test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	// Just check that we got an error about document ID.
	assert.EqualError(t, errE, "document ID not found")
}

func TestDocuments_MissingPropertyMnemonic(t *testing.T) {
	t.Parallel()

	type DocUnknownProp struct {
		ID      []string `documentid:""`
		Unknown string   `              property:"UNKNOWN_PROP"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocUnknownProp{
			ID:      []string{"test", "doc1"},
			Unknown: "test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "mnemonic not found")
}

func TestDocuments_NotAStruct(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		"not a struct",
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "expected struct")
}

func TestDocuments_EmbeddedDocID(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithEmbedded{
			BaseDocFields: BaseDocFields{ID: []string{"test", "doc1"}},
			Name:          "Test Document",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)

	doc := results[0]
	expectedID := identifier.From("test", "doc1")
	assert.Equal(t, expectedID, doc.ID)

	// Check that Name claim was created.
	require.Len(t, doc.Claims.String, 1)

	assert.Equal(t, "Test Document", doc.Claims.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
}

func TestDocuments_NestedEmbeddedDocID(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithNestedEmbedded{
			MiddleFields: MiddleFields{
				BaseDocFields: BaseDocFields{ID: []string{"test", "doc1"}},
				Description:   "A description",
			},
			Title: "A title",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)

	doc := results[0]
	expectedID := identifier.From("test", "doc1")
	assert.Equal(t, expectedID, doc.ID)

	// Should have both Description and Title.
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "doc1", "DESCRIPTION", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.String[1].ID)
}

func TestDocuments_EmbeddedProperties(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	mnemonics["HAS_AUTHOR"] = identifier.From("test", "HAS_AUTHOR")
	mnemonics["EXTRA"] = identifier.From("test", "EXTRA")

	docs := []any{
		&DocWithEmbeddedProperties{
			ID: []string{"test", "doc1"},
			EmbeddedProperties: EmbeddedProperties{
				Name:   "John Doe",
				Author: []core.Ref{{ID: []string{"author", "1"}}},
			},
			Extra: "Extra info",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have Name and Extra as string claims.
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "EXTRA", "0"), doc.Claims.String[1].ID)

	// Should have Author as relation claim.
	require.Len(t, doc.Claims.Reference, 1)

	expectedAuthorID := identifier.From("author", "1")
	assert.Equal(t, expectedAuthorID, doc.Claims.Reference[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "0"), doc.Claims.Reference[0].ID)
}

func TestDocuments_EmbeddedLikeCore(t *testing.T) {
	t.Parallel()

	// Test pattern similar to core.Property which embeds PropertyFields and DocumentFields.
	type DocumentFields struct {
		ID         []string   `documentid:""`
		InstanceOf []core.Ref `              property:"INSTANCE_OF"`
	}

	type PropertyFields struct {
		Name     string `property:"NAME"`
		Mnemonic string `property:"MNEMONIC"`
	}

	type Property struct {
		PropertyFields
		DocumentFields
	}

	mnemonics := createMnemonics()
	mnemonics["INSTANCE_OF"] = identifier.From("core", "INSTANCE_OF")
	mnemonics["MNEMONIC"] = identifier.From("core", "MNEMONIC")

	docs := []any{
		&Property{
			PropertyFields: PropertyFields{
				Name:     "Test Property",
				Mnemonic: "TEST",
			},
			DocumentFields: DocumentFields{
				ID:         []string{"test", "PROP"},
				InstanceOf: []core.Ref{{ID: []string{"core", "PROPERTY"}}},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)

	doc := results[0]
	expectedID := identifier.From("test", "PROP")
	assert.Equal(t, expectedID, doc.ID)

	// PropertyFields is processed first, then DocumentFields.
	// Should have 2 string claims (Name and Mnemonic).
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "PROP", "NAME", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "PROP", "MNEMONIC", "0"), doc.Claims.String[1].ID)

	// Should have 1 relation claim (InstanceOf).
	require.Len(t, doc.Claims.Reference, 1)

	expectedInstanceOf := identifier.From("core", "PROPERTY")
	assert.Equal(t, expectedInstanceOf, doc.Claims.Reference[0].To.ID)
	assert.Equal(t, identifier.From("test", "PROP", "INSTANCE_OF", "0"), doc.Claims.Reference[0].ID)
}

func TestDocuments_MultipleEmbeddedSameLevel(t *testing.T) {
	t.Parallel()

	// Test when multiple structs are embedded at the same level.
	type IDFields struct {
		ID []string `documentid:""`
	}

	type NameFields struct {
		Name string `property:"NAME"`
	}

	type CodeFields struct {
		Code string `property:"CODE" type:"id"`
	}

	type MultiEmbedded struct {
		IDFields
		NameFields
		CodeFields
	}

	mnemonics := createMnemonics()

	docs := []any{
		&MultiEmbedded{
			IDFields:   IDFields{ID: []string{"test", "doc1"}},
			NameFields: NameFields{Name: "Test"},
			CodeFields: CodeFields{Code: "ABC"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have Name string claim.
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)

	// Should have Code identifier claim.
	require.Len(t, doc.Claims.Identifier, 1)

	assert.Equal(t, "ABC", doc.Claims.Identifier[0].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODE", "0"), doc.Claims.Identifier[0].ID)
}

func TestDocuments_NilRefValueUnknown(t *testing.T) {
	t.Parallel()

	// Test that nil *core.Ref in value field creates NoneClaim.
	type RefWithSub struct {
		Ref  *core.Ref `                value:""`
		Note string    `property:"NOTE"`
	}

	type DocWithNilRefValue struct {
		ID       []string   `documentid:""`
		Optional RefWithSub `              property:"OPTIONAL"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNilRefValue{
			ID: []string{"test", "doc1"},
			Optional: RefWithSub{
				Ref:  nil, // Nil ref.
				Note: "Some note",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil *core.Ref value creates HasClaim for the struct.
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.Has[0].ID)

	// Should have sub-claim (Note).
	require.NotNil(t, doc.Claims.Has[0].Sub)
	assert.Len(t, doc.Claims.Has[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0", "NOTE", "0"), doc.Claims.Has[0].Sub.String[0].ID)
}

func TestDocuments_EmptyRefValue(t *testing.T) {
	t.Parallel()

	type RefWithSub struct {
		Ref  core.Ref `                value:""`
		Note string   `property:"NOTE"`
	}

	type DocWithEmptyRefValue struct {
		ID       []string   `documentid:""`
		Optional RefWithSub `              property:"OPTIONAL"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithEmptyRefValue{
			ID: []string{"test", "doc1"},
			Optional: RefWithSub{
				Ref:  core.Ref{ID: []string{}}, // Empty ref.
				Note: "Some note",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create HasClaim for empty ref with sub-claims.
	assert.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.Has[0].ID)
}

func TestDocuments_MultipleFieldsSameProperty(t *testing.T) {
	t.Parallel()

	// Test that multiple fields with same property tag don't create duplicate IDs.
	type DocWithDuplicateProps struct {
		ID               []string   `documentid:""`
		Author           []core.Ref `              property:"HAS_AUTHOR"`
		AuthorHasUnknown bool       `              property:"HAS_AUTHOR" type:"unknown"`
		Artist           []core.Ref `              property:"HAS_ARTIST"`
		ArtistHasUnknown bool       `              property:"HAS_ARTIST" type:"unknown"`
	}

	mnemonics := createMnemonics()
	mnemonics["HAS_AUTHOR"] = identifier.From("test", "HAS_AUTHOR")
	mnemonics["HAS_ARTIST"] = identifier.From("test", "HAS_ARTIST")

	docs := []any{
		&DocWithDuplicateProps{
			ID: []string{"test", "doc1"},
			Author: []core.Ref{
				{ID: []string{"author", "1"}},
			},
			AuthorHasUnknown: true, // Creates claim with same property.
			Artist: []core.Ref{
				{ID: []string{"artist", "1"}},
			},
			ArtistHasUnknown: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 2 relation claims (Author[0] and Artist[0]).
	require.Len(t, doc.Claims.Reference, 2)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "0"), doc.Claims.Reference[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_ARTIST", "0"), doc.Claims.Reference[1].ID)

	// Should have 2 unknown value claims (AuthorHasUnknown and ArtistHasUnknown).
	require.Len(t, doc.Claims.Unknown, 2)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "1"), doc.Claims.Unknown[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_ARTIST", "1"), doc.Claims.Unknown[1].ID)
}

func TestDocuments_CoreHTMLWithoutTag(t *testing.T) {
	t.Parallel()

	// Test that core.HTML without html tag is treated as HTML.
	type DocWithHTML struct {
		ID   []string  `documentid:""`
		HTML core.HTML `              property:"HTML"` // No html tag.
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithHTML{
			ID:   []string{"test", "doc1"},
			HTML: "<p>HTML content</p>",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without html tag, should still be TextClaim.
	require.Len(t, doc.Claims.HTML, 1)

	assert.Equal(t, "&lt;p&gt;HTML content&lt;/p&gt;", doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.HTML[0].ID)
}

func TestDocuments_CoreRawHTMLWithoutTag(t *testing.T) {
	t.Parallel()

	// Test that core.RawHTML without rawhtml tag is treated as raw HTML (not escaped).
	type DocWithRawHTML struct {
		ID      []string     `documentid:""`
		RawHTML core.RawHTML `              property:"HTML"` // No rawhtml tag.
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithRawHTML{
			ID:      []string{"test", "doc1"},
			RawHTML: "<p>HTML content</p>",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without rawhtml tag, should still be TextClaim with unescaped HTML.
	require.Len(t, doc.Claims.HTML, 1)

	assert.Equal(t, "<p>HTML content</p>", doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.HTML[0].ID)
}

func TestDocuments_HTMLvsRawHTMLEscaping(t *testing.T) {
	t.Parallel()

	// Test that HTML is escaped but RawHTML is not.
	type DocComparison struct {
		ID            []string     `documentid:""`
		EscapedHTML   string       `              property:"ESCAPED"   type:"html"`
		UnescapedHTML string       `              property:"UNESCAPED" type:"rawhtml"`
		CoreHTML      core.HTML    `              property:"CORE_HTML"`
		CoreRawHTML   core.RawHTML `              property:"CORE_RAW"`
	}

	mnemonics := createMnemonics()
	mnemonics["ESCAPED"] = identifier.From("test", "ESCAPED")
	mnemonics["UNESCAPED"] = identifier.From("test", "UNESCAPED")
	mnemonics["CORE_HTML"] = identifier.From("test", "CORE_HTML")
	mnemonics["CORE_RAW"] = identifier.From("test", "CORE_RAW")

	testHTML := "<script>alert('xss')</script>"
	docs := []any{
		&DocComparison{
			ID:            []string{"test", "doc1"},
			EscapedHTML:   testHTML,
			UnescapedHTML: testHTML,
			CoreHTML:      core.HTML(testHTML),
			CoreRawHTML:   core.RawHTML(testHTML),
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// UNESCAPED and CORE_RAW claims are skipped because sanitization strips all content from <script> tags.
	require.Len(t, doc.Claims.HTML, 2)

	// Verify HTML is escaped.
	escapedExpected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	assert.Equal(t, escapedExpected, doc.Claims.HTML[0].HTML, "type:html should escape")
	assert.Equal(t, escapedExpected, doc.Claims.HTML[1].HTML, "core.HTML should escape")

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "ESCAPED", "0"), doc.Claims.HTML[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CORE_HTML", "0"), doc.Claims.HTML[1].ID)
}

func TestDocuments_HTMLvsRawHTMLEscapingWithSurroundingText(t *testing.T) {
	t.Parallel()

	// Test that HTML is escaped but RawHTML is not, with text surrounding the script tag
	// so that sanitization leaves a non-empty result.
	type DocComparison struct {
		ID            []string     `documentid:""`
		EscapedHTML   string       `              property:"ESCAPED"   type:"html"`
		UnescapedHTML string       `              property:"UNESCAPED" type:"rawhtml"`
		CoreHTML      core.HTML    `              property:"CORE_HTML"`
		CoreRawHTML   core.RawHTML `              property:"CORE_RAW"`
	}

	mnemonics := createMnemonics()
	mnemonics["ESCAPED"] = identifier.From("test", "ESCAPED")
	mnemonics["UNESCAPED"] = identifier.From("test", "UNESCAPED")
	mnemonics["CORE_HTML"] = identifier.From("test", "CORE_HTML")
	mnemonics["CORE_RAW"] = identifier.From("test", "CORE_RAW")

	testHTML := "hello <script>alert('xss')</script> world"
	docs := []any{
		&DocComparison{
			ID:            []string{"test", "doc1"},
			EscapedHTML:   testHTML,
			UnescapedHTML: testHTML,
			CoreHTML:      core.HTML(testHTML),
			CoreRawHTML:   core.RawHTML(testHTML),
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// All 4 claims are created because sanitization leaves non-empty content.
	require.Len(t, doc.Claims.HTML, 4)

	// Verify HTML is escaped (type:html and core.HTML escape special characters first,
	// so the script tag becomes text and survives sanitization unchanged).
	escapedExpected := "hello &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; world"
	assert.Equal(t, escapedExpected, doc.Claims.HTML[0].HTML, "type:html should escape")
	assert.Equal(t, escapedExpected, doc.Claims.HTML[2].HTML, "core.HTML should escape")

	// Verify RawHTML is sanitized (script tag and its content are stripped, surrounding text remains).
	sanitizedExpected := "hello  world"
	assert.Equal(t, sanitizedExpected, doc.Claims.HTML[1].HTML, "type:rawhtml should sanitize")
	assert.Equal(t, sanitizedExpected, doc.Claims.HTML[3].HTML, "core.RawHTML should sanitize")

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "ESCAPED", "0"), doc.Claims.HTML[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "UNESCAPED", "0"), doc.Claims.HTML[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CORE_HTML", "0"), doc.Claims.HTML[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CORE_RAW", "0"), doc.Claims.HTML[3].ID)
}

func TestDocuments_CoreIRIWithoutTag(t *testing.T) {
	t.Parallel()

	// Test that core.IRI without iri tag is treated as IRI.
	type DocWithPlainIRI struct {
		ID       []string `documentid:""`
		PlainIRI core.IRI `              property:"PLAIN_IRI"` // No iri tag.
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithPlainIRI{
			ID:       []string{"test", "doc1"},
			PlainIRI: "https://example.com",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without iri tag, should still be LinkClaim.
	require.Len(t, doc.Claims.Link, 1)

	assert.Equal(t, "https://example.com", doc.Claims.Link[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "PLAIN_IRI", "0"), doc.Claims.Link[0].ID)
}

func TestDocuments_ZeroTimeSkipped(t *testing.T) {
	t.Parallel()

	type DocWithZeroTime struct {
		ID          []string  `documentid:""`
		ValidTime   core.Time `              property:"VALID_TIME"`
		InvalidTime core.Time `              property:"INVALID_TIME"` // Will be zero.
	}

	mnemonics := createMnemonics()
	mnemonics["VALID_TIME"] = identifier.From("test", "VALID_TIME")
	mnemonics["INVALID_TIME"] = identifier.From("test", "INVALID_TIME")

	now := time.Now()
	docs := []any{
		&DocWithZeroTime{
			ID: []string{"test", "doc1"},
			ValidTime: core.Time{
				Time:      now,
				Precision: document.TimePrecisionSecond,
			},
			InvalidTime: core.Time{}, // Zero value.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only ValidTime.
	assert.Len(t, doc.Claims.Time, 1)
	assert.Equal(t, identifier.From("test", "doc1", "VALID_TIME", "0"), doc.Claims.Time[0].ID)
}

func TestDocuments_EmptyIntervalSkipped(t *testing.T) {
	t.Parallel()

	// Test that partial intervals (missing From or To) are skipped.
	type DocWithEmptyInterval struct {
		ID            []string                 `documentid:""`
		ValidPeriod   core.Interval[core.Time] `              property:"VALID_PERIOD"`
		InvalidPeriod core.Interval[core.Time] `              property:"INVALID_PERIOD"` // Will be empty.
	}

	mnemonics := createMnemonics()
	mnemonics["VALID_PERIOD"] = identifier.From("test", "VALID_PERIOD")
	mnemonics["INVALID_PERIOD"] = identifier.From("test", "INVALID_PERIOD")

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := []any{
		&DocWithEmptyInterval{
			ID: []string{"test", "doc1"},
			ValidPeriod: core.Interval[core.Time]{
				From:          &core.Time{Time: start, Precision: document.TimePrecisionYear},
				FromIsOpen:    false,
				FromIsUnknown: false,
				FromIsNone:    false,
				To:            &core.Time{Time: end, Precision: document.TimePrecisionYear},
				ToIsClosed:    false,
				ToIsUnknown:   false,
				ToIsNone:      false,
			},
			InvalidPeriod: core.Interval[core.Time]{
				From:          nil,
				FromIsOpen:    false,
				FromIsUnknown: false,
				FromIsNone:    false,
				To:            nil,
				ToIsClosed:    false,
				ToIsUnknown:   false,
				ToIsNone:      false,
			}, // Empty interval.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only ValidPeriod should create a claim (empty interval is skipped).
	require.Len(t, doc.Claims.TimeInterval, 1, "empty interval skipped")
	assert.Equal(t, identifier.From("test", "doc1", "VALID_PERIOD", "0"), doc.Claims.TimeInterval[0].ID)
}

func TestDocuments_UniqueClaimIDsWithFieldName(t *testing.T) {
	t.Parallel()

	// Test that field name ensures unique claim IDs.
	type DocWithSamePropertyDifferentFields struct {
		ID     []string `documentid:""`
		Field1 string   `              property:"SHARED"`
		Field2 string   `              property:"SHARED"`
		Field3 string   `              property:"SHARED"`
	}

	mnemonics := createMnemonics()
	mnemonics["SHARED"] = identifier.From("test", "SHARED")

	docs := []any{
		&DocWithSamePropertyDifferentFields{
			ID:     []string{"test", "doc1"},
			Field1: "Value 1",
			Field2: "Value 2",
			Field3: "Value 3",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 3 string claims with unique IDs.
	require.Len(t, doc.Claims.String, 3)

	// Verify all claim IDs are unique.
	id1 := doc.Claims.String[0].ID.String()
	id2 := doc.Claims.String[1].ID.String()
	id3 := doc.Claims.String[2].ID.String()

	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id1, id3)
	assert.NotEqual(t, id2, id3)
}

func TestDocuments_PointerToRefInValue(t *testing.T) {
	t.Parallel()

	// Test value:"" with *core.Ref (pointer to core.Ref).
	type RefValue struct {
		Ref  *core.Ref `                value:""`
		Note string    `property:"NOTE"`
	}

	type DocWithPointerRefValue struct {
		ID     []string `documentid:""`
		Target RefValue `              property:"TARGET"`
	}

	mnemonics := createMnemonics()
	mnemonics["TARGET"] = identifier.From("test", "TARGET")

	ref := core.Ref{ID: []string{"target", "1"}}
	docs := []any{
		&DocWithPointerRefValue{
			ID: []string{"test", "doc1"},
			Target: RefValue{
				Ref:  &ref,
				Note: "Important",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 relation claim.
	require.Len(t, doc.Claims.Reference, 1)

	expectedRefID := identifier.From("target", "1")
	assert.Equal(t, expectedRefID, doc.Claims.Reference[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "TARGET", "0"), doc.Claims.Reference[0].ID)

	// Should have sub-claim (Note).
	require.NotNil(t, doc.Claims.Reference[0].Sub)
	assert.Len(t, doc.Claims.Reference[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TARGET", "0", "NOTE", "0"), doc.Claims.Reference[0].Sub.String[0].ID)
}

func TestDocuments_HTMLEscaping(t *testing.T) {
	t.Parallel()

	// Test that HTML in TextClaim is properly escaped.
	type DocWithRawHTML struct {
		ID      []string `documentid:""`
		Content string   `              property:"CONTENT" type:"html"`
	}

	mnemonics := createMnemonics()
	mnemonics["CONTENT"] = identifier.From("test", "CONTENT")

	docs := []any{
		&DocWithRawHTML{
			ID:      []string{"test", "doc1"},
			Content: "<script>alert('xss')</script>",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.HTML, 1)

	// HTML should be escaped.
	expected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	assert.Equal(t, expected, doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.HTML[0].ID)
}

func TestDocuments_ValueFieldWithPointerString(t *testing.T) {
	t.Parallel()

	// Test value:"" with *string (pointer to string).
	type StringValue struct {
		Value *string `                value:""`
		Note  string  `property:"NOTE"`
	}

	type DocWithPointerStringValue struct {
		ID    []string    `documentid:""`
		Title StringValue `              property:"TITLE"`
	}

	mnemonics := createMnemonics()

	str := "Test Title"
	docs := []any{
		&DocWithPointerStringValue{
			ID: []string{"test", "doc1"},
			Title: StringValue{
				Value: &str,
				Note:  "Important",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 string claim.
	require.Len(t, doc.Claims.String, 1)

	assert.Equal(t, "Test Title", doc.Claims.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.String[0].ID)

	// Should have sub-claim (Note).
	require.NotNil(t, doc.Claims.String[0].Sub)
	assert.Len(t, doc.Claims.String[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), doc.Claims.String[0].Sub.String[0].ID)
}

func TestDocuments_ValueFieldWithPointerStringNil(t *testing.T) {
	t.Parallel()

	// Test value:"" with *string that is nil (no required tag).
	type StringValue struct {
		Value *string `                value:""`
		Note  string  `property:"NOTE"`
	}

	type DocWithNilStringValue struct {
		ID    []string    `documentid:""`
		Title StringValue `              property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNilStringValue{
			ID: []string{"test", "doc1"},
			Title: StringValue{
				Value: nil,
				Note:  "Note text",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil value creates HasClaim (has sub-claims).
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.Has[0].ID)

	// Should have sub-claim.
	require.NotNil(t, doc.Claims.Has[0].Sub)
	assert.Len(t, doc.Claims.Has[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), doc.Claims.Has[0].Sub.String[0].ID)
}

func TestDocuments_ValueFieldWithPointerStringRequired(t *testing.T) {
	t.Parallel()

	// Test value:"" with *string, cardinality:"1", default:"none", and nil value.
	type StringValue struct {
		Value *string `                value:""`
		Note  string  `property:"NOTE"`
	}

	type DocWithRequiredStringValue struct {
		ID    []string    `                               documentid:""`
		Title StringValue `cardinality:"1" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithRequiredStringValue{
			ID: []string{"test", "doc1"},
			Title: StringValue{
				Value: nil,
				Note:  "Note text",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil value with sub-claims creates HasClaim (default:"none" applies to cardinality, not claim type).
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.Has[0].ID)
}

func TestDocuments_ValueFieldWithPointerTime(t *testing.T) {
	t.Parallel()

	// Test value:"" with *core.Time (pointer to core.Time).
	type TimeValue struct {
		Value *core.Time `                value:""`
		Note  string     `property:"NOTE"`
	}

	type DocWithPointerTimeValue struct {
		ID      []string  `documentid:""`
		Created TimeValue `              property:"CREATED"`
	}

	mnemonics := createMnemonics()
	mnemonics["CREATED"] = identifier.From("test", "CREATED")

	now := time.Now()
	coreTime := core.Time{
		Time:      now,
		Precision: document.TimePrecisionSecond,
	}

	docs := []any{
		&DocWithPointerTimeValue{
			ID: []string{"test", "doc1"},
			Created: TimeValue{
				Value: &coreTime,
				Note:  "Important date",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 time claim.
	require.Len(t, doc.Claims.Time, 1)

	tt, errE := doc.Claims.Time[0].Time.Time(doc.Claims.Time[0].Precision, time.UTC)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, now.UTC().Truncate(time.Second), tt)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0"), doc.Claims.Time[0].ID)

	// Should have sub-claim (Note).
	require.NotNil(t, doc.Claims.Time[0].Sub)
	assert.Len(t, doc.Claims.Time[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0", "NOTE", "0"), doc.Claims.Time[0].Sub.String[0].ID)
}

func TestDocuments_ValueFieldWithTime(t *testing.T) {
	t.Parallel()

	// Test value:"" with core.Time (non-pointer).
	type TimeValue struct {
		Value core.Time `                value:""`
		Note  string    `property:"NOTE"`
	}

	type DocWithTimeValue struct {
		ID      []string  `documentid:""`
		Created TimeValue `              property:"CREATED"`
	}

	mnemonics := createMnemonics()
	mnemonics["CREATED"] = identifier.From("test", "CREATED")

	now := time.Now()
	docs := []any{
		&DocWithTimeValue{
			ID: []string{"test", "doc1"},
			Created: TimeValue{
				Value: core.Time{
					Time:      now,
					Precision: document.TimePrecisionSecond,
				},
				Note: "Important date",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 time claim.
	require.Len(t, doc.Claims.Time, 1)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0"), doc.Claims.Time[0].ID)
}

func TestDocuments_ValueFieldWithIdentifier(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test value:"" with string and type:"id" tag.
	type IDValue struct {
		Value string `                type:"id" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithIDValue struct {
		ID   []string `documentid:""`
		Code IDValue  `              property:"CODE"`
	}

	mnemonics := createMnemonics()
	mnemonics["CODE"] = identifier.From("test", "CODE")

	docs := []any{
		&DocWithIDValue{
			ID: []string{"test", "doc1"},
			Code: IDValue{
				Value: "ABC123",
				Note:  "Primary code",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 identifier claim.
	require.Len(t, doc.Claims.Identifier, 1)

	assert.Equal(t, "ABC123", doc.Claims.Identifier[0].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODE", "0"), doc.Claims.Identifier[0].ID)
}

func TestDocuments_ValueFieldWithHTMLTag(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test value:"" with type:"html" tag.
	type HTMLValue struct {
		Value string `                type:"html" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithHTMLValue struct {
		ID      []string  `documentid:""`
		Content HTMLValue `              property:"CONTENT"`
	}

	mnemonics := createMnemonics()
	mnemonics["CONTENT"] = identifier.From("test", "CONTENT")

	docs := []any{
		&DocWithHTMLValue{
			ID: []string{"test", "doc1"},
			Content: HTMLValue{
				Value: "<p>Test</p>",
				Note:  "HTML content",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 text claim.
	require.Len(t, doc.Claims.HTML, 1)

	// HTML should be escaped.
	assert.Equal(t, "&lt;p&gt;Test&lt;/p&gt;", doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.HTML[0].ID)
}

func TestDocuments_ValueFieldWithRawHTMLTag(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test value:"" with type:"rawhtml" tag.
	type RawHTMLValue struct {
		Value string `                type:"rawhtml" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithRawHTMLValue struct {
		ID      []string     `documentid:""`
		Content RawHTMLValue `              property:"CONTENT"`
	}

	mnemonics := createMnemonics()
	mnemonics["CONTENT"] = identifier.From("test", "CONTENT")

	docs := []any{
		&DocWithRawHTMLValue{
			ID: []string{"test", "doc1"},
			Content: RawHTMLValue{
				Value: "<p>Test</p>",
				Note:  "Raw HTML content",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 text claim.
	require.Len(t, doc.Claims.HTML, 1)

	// HTML should NOT be escaped for rawhtml type.
	assert.Equal(t, "<p>Test</p>", doc.Claims.HTML[0].HTML)
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.HTML[0].ID)
}

func TestDocuments_ValueFieldWithIRITag(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test value:"" with type:"iri" tag.
	type IRIValue struct {
		Value string `                type:"iri" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithIRIValue struct {
		ID   []string `documentid:""`
		Link IRIValue `              property:"LINK"`
	}

	mnemonics := createMnemonics()
	mnemonics["LINK"] = identifier.From("test", "LINK")

	docs := []any{
		&DocWithIRIValue{
			ID: []string{"test", "doc1"},
			Link: IRIValue{
				Value: "https://example.com",
				Note:  "Homepage",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 reference claim.
	require.Len(t, doc.Claims.Link, 1)

	assert.Equal(t, "https://example.com", doc.Claims.Link[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "LINK", "0"), doc.Claims.Link[0].ID)
}

func TestDocuments_ValueFieldUnsupportedSlice(t *testing.T) {
	t.Parallel()

	// Test that value:"" with slice type returns error.
	type SliceValue struct {
		Value []string `                value:""`
		Note  string   `property:"NOTE"`
	}

	type DocWithSliceValue struct {
		ID    []string   `documentid:""`
		Items SliceValue `              property:"ITEMS"`
	}

	mnemonics := createMnemonics()
	mnemonics["ITEMS"] = identifier.From("test", "ITEMS")

	docs := []any{
		&DocWithSliceValue{
			ID: []string{"test", "doc1"},
			Items: SliceValue{
				Value: []string{"a", "b"},
				Note:  "List",
			},
		},
	}

	// Slices are not supported in value:"" fields.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "field has unsupported or unexpected value type")
}

func TestDocuments_ValueFieldUnsupportedStruct(t *testing.T) {
	t.Parallel()

	// Test that value:"" with non-core struct type returns error.
	type CustomStruct struct {
		Field1 string
		Field2 int
	}

	type StructValue struct {
		Value CustomStruct `                value:""`
		Note  string       `property:"NOTE"`
	}

	type DocWithStructValue struct {
		ID   []string    `documentid:""`
		Data StructValue `              property:"DATA"`
	}

	mnemonics := createMnemonics()
	mnemonics["DATA"] = identifier.From("test", "DATA")

	docs := []any{
		&DocWithStructValue{
			ID: []string{"test", "doc1"},
			Data: StructValue{
				Value: CustomStruct{Field1: "test", Field2: 42},
				Note:  "Data",
			},
		},
	}

	// Non-core structs are not supported in value:"" fields.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "field has unsupported or unexpected value type")
}

func TestDocuments_ValueFieldWithAmount(t *testing.T) {
	t.Parallel()

	// Test value:"" with numeric type.
	type AmountValue struct {
		Value float64 `precision:"0.01"                 value:""`
		Note  string  `                 property:"NOTE"`
	}

	type DocWithAmountValue struct {
		ID     []string    `documentid:""`
		Height AmountValue `              property:"HEIGHT"`
	}

	mnemonics := createMnemonics()
	mnemonics["HEIGHT"] = identifier.From("test", "HEIGHT")

	docs := []any{
		&DocWithAmountValue{
			ID: []string{"test", "doc1"},
			Height: AmountValue{
				Value: 1.75,
				Note:  "In meters",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 amount claim.
	require.Len(t, doc.Claims.Amount, 1)

	assert.Equal(t, document.Amount("1.75"), doc.Claims.Amount[0].Amount)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[0].ID)

	// Should have sub-claim.
	require.NotNil(t, doc.Claims.Amount[0].Sub)
	assert.Len(t, doc.Claims.Amount[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0", "NOTE", "0"), doc.Claims.Amount[0].Sub.String[0].ID)
}

func TestDocuments_ValueFieldWithBool(t *testing.T) {
	t.Parallel()

	// Test value:"" with bool type.
	type BoolValue struct {
		Value bool   `                value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithBoolValue struct {
		ID     []string  `documentid:""`
		Active BoolValue `              property:"ACTIVE"`
	}

	mnemonics := createMnemonics()
	mnemonics["ACTIVE"] = identifier.From("test", "ACTIVE")

	docs := []any{
		&DocWithBoolValue{
			ID: []string{"test", "doc1"},
			Active: BoolValue{
				Value: true,
				Note:  "Status",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Bool true creates HasClaim.
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0"), doc.Claims.Has[0].ID)

	// Should have sub-claim.
	require.NotNil(t, doc.Claims.Has[0].Sub)
	assert.Len(t, doc.Claims.Has[0].Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0", "NOTE", "0"), doc.Claims.Has[0].Sub.String[0].ID)
}

func TestDocuments_ValueFieldWithBoolFalse(t *testing.T) {
	t.Parallel()

	// Test value:"" with bool false - should be skipped.
	type BoolValue struct {
		Value bool   `                value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithBoolValue struct {
		ID     []string  `documentid:""`
		Active BoolValue `              property:"ACTIVE"`
	}

	mnemonics := createMnemonics()
	mnemonics["ACTIVE"] = identifier.From("test", "ACTIVE")

	docs := []any{
		&DocWithBoolValue{
			ID: []string{"test", "doc1"},
			Active: BoolValue{
				Value: false,
				Note:  "Status",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Bool false is empty, so HasClaim for the struct (has sub-claims).
	require.Len(t, doc.Claims.Has, 1, "false bool creates has claim for struct when sub-claims exist")
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0"), doc.Claims.Has[0].ID)
}

func TestDocuments_StructWithoutValueField(t *testing.T) {
	t.Parallel()

	// Test struct without value:"" field - all fields become sub-claims.
	type NoValueStruct struct {
		Location core.Ref `property:"LOCATION"`
		Note     string   `property:"NOTE"`
	}

	type DocWithoutValueField struct {
		ID      []string      `documentid:""`
		Address NoValueStruct `              property:"ADDRESS"`
	}

	mnemonics := createMnemonics()
	mnemonics["LOCATION"] = identifier.From("test", "LOCATION")

	docs := []any{
		&DocWithoutValueField{
			ID: []string{"test", "doc1"},
			Address: NoValueStruct{
				Location: core.Ref{ID: []string{"loc", "1"}},
				Note:     "Home",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without value field, creates HasClaim with sub-claims.
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0"), doc.Claims.Has[0].ID)

	hasClaim := doc.Claims.Has[0]
	require.NotNil(t, hasClaim.Sub)

	// Should have Location and Note as sub-claims.
	assert.Len(t, hasClaim.Sub.Reference, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "LOCATION", "0"), hasClaim.Sub.Reference[0].ID)
	assert.Len(t, hasClaim.Sub.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "NOTE", "0"), hasClaim.Sub.String[0].ID)
}

func TestDocuments_RequiredPointerNilSlice(t *testing.T) {
	t.Parallel()

	// Test cardinality:"1.." default:"none" with nil slice of pointers.
	type DocWithRequiredPointerSlice struct {
		ID    []string    `                                 documentid:""`
		Items []*core.Ref `cardinality:"1.." default:"none"               property:"ITEMS"`
	}

	mnemonics := createMnemonics()
	mnemonics["ITEMS"] = identifier.From("test", "ITEMS")

	docs := []any{
		&DocWithRequiredPointerSlice{
			ID:    []string{"test", "doc1"},
			Items: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil slice with required creates NoneClaim.
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_RequiredPointerFieldNil(t *testing.T) {
	t.Parallel()

	// Test cardinality:"1" default:"none" with nil pointer field (not in slice).
	type DocWithRequiredPointer struct {
		ID       []string  `                               documentid:""`
		Optional *core.Ref `cardinality:"1" default:"none"               property:"OPTIONAL"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithRequiredPointer{
			ID:       []string{"test", "doc1"},
			Optional: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil pointer with required creates NoneClaim.
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_NestedStructInSlice(t *testing.T) {
	t.Parallel()

	// Test slice of structs with value fields.
	type ItemValue struct {
		Value string `                value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithStructSlice struct {
		ID    []string    `documentid:""`
		Items []ItemValue `              property:"ITEMS"`
	}

	mnemonics := createMnemonics()
	mnemonics["ITEMS"] = identifier.From("test", "ITEMS")

	docs := []any{
		&DocWithStructSlice{
			ID: []string{"test", "doc1"},
			Items: []ItemValue{
				{Value: "Item 1", Note: "First"},
				{Value: "Item 2", Note: "Second"},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 2 string claims (one for each item).
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "1"), doc.Claims.String[1].ID)

	// Each should have sub-claims.
	for i, claim := range doc.Claims.String {
		require.NotNil(t, claim.Sub, "claim %d", i)
		assert.Len(t, claim.Sub.String, 1, "claim %d", i)
	}
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0", "NOTE", "0"), doc.Claims.String[0].Sub.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "1", "NOTE", "0"), doc.Claims.String[1].Sub.String[0].ID)
}

func TestDocuments_MultipleValueFieldsError(t *testing.T) {
	t.Parallel()

	// Test that multiple value:"" fields in same struct returns error.
	type MultiValueStruct struct {
		Value1 string `                value:""`
		Value2 string `                value:""`
		Note   string `property:"NOTE"`
	}

	type DocWithMultiValue struct {
		ID   []string         `documentid:""`
		Data MultiValueStruct `              property:"DATA"`
	}

	mnemonics := createMnemonics()
	mnemonics["DATA"] = identifier.From("test", "DATA")

	docs := []any{
		&DocWithMultiValue{
			ID: []string{"test", "doc1"},
			Data: MultiValueStruct{
				Value1: "First",
				Value2: "Second",
				Note:   "Test",
			},
		},
	}

	// Multiple value fields should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "multiple value claims found")
}

func TestDocuments_ValueAndPropertyTagError(t *testing.T) {
	t.Parallel()

	// Test that value:"" and property:"" together returns error.
	type InvalidStruct struct {
		Value string `property:"VALUE" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithInvalidValue struct {
		ID   []string      `documentid:""`
		Data InvalidStruct `              property:"DATA"`
	}

	mnemonics := createMnemonics()
	mnemonics["DATA"] = identifier.From("test", "DATA")
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	docs := []any{
		&DocWithInvalidValue{
			ID: []string{"test", "doc1"},
			Data: InvalidStruct{
				Value: "Test",
				Note:  "Note",
			},
		},
	}

	// value and property together should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "property tag cannot be used with value tag")
}

func TestDocuments_EmptyDocumentIDError(t *testing.T) {
	t.Parallel()

	// Test that empty document ID slice returns error.
	type DocEmptyID struct {
		ID   []string `documentid:""`
		Name string   `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocEmptyID{
			ID:   []string{}, // Empty slice.
			Name: "Test",
		},
	}

	// Empty document ID should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "empty ID")
}

func TestDocuments_MultipleDocumentIDsError(t *testing.T) {
	t.Parallel()

	// Test that multiple documentid fields returns error.
	type MultiIDDoc struct {
		ID1  []string `documentid:""`
		ID2  []string `documentid:""`
		Name string   `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&MultiIDDoc{
			ID1:  []string{"test", "id1"},
			ID2:  []string{"test", "id2"},
			Name: "Test",
		},
	}

	// Multiple document IDs should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "multiple document IDs found")
}

func TestDocuments_InfinityFloatError(t *testing.T) {
	t.Parallel()

	// Test that infinity float returns error.
	type DocWithInfinity struct {
		ID    []string `documentid:""`
		Value float64  `              precision:"1" property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	docs := []any{
		&DocWithInfinity{
			ID:    []string{"test", "doc1"},
			Value: math.Inf(1),
		},
	}

	// Infinity should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "value is infinity or not a number")
}

func TestDocuments_NaNFloatError(t *testing.T) {
	t.Parallel()

	// Test that NaN float returns error.
	type DocWithNaN struct {
		ID    []string `documentid:""`
		Value float64  `              precision:"1" property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	docs := []any{
		&DocWithNaN{
			ID:    []string{"test", "doc1"},
			Value: math.NaN(),
		},
	}

	// NaN should error.
	_, errE := transform.Documents(t.Context(), mnemonics, docs)

	assert.EqualError(t, errE, "value is infinity or not a number")
}

func TestDocuments_EmbeddedStructWithPropertySkip(t *testing.T) {
	t.Parallel()

	// Test that embedded struct with property:"-" is skipped.
	type SkippedEmbedded struct {
		Field1 string `property:"FIELD1"`
		Field2 string `property:"FIELD2"`
	}

	type DocWithSkippedEmbedded struct {
		SkippedEmbedded `property:"-"`

		ID    []string `documentid:""`
		Name  string   `              property:"NAME"`
		Extra string   `              property:"EXTRA"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD1"] = identifier.From("test", "FIELD1")
	mnemonics["FIELD2"] = identifier.From("test", "FIELD2")
	mnemonics["EXTRA"] = identifier.From("test", "EXTRA")

	docs := []any{
		&DocWithSkippedEmbedded{
			SkippedEmbedded: SkippedEmbedded{
				Field1: "Value1",
				Field2: "Value2",
			},
			ID:    []string{"test", "doc1"},
			Name:  "Test",
			Extra: "Extra",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only Name and Extra should be processed (embedded struct skipped).
	require.Len(t, doc.Claims.String, 2)

	// Verify the claims are Name and Extra, not Field1 or Field2.
	propIDs := []identifier.Identifier{
		doc.Claims.String[0].Prop.ID,
		doc.Claims.String[1].Prop.ID,
	}

	nameID := mnemonics["NAME"]
	extraID := mnemonics["EXTRA"]

	assert.True(t, (propIDs[0] == nameID && propIDs[1] == extraID) || (propIDs[0] == extraID && propIDs[1] == nameID), "expected Name and Extra properties, got %v", propIDs)

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "EXTRA", "0"), doc.Claims.String[1].ID)
}

func TestDocuments_ComplexSharedPropertyWithSubAndValue(t *testing.T) {
	t.Parallel()

	// Test complex scenario: multiple sources contributing to same property,
	// including multiple embedded structs, top-level fields, value fields, and sub-claim fields.
	type EmbeddedSharedA struct {
		Field1 string `property:"SHARED"`
		Field2 string `property:"SHARED"`
	}

	type EmbeddedSharedB struct {
		Field3 []string `property:"SHARED"` // Slice in embedded struct.
	}

	type ValueWithSubShared struct {
		Value     string   `                  value:""` // Value for SHARED property.
		SubShared []string `property:"SHARED"`          // Sub-claim field also for SHARED property.
		SubNote   string   `property:"NOTE"`            // Sub-claim field for different property.
	}

	type DocComplex struct {
		EmbeddedSharedA
		EmbeddedSharedB

		ID []string `documentid:""`

		TopShared1 string             `property:"SHARED"` // Top-level field for SHARED.
		TopShared2 []string           `property:"SHARED"` // Slice creates multiple claims.
		WithValue  ValueWithSubShared `property:"SHARED"` // Value field for SHARED with sub-claims.
	}

	mnemonics := createMnemonics()
	mnemonics["SHARED"] = identifier.From("test", "SHARED")

	docs := []any{
		&DocComplex{
			EmbeddedSharedA: EmbeddedSharedA{
				Field1: "From Embedded A1",
				Field2: "From Embedded A2",
			},
			EmbeddedSharedB: EmbeddedSharedB{
				Field3: []string{"From Embedded B1", "From Embedded B2"},
			},
			ID:         []string{"test", "complex"},
			TopShared1: "From Top 1",
			TopShared2: []string{"From Top 2a", "From Top 2b"},
			WithValue: ValueWithSubShared{
				Value:     "From Value",
				SubShared: []string{"Sub Shared 1", "Sub Shared 2"},
				SubNote:   "Sub Note",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// All claims should be for SHARED property:
	// - Field1 (embedded A) -> claim 0
	// - Field2 (embedded A) -> claim 1
	// - Field3[0] (embedded B) -> claim 2
	// - Field3[1] (embedded B) -> claim 3
	// - TopShared1 -> claim 4
	// - TopShared2[0] -> claim 5
	// - TopShared2[1] -> claim 6
	// - WithValue.Value -> claim 7
	// Total: 8 string claims for SHARED property.
	require.Len(t, doc.Claims.String, 8)

	sharedPropID := mnemonics["SHARED"]

	// Verify all claims have the same property.
	for i, claim := range doc.Claims.String {
		assert.Equal(t, sharedPropID, claim.Prop.ID, "claim %d", i)
	}

	// Verify claim IDs increment correctly across all sources.
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "1"), doc.Claims.String[1].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "2"), doc.Claims.String[2].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "3"), doc.Claims.String[3].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "4"), doc.Claims.String[4].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "5"), doc.Claims.String[5].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "6"), doc.Claims.String[6].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7"), doc.Claims.String[7].ID)

	// Verify the value claim (claim 7) has sub-claims.
	valueClaim := doc.Claims.String[7]
	require.NotNil(t, valueClaim.Sub)

	// Sub-claims: SubShared creates 2 string claims for SHARED, SubNote creates 1 for NOTE.
	assert.Len(t, valueClaim.Sub.String, 3)

	// Sub-claim IDs should start from 0 within the sub-claim context.
	// Sub-claims inherit parent path ["test", "complex", "SHARED", "7"] and add their property.
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "SHARED", "0"), valueClaim.Sub.String[0].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "SHARED", "1"), valueClaim.Sub.String[1].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "NOTE", "0"), valueClaim.Sub.String[2].ID)

	// Verify sub-claims have correct property IDs.
	assert.Equal(t, sharedPropID, valueClaim.Sub.String[0].Prop.ID)
	assert.Equal(t, sharedPropID, valueClaim.Sub.String[1].Prop.ID)
	assert.Equal(t, mnemonics["NOTE"], valueClaim.Sub.String[2].Prop.ID)
}

type DocWithInvalidUnknown struct {
	ID         []string `                  documentid:""`
	InvalidUnk string   `default:"unknown"               property:"INVALID"`
}

type UnsupportedFieldDoc struct {
	ID          []string `documentid:""`
	Unsupported chan int `              property:"UNSUPPORTED"`
}

func TestDocuments_ErrorCases(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NAME":        identifier.New(),
		"INVALID":     identifier.New(),
		"UNSUPPORTED": identifier.New(),
		"NESTED":      identifier.New(),
	}

	t.Run("UnknownTagWithNonBool", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithInvalidUnknown{
				ID:         []string{"doc1"},
				InvalidUnk: "not a bool",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		// Unknown tag on non-bool field without cardinality (defaults to min=0) causes error.
		assert.EqualError(t, errE, "field cannot have default tag with min cardinality 0")
		assert.Nil(t, result)
	})

	t.Run("UnsupportedFieldType", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&UnsupportedFieldDoc{
				ID:          []string{"doc1"},
				Unsupported: make(chan int),
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "field has unsupported or unexpected value type")
		assert.Nil(t, result)
	})
}

type DocWithEmptySliceValues struct {
	ID      []string    `documentid:""`
	Refs    []core.Ref  `              property:"REFS"`
	Strings []string    `              property:"STRINGS"`
	Times   []core.Time `              property:"TIMES"`
}

type DocWithEmbeddedStructError struct {
	Embedded

	ID []string `documentid:""`
}

type Embedded struct {
	Invalid chan int `property:"INVALID"`
}

type DocWithNestedStructNoValue struct {
	ID     []string            `documentid:""`
	Nested NestedStructNoValue `              property:"NESTED"`
}

type NestedStructNoValue struct {
	// No value field, only sub-claims.
	SubField string `property:"SUB"`
}

type DocWithNestedStructEmptySubClaims struct {
	ID     []string          `documentid:""`
	Nested NestedStructEmpty `              property:"NESTED"`
}

type NestedStructEmpty struct {
	// Intentionally empty - no fields.
}

func TestDocuments_EdgeCases(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"REFS":    identifier.New(),
		"STRINGS": identifier.New(),
		"TIMES":   identifier.New(),
		"INVALID": identifier.New(),
		"NESTED":  identifier.New(),
		"SUB":     identifier.New(),
	}

	t.Run("SliceWithEmptyValues", func(t *testing.T) {
		t.Parallel()

		// Create slice with empty values that should be skipped.
		docs := []any{
			&DocWithEmptySliceValues{
				ID: []string{"doc1"},
				Refs: []core.Ref{
					{ID: []string{}},        // Empty - should be skipped.
					{ID: []string{"valid"}}, // Valid.
				},
				Strings: []string{
					"",      // Empty - should be skipped.
					"valid", // Valid.
				},
				Times: []core.Time{
					{}, // Zero time - should be skipped.
					{Time: time.Now(), Precision: document.TimePrecisionSecond}, // Valid.
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should have claims only for the valid values.
		refsProperty := mnemonics["REFS"]
		refsClaims := result[0].Get(refsProperty)
		assert.Len(t, refsClaims, 1) // Only one valid ref.

		stringsProperty := mnemonics["STRINGS"]
		stringsClaims := result[0].Get(stringsProperty)
		assert.Len(t, stringsClaims, 1) // Only one valid string.

		timesProperty := mnemonics["TIMES"]
		timesClaims := result[0].Get(timesProperty)
		assert.Len(t, timesClaims, 1) // Only one valid time.
	})

	t.Run("EmbeddedStructWithError", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithEmbeddedStructError{
				ID: []string{"doc1"},
				Embedded: Embedded{
					Invalid: make(chan int),
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "field has unsupported or unexpected value type")
		assert.Nil(t, result)
	})

	t.Run("NestedStructNoValueWithSubClaims", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNestedStructNoValue{
				ID: []string{"doc1"},
				Nested: NestedStructNoValue{
					SubField: "sub-value",
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should create a HasClaim since there's no value field but there are sub-claims.
		nestedProperty := mnemonics["NESTED"]
		nestedClaims := result[0].Get(nestedProperty)
		assert.Len(t, nestedClaims, 1)
		assert.IsType(t, &document.HasClaim{}, nestedClaims[0])
	})

	t.Run("NestedStructWithEmptySubClaims", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNestedStructEmptySubClaims{
				ID:     []string{"doc1"},
				Nested: NestedStructEmpty{},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should not create a claim since there's no value field and no sub-claims.
		nestedProperty := mnemonics["NESTED"]
		nestedClaims := result[0].Get(nestedProperty)
		assert.Empty(t, nestedClaims)
	})
}

type DocWithConflictingTags struct {
	ID            []string        `documentid:""`
	IdentifierStr core.Identifier `              property:"ID_STR"   type:"html"` // Conflicting type tag.
	IRIStr        core.IRI        `              property:"IRI_STR"  type:"id"`   // Conflicting type tag.
	HTMLStr       core.HTML       `              property:"HTML_STR" type:"id"`   // Conflicting type tag.
}

func TestDocuments_ConflictingTags(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"ID_STR":   identifier.New(),
		"IRI_STR":  identifier.New(),
		"HTML_STR": identifier.New(),
	}

	t.Run("IdentifierWithConflictingTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithConflictingTags{ //nolint:exhaustruct
				ID:            []string{"doc1"},
				IdentifierStr: "test-id",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "identifier field used with conflicting tag")
		assert.Nil(t, result)
	})

	t.Run("IRIWithConflictingTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithConflictingTags{ //nolint:exhaustruct
				ID:     []string{"doc1"},
				IRIStr: "https://example.com",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "IRI field used with conflicting tag")
		assert.Nil(t, result)
	})

	t.Run("HTMLWithConflictingTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithConflictingTags{ //nolint:exhaustruct
				ID:      []string{"doc1"},
				HTMLStr: "test",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "HTML field used with conflicting tag")
		assert.Nil(t, result)
	})
}

type DocWithNumeric struct {
	ID    []string `documentid:""`
	Count int      `              precision:"1" property:"COUNT"`
}

type DocWithInvalidDocID struct {
	ID string `documentid:""` // Should be []string, not string.
}

type DocWithInvalidFloatValue struct {
	ID    []string `documentid:""`
	Value float64  `              precision:"1" property:"VALUE"`
}

func TestDocuments_MoreEdgeCases(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"COUNT": identifier.New(),
		"VALUE": identifier.New(),
	}

	t.Run("NumericField", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNumeric{
				ID:    []string{"doc1"},
				Count: 42,
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)
	})

	t.Run("DocumentIDNotStringSlice", func(t *testing.T) {
		t.Parallel()

		// This will fail during transformation because ID should be []string.
		docs := []any{
			&DocWithInvalidDocID{
				ID: "not-a-slice",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "document ID field is not a string slice")
		assert.Nil(t, result)
	})

	t.Run("FloatInfinityValue", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithInvalidFloatValue{
				ID:    []string{"doc1"},
				Value: math.Inf(1),
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "value is infinity or not a number")
		assert.Nil(t, result)
	})

	t.Run("FloatNaNValue", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithInvalidFloatValue{
				ID:    []string{"doc1"},
				Value: math.NaN(),
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "value is infinity or not a number")
		assert.Nil(t, result)
	})
}

type DocWithPropertyConflict struct {
	ID     []string               `documentid:""`
	Nested PropertyConflictStruct `              property:"NESTED"`
}

type PropertyConflictStruct struct {
	ValueField string `property:"FOO" value:""`
}

func TestDocuments_ValueTagConflicts(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NESTED": identifier.New(),
		"FOO":    identifier.New(),
	}

	t.Run("ValueTagWithPropertyTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithPropertyConflict{
				ID: []string{"doc1"},
				Nested: PropertyConflictStruct{
					ValueField: "test",
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "property tag cannot be used with value tag")
		assert.Nil(t, result)
	})
}

type DocWithIntervalVariants struct {
	ID       []string                 `documentid:""`
	Interval core.Interval[core.Time] `              property:"INTERVAL"`
}

type DocWithRawHTMLConflict struct {
	ID      []string     `documentid:""`
	RawHTML core.RawHTML `              property:"RAW" type:"id"` // Conflicting tag.
}

func TestDocuments_RawHTMLConflict(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"RAW": identifier.New(),
	}

	t.Run("RawHTMLWithConflictingTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithRawHTMLConflict{
				ID:      []string{"doc1"},
				RawHTML: "<p>test</p>",
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		assert.EqualError(t, errE, "raw HTML field used with conflicting tag")
		assert.Nil(t, result)
	})
}

func TestDocuments_IntervalEdgeCases(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"INTERVAL": identifier.New(),
	}

	t.Run("IntervalWithDifferentPrecisions", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		end := start.Add(24 * time.Hour)

		docs := []any{
			&DocWithIntervalVariants{
				ID: []string{"doc1"},
				Interval: core.Interval[core.Time]{ //nolint:exhaustruct
					From: &core.Time{
						Time:      start,
						Precision: document.TimePrecisionSecond,
					},
					To: &core.Time{
						Time:      end,
						Precision: document.TimePrecisionDay, // Different precision - should use higher.
					},
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		intervalProperty := mnemonics["INTERVAL"]
		intervalClaims := result[0].Get(intervalProperty)
		require.Len(t, intervalClaims, 1)

		timeRangeClaim, ok := intervalClaims[0].(*document.TimeIntervalClaim)
		require.True(t, ok)
		// Verify that precision is set on both bounds.
		require.NotNil(t, timeRangeClaim.FromPrecision)
		assert.Positive(t, int(*timeRangeClaim.FromPrecision))
	})

	t.Run("IntervalWithNilFrom", func(t *testing.T) {
		t.Parallel()

		end := time.Now()

		docs := []any{
			&DocWithIntervalVariants{
				ID: []string{"doc1"},
				Interval: core.Interval[core.Time]{ //nolint:exhaustruct
					From: nil,
					To: &core.Time{
						Time:      end,
						Precision: document.TimePrecisionDay,
					},
				},
			},
		}

		_, errE := transform.Documents(t.Context(), mnemonics, docs)
		// Nil From without any flag is an invalid interval bound.
		assert.EqualError(t, errE, `interval's "from" bound is not set`)
	})

	t.Run("IntervalWithUnknownBounds", func(t *testing.T) {
		t.Parallel()

		end := time.Now()

		docs := []any{
			&DocWithIntervalVariants{
				ID: []string{"doc1"},
				Interval: core.Interval[core.Time]{ //nolint:exhaustruct
					From:          nil, // From is unknown (no concrete value).
					FromIsUnknown: true,
					To: &core.Time{
						Time:      end,
						Precision: document.TimePrecisionDay,
					},
				},
			},
		}

		result, errE := transform.Documents(t.Context(), mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// FromIsUnknown creates a TimeIntervalClaim with unknown From bound.
		intervalProperty := mnemonics["INTERVAL"]
		intervalClaims := result[0].Get(intervalProperty)
		require.Len(t, intervalClaims, 1)
		timeRange, ok := intervalClaims[0].(*document.TimeIntervalClaim)
		require.True(t, ok)
		assert.True(t, timeRange.FromIsUnknown)
		assert.Nil(t, timeRange.From)
		assert.NotNil(t, timeRange.To)
	})
}

type DocFlatFields struct {
	ID          []string `documentid:""`
	Name        string   `                            property:"NAME"`
	Description string   `                            property:"DESCRIPTION" type:"html"`
	Age         int      `              precision:"1" property:"AGE"`
	IsActive    bool     `                            property:"IS_ACTIVE"`
}

type CommonFields struct {
	Name        string `              property:"NAME"`
	Description string `              property:"DESCRIPTION" type:"html"`
	Age         int    `precision:"1" property:"AGE"`
	IsActive    bool   `              property:"IS_ACTIVE"`
}

type DocEmbeddedEquivalent struct {
	CommonFields

	ID []string `documentid:""`
}

func TestDocuments_EmbeddedStructEquivalence(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"NAME":        identifier.New(),
		"DESCRIPTION": identifier.New(),
		"AGE":         identifier.New(),
		"IS_ACTIVE":   identifier.New(),
	}

	flatDoc := &DocFlatFields{
		ID:          []string{"test", "doc"},
		Name:        "Test Document",
		Description: "A test <b>description</b>",
		Age:         42,
		IsActive:    true,
	}

	embeddedDoc := &DocEmbeddedEquivalent{
		ID: []string{"test", "doc"},
		CommonFields: CommonFields{
			Name:        "Test Document",
			Description: "A test <b>description</b>",
			Age:         42,
			IsActive:    true,
		},
	}

	flatResult, errE := transform.Documents(t.Context(), mnemonics, []any{flatDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, flatResult, 1)

	embeddedResult, errE := transform.Documents(t.Context(), mnemonics, []any{embeddedDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, embeddedResult, 1)

	assert.Equal(t, flatResult[0], embeddedResult[0])
}

type InnerEmbeddedFields struct {
	Field1 string `property:"FIELD1"`
}

type MiddleEmbeddedFields struct {
	InnerEmbeddedFields

	Field2 string `property:"FIELD2"`
}

type DocMultiLevelEmbedded struct {
	ID                   []string `documentid:""`
	Field0               string   `              property:"FIELD0"`
	MiddleEmbeddedFields          //nolint:embeddedstructfieldcheck
	Field3               string   `property:"FIELD3"`
}

type DocMultiLevelFlat struct {
	ID     []string `documentid:""`
	Field0 string   `              property:"FIELD0"`
	Field1 string   `              property:"FIELD1"`
	Field2 string   `              property:"FIELD2"`
	Field3 string   `              property:"FIELD3"`
}

func TestDocuments_MultiLevelEmbeddedEquivalence(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"FIELD0": identifier.New(),
		"FIELD1": identifier.New(),
		"FIELD2": identifier.New(),
		"FIELD3": identifier.New(),
	}

	nestedDoc := &DocMultiLevelEmbedded{
		ID:     []string{"nested", "doc"},
		Field0: "value0",
		MiddleEmbeddedFields: MiddleEmbeddedFields{
			InnerEmbeddedFields: InnerEmbeddedFields{
				Field1: "value1",
			},
			Field2: "value2",
		},
		Field3: "value3",
	}

	flatDoc := &DocMultiLevelFlat{
		ID:     []string{"nested", "doc"},
		Field0: "value0",
		Field1: "value1",
		Field2: "value2",
		Field3: "value3",
	}

	nestedResult, errE := transform.Documents(t.Context(), mnemonics, []any{nestedDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, nestedResult, 1)

	flatResult, errE := transform.Documents(t.Context(), mnemonics, []any{flatDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, flatResult, 1)

	assert.Equal(t, nestedResult[0], flatResult[0])
}

func TestDocuments_CardinalityZeroOrOne(t *testing.T) {
	t.Parallel()

	// Test cardinality:"0..1" - zero or one value allowed (must use pointer).
	type DocWithOptionalSingle struct {
		ID    []string `                   documentid:""`
		Title *string  `cardinality:"0..1"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Test with nil value - should succeed, no claim.
	docs := []any{
		&DocWithOptionalSingle{
			ID:    []string{"test", "doc1"},
			Title: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Empty(t, doc.Claims.String)
	require.Empty(t, doc.Claims.None)

	// Test with one value - should succeed.
	hello := "Hello"
	docs = []any{
		&DocWithOptionalSingle{
			ID:    []string{"test", "doc2"},
			Title: &hello,
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "Hello", doc.Claims.String[0].String)
}

func TestDocuments_CardinalityOneOrMore(t *testing.T) {
	t.Parallel()

	// Test cardinality:"1.." - one or more values required.
	type DocWithOneOrMore struct {
		ID     []string `                                 documentid:""`
		Titles []string `cardinality:"1.." default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Test with empty slice - should add one NoneClaim (due to default:"none").
	docs := []any{
		&DocWithOneOrMore{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[0].ID)

	// Test with one value - should succeed.
	docs = []any{
		&DocWithOneOrMore{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)

	// Test with multiple values - should succeed.
	docs = []any{
		&DocWithOneOrMore{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2", "Title 3"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 3)
}

func TestDocuments_CardinalityExactCount(t *testing.T) {
	t.Parallel()

	// Test cardinality:"2" - exactly 2 values required.
	type DocWithExactTwo struct {
		ID     []string `                               documentid:""`
		Titles []string `cardinality:"2" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Test with 0 values - should add 2 NoneClaims (due to default:"none").
	docs := []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.None[1].ID)

	// Test with 1 value - should add 1 NoneClaim.
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.None, 1)

	// Test with exactly 2 values - should succeed.
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 2)
	require.Empty(t, doc.Claims.None)

	// Test with 3 values - should fail (exceeds max).
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc4"},
			Titles: []string{"Title 1", "Title 2", "Title 3"},
		},
	}

	_, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value exceeds maximum cardinality")
}

func TestDocuments_CardinalityRange(t *testing.T) {
	t.Parallel()

	// Test cardinality:"2..4" - between 2 and 4 values.
	type DocWithRange struct {
		ID     []string `                                  documentid:""`
		Titles []string `cardinality:"2..4" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Test with 0 values - should add 2 NoneClaims.
	docs := []any{
		&DocWithRange{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 2)

	// Test with 1 value - should add 1 NoneClaim.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.None, 1)

	// Test with 2 values - should succeed.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 2)
	require.Empty(t, doc.Claims.None)

	// Test with 4 values - should succeed.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc4"},
			Titles: []string{"Title 1", "Title 2", "Title 3", "Title 4"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 4)

	// Test with 5 values - should fail (exceeds max).
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc5"},
			Titles: []string{"Title 1", "Title 2", "Title 3", "Title 4", "Title 5"},
		},
	}

	_, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value exceeds maximum cardinality")
}

func TestDocuments_NoneTag_WithCardinality(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test that with default:"none", missing required values are filled with NoneClaims.
	type DocWithNone struct {
		ID     []string `                               documentid:""`
		Titles []string `cardinality:"2" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 0 values, should add 2 NoneClaims.
	docs := []any{
		&DocWithNone{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.None[1].ID)

	// With 1 value, should add 1 NoneClaim.
	docs = []any{
		&DocWithNone{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc2", "TITLE", "1"), doc.Claims.None[0].ID)
}

func TestDocuments_NoneTag_WithoutNone(t *testing.T) {
	t.Parallel()

	// Test that without default:"none", missing required values cause an error.
	type DocWithoutNone struct {
		ID     []string `                documentid:""`
		Titles []string `cardinality:"2"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 0 values, should return error.
	docs := []any{
		&DocWithoutNone{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")

	// With 1 value, should return error.
	docs = []any{
		&DocWithoutNone{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	_, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")

	// With 2 values, should succeed.
	docs = []any{
		&DocWithoutNone{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 2)
}

func TestDocuments_NoneTag_Pointer(t *testing.T) {
	t.Parallel()

	// Test default:"none" with pointer field.
	type DocWithNonePointer struct {
		ID    []string  `                               documentid:""`
		Value *core.Ref `cardinality:"1" default:"none"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// With nil pointer, should add 1 NoneClaim.
	docs := []any{
		&DocWithNonePointer{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "VALUE", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_CardinalityValidation_MaxCannotBeZero(t *testing.T) {
	t.Parallel()

	// Test that max cardinality cannot be 0.
	type DocWithMaxZero struct {
		ID     []string `                   documentid:""`
		Titles []string `cardinality:"0..0"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithMaxZero{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality max value cannot be negative or zero")
}

func TestDocuments_CardinalityValidation_PointerMaxOne(t *testing.T) {
	t.Parallel()

	// Test that pointer fields cannot have max cardinality > 1.
	type DocWithPointerMaxTwo struct {
		ID    []string  `                   documentid:""`
		Value *core.Ref `cardinality:"1..2"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	docs := []any{
		&DocWithPointerMaxTwo{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "pointer field cannot have max cardinality greater than 1")
}

func TestDocuments_CardinalityValidation_SingleValueMaxOne(t *testing.T) {
	t.Parallel()

	// Test that non-pointer, non-slice fields cannot have max cardinality > 1.
	type DocWithSingleValueBad struct {
		ID    []string `                   documentid:""`
		Title string   `cardinality:"1..2"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithSingleValueBad{
			ID:    []string{"test", "doc1"},
			Title: "Hello",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "single value field cannot have max cardinality greater than 1")
}

func TestDocuments_CardinalityValidation_NoneWithMinZero(t *testing.T) {
	t.Parallel()

	// Test that default:"none" tag cannot be used with min cardinality 0.
	type DocWithNoneAndMinZero struct {
		ID    []string `                                  documentid:""`
		Title string   `cardinality:"0..1" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNoneAndMinZero{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field cannot have default tag with min cardinality 0")
}

func TestDocuments_CardinalityValidation_PointerCanBeZeroOrOne(t *testing.T) {
	t.Parallel()

	// Test that pointer fields can have cardinality "0..1" or "1".
	type DocWithPointerZeroOrOne struct {
		ID    []string  `                   documentid:""`
		Value *core.Ref `cardinality:"0..1"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// With nil pointer.
	docs := []any{
		&DocWithPointerZeroOrOne{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	// With non-nil pointer.
	docs = []any{
		&DocWithPointerZeroOrOne{
			ID:    []string{"test", "doc2"},
			Value: &core.Ref{ID: []string{"test", "ref1"}},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Reference, 1)
}

func TestDocuments_CardinalityValidation_PointerWithNone(t *testing.T) {
	t.Parallel()

	// Test that pointer fields can have default:"none" tag.
	type DocWithPointerAndNone struct {
		ID    []string  `                               documentid:""`
		Value *core.Ref `cardinality:"1" default:"none"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// With nil pointer and default:"none" tag, should add NoneClaim.
	docs := []any{
		&DocWithPointerAndNone{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
}

func TestDocuments_CardinalityValidation_InvalidFormat(t *testing.T) {
	t.Parallel()

	// Test invalid cardinality format with multiple "..".
	type DocWithInvalidCardinality struct {
		ID     []string `                      documentid:""`
		Titles []string `cardinality:"1..2..3"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithInvalidCardinality{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "invalid cardinality format")
}

func TestDocuments_CardinalityValidation_EmptyMin(t *testing.T) {
	t.Parallel()

	// Test cardinality with empty min value.
	type DocWithEmptyMin struct {
		ID     []string `                  documentid:""`
		Titles []string `cardinality:"..5"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithEmptyMin{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality min value is empty")
}

func TestDocuments_CardinalityValidation_InvalidMinInteger(t *testing.T) {
	t.Parallel()

	// Test cardinality with non-integer min.
	type DocWithInvalidMin struct {
		ID     []string `                     documentid:""`
		Titles []string `cardinality:"abc..5"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithInvalidMin{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality min value is not a valid integer")
}

func TestDocuments_CardinalityValidation_NegativeMin(t *testing.T) {
	t.Parallel()

	// Test cardinality with negative min.
	type DocWithNegativeMin struct {
		ID     []string `                    documentid:""`
		Titles []string `cardinality:"-1..5"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNegativeMin{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality min value cannot be negative")
}

func TestDocuments_CardinalityValidation_InvalidMaxInteger(t *testing.T) {
	t.Parallel()

	// Test cardinality with non-integer max.
	type DocWithInvalidMax struct {
		ID     []string `                     documentid:""`
		Titles []string `cardinality:"1..xyz"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithInvalidMax{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality max value is not a valid integer")
}

func TestDocuments_CardinalityValidation_NegativeMax(t *testing.T) {
	t.Parallel()

	// Test cardinality with negative max.
	type DocWithNegativeMax struct {
		ID     []string `                    documentid:""`
		Titles []string `cardinality:"1..-5"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNegativeMax{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality max value cannot be negative or zero")
}

func TestDocuments_CardinalityValidation_MaxLessThanMin(t *testing.T) {
	t.Parallel()

	// Test cardinality where max < min.
	type DocWithMaxLessThanMin struct {
		ID     []string `                   documentid:""`
		Titles []string `cardinality:"5..2"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithMaxLessThanMin{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality max value cannot be less than min")
}

func TestDocuments_CardinalityValidation_InvalidSingleValue(t *testing.T) {
	t.Parallel()

	// Test cardinality with non-integer single value.
	type DocWithInvalidSingle struct {
		ID     []string `                  documentid:""`
		Titles []string `cardinality:"abc"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithInvalidSingle{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality value is not a valid integer")
}

func TestDocuments_CardinalityValidation_NegativeSingleValue(t *testing.T) {
	t.Parallel()

	// Test cardinality with negative single value.
	type DocWithNegativeSingle struct {
		ID     []string `                 documentid:""`
		Titles []string `cardinality:"-1"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNegativeSingle{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality value cannot be negative or zero")
}

func TestDocuments_CardinalityValidation_SliceExceedsMax(t *testing.T) {
	t.Parallel()

	// Test that slice with too many values fails.
	type DocWithMaxTwo struct {
		ID     []string `                   documentid:""`
		Titles []string `cardinality:"0..2"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithMaxTwo{
			ID:     []string{"test", "doc1"},
			Titles: []string{"One", "Two", "Three"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value exceeds maximum cardinality")
}

func TestDocuments_CardinalityValidation_SingleValueWithCardinality(t *testing.T) {
	t.Parallel()

	// Test that single value fields can have cardinality "1".
	type DocWithSingleValueOne struct {
		ID    []string `                documentid:""`
		Title string   `cardinality:"1"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithSingleValueOne{
			ID:    []string{"test", "doc1"},
			Title: "Hello",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "Hello", doc.Claims.String[0].String)
}

func TestDocuments_CardinalityValidation_SingleValueZeroOrOne(t *testing.T) {
	t.Parallel()

	// Test that single value fields can have cardinality "0..1".
	type DocWithSingleValueOptional struct {
		ID    []string `                   documentid:""`
		Title string   `cardinality:"0..1"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Empty value should produce no claim.
	docs := []any{
		&DocWithSingleValueOptional{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Empty(t, doc.Claims.String)

	// Non-empty value should produce a claim.
	docs = []any{
		&DocWithSingleValueOptional{
			ID:    []string{"test", "doc2"},
			Title: "Hello",
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "Hello", doc.Claims.String[0].String)
}

func TestDocuments_ValueTag_WithPropertyTag(t *testing.T) {
	t.Parallel()

	// Test that value tag cannot be combined with property tag.
	type InvalidValue struct {
		Value string `property:"PROP" value:""`
	}

	type DocWithInvalidValue struct {
		ID    []string     `documentid:""`
		Field InvalidValue `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithInvalidValue{
			ID:    []string{"test", "doc1"},
			Field: InvalidValue{Value: "test"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "property tag cannot be used with value tag")
}

func TestDocuments_MultipleValueClaims(t *testing.T) {
	t.Parallel()

	// Test that multiple value fields in a struct cause an error.
	type MultipleValues struct {
		Value1 string `value:""`
		Value2 string `value:""`
	}

	type DocWithMultipleValues struct {
		ID    []string       `documentid:""`
		Field MultipleValues `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithMultipleValues{
			ID:    []string{"test", "doc1"},
			Field: MultipleValues{Value1: "test1", Value2: "test2"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "multiple value claims found")
}

func TestDocuments_PointerWithCardinality(t *testing.T) {
	t.Parallel()

	// Test that pointer field respects cardinality "1".
	type DocWithPointerOne struct {
		ID    []string `                documentid:""`
		Value *string  `cardinality:"1"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	value := "test"
	docs := []any{
		&DocWithPointerOne{
			ID:    []string{"test", "doc1"},
			Value: &value,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "test", doc.Claims.String[0].String)
}

func TestDocuments_EmptyStructAsValue(t *testing.T) {
	t.Parallel()

	// Test struct with no value field and no sub-claims produces no claim.
	type EmptyStruct struct {
		// No fields.
	}

	type DocWithEmptyStruct struct {
		ID    []string    `documentid:""`
		Field EmptyStruct `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithEmptyStruct{
			ID:    []string{"test", "doc1"},
			Field: EmptyStruct{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Empty struct with no value field and no sub-claims produces no claim.
	fieldProperty := mnemonics["FIELD"]
	fieldClaims := doc.Get(fieldProperty)
	assert.Empty(t, fieldClaims)
}

func TestDocuments_EmbeddedStructWithValueClaim(t *testing.T) {
	t.Parallel()

	// Test embedded struct that contains a value field.
	type EmbeddedWithValue struct {
		Value string `               value:""`
		Sub   string `property:"SUB"`
	}

	type OuterStruct struct {
		EmbeddedWithValue
	}

	type DocWithEmbeddedValue struct {
		ID    []string    `documentid:""`
		Field OuterStruct `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")
	mnemonics["SUB"] = identifier.From("test", "SUB")

	docs := []any{
		&DocWithEmbeddedValue{
			ID: []string{"test", "doc1"},
			Field: OuterStruct{
				EmbeddedWithValue: EmbeddedWithValue{
					Value: "test-value",
					Sub:   "sub-value",
				},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "test-value", doc.Claims.String[0].String)

	// Check sub-claim.
	require.NotNil(t, doc.Claims.String[0].Sub)
	require.Len(t, doc.Claims.String[0].Sub.String, 1)
	assert.Equal(t, "sub-value", doc.Claims.String[0].Sub.String[0].String)
}

func TestDocuments_EmbeddedStructWithEmptyValue(t *testing.T) {
	t.Parallel()

	// Test embedded struct with empty value field but has sub-claims.
	type EmbeddedWithEmptyValue struct {
		Value string `               value:""`
		Sub   string `property:"SUB"`
	}

	type OuterStruct struct {
		EmbeddedWithEmptyValue
	}

	type DocWithEmbeddedEmptyValue struct {
		ID    []string    `documentid:""`
		Field OuterStruct `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")
	mnemonics["SUB"] = identifier.From("test", "SUB")

	docs := []any{
		&DocWithEmbeddedEmptyValue{
			ID: []string{"test", "doc1"},
			Field: OuterStruct{
				EmbeddedWithEmptyValue: EmbeddedWithEmptyValue{
					Value: "", // Empty value.
					Sub:   "sub-value",
				},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have HasClaim since value is empty but there are sub-claims.
	fieldProperty := mnemonics["FIELD"]
	fieldClaims := doc.Get(fieldProperty)
	require.Len(t, fieldClaims, 1)
	assert.IsType(t, &document.HasClaim{}, fieldClaims[0])

	// Check sub-claim exists.
	hasClaim, ok := fieldClaims[0].(*document.HasClaim)
	require.True(t, ok)
	require.NotNil(t, hasClaim.Sub)
	require.Len(t, hasClaim.Sub.String, 1)
	assert.Equal(t, "sub-value", hasClaim.Sub.String[0].String)
}

func TestDocuments_SliceWithMinCardinality(t *testing.T) {
	t.Parallel()

	// Test slice that doesn't meet minimum cardinality without default:"none" tag.
	type DocWithMinCardinality struct {
		ID     []string `                  documentid:""`
		Titles []string `cardinality:"3.."               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Only 2 values when min is 3 - should fail.
	docs := []any{
		&DocWithMinCardinality{
			ID:     []string{"test", "doc1"},
			Titles: []string{"One", "Two"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")
}

func TestDocuments_PointerDoesNotMeetMin(t *testing.T) {
	t.Parallel()

	// Test pointer that doesn't meet minimum cardinality without default:"none" tag.
	type DocWithPointerMin struct {
		ID    []string `                documentid:""`
		Value *string  `cardinality:"1"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// Nil pointer when min is 1 - should fail.
	docs := []any{
		&DocWithPointerMin{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")
}

func TestDocuments_SingleValueDoesNotMeetMin(t *testing.T) {
	t.Parallel()

	// Test single value that doesn't meet minimum cardinality without default:"none" tag.
	type DocWithSingleMin struct {
		ID    []string `                documentid:""`
		Title string   `cardinality:"1"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Empty string when min is 1 - should fail.
	docs := []any{
		&DocWithSingleMin{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")
}

func TestDocuments_UnknownTag_WithCardinality(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test that with default:"unknown", missing required values are filled with UnknownClaims.
	type DocWithUnknownCardinality struct {
		ID     []string `                                  documentid:""`
		Titles []string `cardinality:"2" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 0 values, should add 2 UnknownClaims.
	docs := []any{
		&DocWithUnknownCardinality{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.Unknown[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.Unknown[1].ID)

	// With 1 value, should add 1 UnknownClaim.
	docs = []any{
		&DocWithUnknownCardinality{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc2", "TITLE", "1"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_UnknownTag_OneOrMore(t *testing.T) {
	t.Parallel()

	// Test default:"unknown" with cardinality "1..".
	type DocWithUnknownOneOrMore struct {
		ID     []string `                                    documentid:""`
		Titles []string `cardinality:"1.." default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With empty slice, should add one UnknownClaim.
	docs := []any{
		&DocWithUnknownOneOrMore{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_UnknownTag_Pointer(t *testing.T) {
	t.Parallel()

	// Test default:"unknown" with pointer field and cardinality.
	type DocWithUnknownPointer struct {
		ID    []string  `                                  documentid:""`
		Value *core.Ref `cardinality:"1" default:"unknown"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// With nil pointer, should add 1 UnknownClaim.
	docs := []any{
		&DocWithUnknownPointer{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "VALUE", "0"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_UnknownTag_WithMinZero(t *testing.T) {
	t.Parallel()

	// Test that default:"unknown" tag cannot be used with min cardinality 0.
	type DocWithUnknownMinZero struct {
		ID     []string `                                    documentid:""`
		Titles []string `cardinality:"0.." default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithUnknownMinZero{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field cannot have default tag with min cardinality 0")
}

func TestDocuments_UnknownTag_SingleValue(t *testing.T) {
	t.Parallel()

	// Test default:"unknown" with single value field and cardinality "1".
	type DocWithUnknownSingle struct {
		ID    []string `                                  documentid:""`
		Title string   `cardinality:"1" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With empty value, should add 1 UnknownClaim.
	docs := []any{
		&DocWithUnknownSingle{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_UnknownTag_BooleanBehavior(t *testing.T) {
	t.Parallel()

	// Test that type:"unknown" on boolean field still works as before (creates UnknownClaim when true).
	type DocWithUnknownBool struct {
		ID           []string `documentid:""`
		Name         string   `              property:"NAME"`
		AgeIsUnknown bool     `              property:"AGE"  type:"unknown"`
	}

	mnemonics := createMnemonics()
	mnemonics["AGE"] = identifier.From("test", "AGE")

	docs := []any{
		&DocWithUnknownBool{
			ID:           []string{"test", "doc1"},
			Name:         "John",
			AgeIsUnknown: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have UnknownClaim for AGE (from boolean field).
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_NoneTag_BooleanBehavior(t *testing.T) {
	t.Parallel()

	// Test that type:"none" on boolean field creates NoneClaim when true.
	type DocWithNoneBool struct {
		ID               []string `documentid:""`
		Name             string   `              property:"NAME"`
		LastNameIsAbsent bool     `              property:"LAST_NAME" type:"none"`
	}

	mnemonics := createMnemonics()
	mnemonics["LAST_NAME"] = identifier.From("test", "LAST_NAME")

	// When true, should create NoneClaim.
	docs := []any{
		&DocWithNoneBool{
			ID:               []string{"test", "doc1"},
			Name:             "John",
			LastNameIsAbsent: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have NoneClaim for LAST_NAME (from boolean field).
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.None[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithNoneBool{
			ID:               []string{"test", "doc2"},
			Name:             "Jane",
			LastNameIsAbsent: false,
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.None)
}

func TestDocuments_NoneTag_BooleanVsCardinalityMode(t *testing.T) {
	t.Parallel()

	// Test that none works differently on boolean vs non-boolean fields.
	type DocWithBothNoneModes struct {
		ID               []string `                                 documentid:""`
		LastNameIsAbsent bool     `                                               property:"LAST_NAME" type:"none"` // Boolean mode.
		Titles           []string `cardinality:"1.." default:"none"               property:"TITLE"`                 // Cardinality mode.
	}

	mnemonics := createMnemonics()
	mnemonics["LAST_NAME"] = identifier.From("test", "LAST_NAME")

	docs := []any{
		&DocWithBothNoneModes{
			ID:               []string{"test", "doc1"},
			LastNameIsAbsent: true,       // Creates NoneClaim (boolean mode).
			Titles:           []string{}, // Creates NoneClaim (cardinality mode).
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have 2 NoneClaims (one from boolean, one from cardinality).
	require.Len(t, doc.Claims.None, 2)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.None[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.None[1].ID)
}

func TestDocuments_CoreNoneType(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test core.None type creates NoneClaim when true.
	type DocWithCoreNone struct {
		ID               []string  `documentid:""`
		Name             string    `              property:"NAME"`
		LastNameIsAbsent core.None `              property:"LAST_NAME"`
	}

	mnemonics := createMnemonics()
	mnemonics["LAST_NAME"] = identifier.From("test", "LAST_NAME")

	// When true, should create NoneClaim.
	docs := []any{
		&DocWithCoreNone{
			ID:               []string{"test", "doc1"},
			Name:             "John",
			LastNameIsAbsent: core.None(true),
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.None[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithCoreNone{
			ID:               []string{"test", "doc2"},
			Name:             "Jane",
			LastNameIsAbsent: core.None(false),
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.None)
}

func TestDocuments_CoreUnknownType(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test core.Unknown type creates UnknownClaim when true.
	type DocWithCoreUnknown struct {
		ID           []string     `documentid:""`
		Name         string       `              property:"NAME"`
		AgeIsUnknown core.Unknown `              property:"AGE"`
	}

	mnemonics := createMnemonics()
	mnemonics["AGE"] = identifier.From("test", "AGE")

	// When true, should create UnknownClaim.
	docs := []any{
		&DocWithCoreUnknown{
			ID:           []string{"test", "doc1"},
			Name:         "John",
			AgeIsUnknown: core.Unknown(true),
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.Unknown[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithCoreUnknown{
			ID:           []string{"test", "doc2"},
			Name:         "Jane",
			AgeIsUnknown: core.Unknown(false),
		},
	}

	results, errE = transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.Unknown)
}

func TestDocuments_BoolWithTypeNone(t *testing.T) {
	t.Parallel()

	// Test regular bool with type:"none" creates NoneClaim when true.
	type DocWithBoolNone struct {
		ID       []string `documentid:""`
		Name     string   `              property:"NAME"`
		IsAbsent bool     `              property:"NOTE" type:"none"`
	}

	mnemonics := createMnemonics()
	mnemonics["NOTE"] = identifier.From("test", "NOTE")

	// When true, should create NoneClaim.
	docs := []any{
		&DocWithBoolNone{
			ID:       []string{"test", "doc1"},
			Name:     "John",
			IsAbsent: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NOTE", "0"), doc.Claims.None[0].ID)
}

func TestDocuments_BoolWithTypeUnknown(t *testing.T) {
	t.Parallel()

	// Test regular bool with type:"unknown" creates UnknownClaim when true.
	type DocWithBoolUnknown struct {
		ID        []string `documentid:""`
		Name      string   `              property:"NAME"`
		IsUnknown bool     `              property:"AGE"  type:"unknown"`
	}

	mnemonics := createMnemonics()
	mnemonics["AGE"] = identifier.From("test", "AGE")

	// When true, should create UnknownClaim.
	docs := []any{
		&DocWithBoolUnknown{
			ID:        []string{"test", "doc1"},
			Name:      "John",
			IsUnknown: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.Unknown[0].ID)
}

func TestDocuments_CoreNoneWithConflictingTag(t *testing.T) {
	t.Parallel()

	// Test core.None with conflicting type tag.
	type DocWithConflictingNone struct {
		ID       []string  `documentid:""`
		IsAbsent core.None `              property:"FIELD" type:"html"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithConflictingNone{
			ID:       []string{"test", "doc1"},
			IsAbsent: core.None(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "none field used with conflicting tag")
}

func TestDocuments_CoreUnknownWithConflictingTag(t *testing.T) {
	t.Parallel()

	// Test core.Unknown with conflicting type tag.
	type DocWithConflictingUnknown struct {
		ID        []string     `documentid:""`
		IsUnknown core.Unknown `              property:"FIELD" type:"id"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithConflictingUnknown{
			ID:        []string{"test", "doc1"},
			IsUnknown: core.Unknown(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "unknown field used with conflicting tag")
}

func TestDocuments_DefaultTagValidation_BothDefaults(t *testing.T) {
	t.Parallel()

	// Test that we can't have invalid default tag value.
	type DocWithInvalidDefault struct {
		ID     []string `                                    documentid:""`
		Titles []string `cardinality:"1.." default:"invalid"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithInvalidDefault{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	// Should fail upfront validation of the default tag value.
	assert.EqualError(t, errE, `default tag must be "none" or "unknown"`)
}

func TestDocuments_SliceWithDefaultNone(t *testing.T) {
	t.Parallel()

	// Test slice with multiple missing values filled by default:"none".
	type DocWithSliceDefault struct {
		ID     []string `                                  documentid:""`
		Titles []string `cardinality:"3..5" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 1 value, should add 2 NoneClaims.
	docs := []any{
		&DocWithSliceDefault{
			ID:     []string{"test", "doc1"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.None, 2)
}

func TestDocuments_SliceWithDefaultUnknown(t *testing.T) {
	t.Parallel()

	// Test slice with multiple missing values filled by default:"unknown".
	type DocWithSliceDefaultUnknown struct {
		ID     []string `                                     documentid:""`
		Titles []string `cardinality:"3..5" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 1 value, should add 2 UnknownClaims.
	docs := []any{
		&DocWithSliceDefaultUnknown{
			ID:     []string{"test", "doc1"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.Unknown, 2)
}

func TestDocuments_PointerWithDefaultNone(t *testing.T) {
	t.Parallel()

	// Test pointer field with default:"none".
	type DocWithPointerDefault struct {
		ID    []string `                               documentid:""`
		Value *string  `cardinality:"1" default:"none"               property:"VALUE"`
	}

	mnemonics := createMnemonics()
	mnemonics["VALUE"] = identifier.From("test", "VALUE")

	// Nil pointer with min=1 and default:"none" should add NoneClaim.
	docs := []any{
		&DocWithPointerDefault{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
}

func TestDocuments_SingleValueWithDefaultUnknown(t *testing.T) {
	t.Parallel()

	// Test single value field with default:"unknown".
	type DocWithSingleDefault struct {
		ID    []string `                                  documentid:""`
		Title string   `cardinality:"1" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Empty string with min=1 and default:"unknown" should add UnknownClaim.
	docs := []any{
		&DocWithSingleDefault{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Unknown, 1)
}

func TestDocuments_BooleanWithMaxCardinality(t *testing.T) {
	t.Parallel()

	// Test that boolean fields cannot have max cardinality > 1.
	type DocWithBoolMax struct {
		ID   []string `                   documentid:""`
		Flag bool     `cardinality:"0..2"               property:"FLAG"`
	}

	mnemonics := createMnemonics()
	mnemonics["FLAG"] = identifier.From("test", "FLAG")

	docs := []any{
		&DocWithBoolMax{
			ID:   []string{"test", "doc1"},
			Flag: true,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	// Boolean fields are also single value fields, so we get the single value error.
	assert.EqualError(t, errE, "single value field cannot have max cardinality greater than 1")
}

func TestDocuments_ValueFieldCannotHaveCardinality(t *testing.T) {
	t.Parallel()

	// Test that value field cannot have cardinality tag.
	type StructWithCardinalityValue struct {
		Value string `cardinality:"1" value:""`
	}

	type DocWithCardinalityValue struct {
		ID    []string                   `documentid:""`
		Field StructWithCardinalityValue `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithCardinalityValue{
			ID:    []string{"test", "doc1"},
			Field: StructWithCardinalityValue{Value: "test"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality tag cannot be used with value tag")
}

func TestDocuments_ValueFieldDefaultNoneEmptyValueNoSub(t *testing.T) {
	t.Parallel()

	// Test that value:"" default:"none" does not create a NoneClaim when the value is empty and there are no sub-claims.
	type NameValue struct {
		Value string `default:"none" value:""`
	}

	type DocWithDefaultNoneValue struct {
		ID   []string  `documentid:""`
		Name NameValue `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithDefaultNoneValue{
			ID:   []string{"test", "doc1"},
			Name: NameValue{Value: ""},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Empty value with default:"none" but no sub-claims should not create any claim.
	assert.Empty(t, doc.Claims.None)
}

func TestDocuments_ValueFieldDefaultUnknownEmptyValueNoSub(t *testing.T) {
	t.Parallel()

	// Test that value:"" default:"unknown" does not create an UnknownClaim when the value is empty and there are no sub-claims.
	type NameValue struct {
		Value string `default:"unknown" value:""`
	}

	type DocWithDefaultUnknownValue struct {
		ID   []string  `documentid:""`
		Name NameValue `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithDefaultUnknownValue{
			ID:   []string{"test", "doc1"},
			Name: NameValue{Value: ""},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Empty value with default:"unknown" but no sub-claims should not create any claim.
	assert.Empty(t, doc.Claims.Unknown)
}

func TestDocuments_ValueFieldDefaultNoneWithValue(t *testing.T) {
	t.Parallel()

	// Test that value:"" default:"none" creates a normal claim when the value is non-empty.
	type NameValue struct {
		Value string `default:"none" value:""`
	}

	type DocWithDefaultNoneValue struct {
		ID   []string  `documentid:""`
		Name NameValue `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithDefaultNoneValue{
			ID:   []string{"test", "doc1"},
			Name: NameValue{Value: "Alice"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Non-empty value should create a normal StringClaim, not a NoneClaim.
	require.Len(t, doc.Claims.String, 1)
	assert.Empty(t, doc.Claims.None)
	assert.Equal(t, "Alice", doc.Claims.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
}

//nolint:dupl
func TestDocuments_ValueFieldDefaultNoneWithSubClaims(t *testing.T) {
	t.Parallel()

	// Test that value:"" default:"none" creates a NoneClaim with sub-claims when value is empty.
	type NameValue struct {
		Value string `default:"none"                 value:""`
		Note  string `               property:"NOTE"`
	}

	type DocWithDefaultNoneSubValue struct {
		ID   []string  `documentid:""`
		Name NameValue `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithDefaultNoneSubValue{
			ID: []string{"test", "doc1"},
			Name: NameValue{
				Value: "",
				Note:  "Annotation",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Empty value with default:"none" and sub-claims should create a NoneClaim with sub-claims.
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.None[0].ID)

	require.NotNil(t, doc.Claims.None[0].Sub)
	require.Len(t, doc.Claims.None[0].Sub.String, 1)
	assert.Equal(t, "Annotation", doc.Claims.None[0].Sub.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0", "NOTE", "0"), doc.Claims.None[0].Sub.String[0].ID)
}

//nolint:dupl
func TestDocuments_ValueFieldDefaultUnknownWithSubClaims(t *testing.T) {
	t.Parallel()

	// Test that value:"" default:"unknown" creates an UnknownClaim with sub-claims when value is empty.
	type NameValue struct {
		Value string `default:"unknown"                 value:""`
		Note  string `                  property:"NOTE"`
	}

	type DocWithDefaultUnknownSubValue struct {
		ID   []string  `documentid:""`
		Name NameValue `              property:"NAME"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithDefaultUnknownSubValue{
			ID: []string{"test", "doc1"},
			Name: NameValue{
				Value: "",
				Note:  "Source unclear",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Empty value with default:"unknown" and sub-claims should create an UnknownClaim with sub-claims.
	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.Unknown[0].ID)

	require.NotNil(t, doc.Claims.Unknown[0].Sub)
	require.Len(t, doc.Claims.Unknown[0].Sub.String, 1)
	assert.Equal(t, "Source unclear", doc.Claims.Unknown[0].Sub.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0", "NOTE", "0"), doc.Claims.Unknown[0].Sub.String[0].ID)
}

func TestClaimNotMadeError(t *testing.T) {
	t.Parallel()

	// Test that claimNotMadeError.Error() returns the expected message.
	assert.Equal(t, "claim not made", transform.TestingErrClaimNotMade.Error())
}

func TestDocuments_ContextCancelled(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, errE := transform.Documents(ctx, mnemonics, []any{
		&SimpleDoc{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	})

	require.Error(t, errE)
	assert.ErrorIs(t, errE, context.Canceled)
}

func TestDocuments_SliceIdentifierConflict(t *testing.T) {
	t.Parallel()

	// A slice of core.Identifier fields with conflicting type tag causes error propagation through processField.
	type DocWithIdentifierSlice struct {
		ID    []string          `documentid:""`
		Codes []core.Identifier `              property:"CODE" type:"iri"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithIdentifierSlice{
			ID:    []string{"test", "doc1"},
			Codes: []core.Identifier{"ABC"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "identifier field used with conflicting tag")
}

func TestDocuments_PointerIdentifierConflict(t *testing.T) {
	t.Parallel()

	// A pointer to core.Identifier with conflicting type tag causes error propagation through processField.
	type DocWithIdentifierPointer struct {
		ID   []string         `documentid:""`
		Code *core.Identifier `              property:"CODE" type:"iri"`
	}

	mnemonics := createMnemonics()

	val := core.Identifier("ABC123")
	docs := []any{
		&DocWithIdentifierPointer{
			ID:   []string{"test", "doc1"},
			Code: &val,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "identifier field used with conflicting tag")
}

func TestDocuments_SubClaimsProcessingError(t *testing.T) {
	t.Parallel()

	// A nested struct where a sub-claim field succeeds, with numeric sub-claim.
	type NestedWithBadSub struct {
		Value string `                               value:""`
		Count int    `precision:"1" property:"COUNT"`
	}

	type DocWithNestedBadSub struct {
		ID   []string         `documentid:""`
		Data NestedWithBadSub `              property:"DATA"`
	}

	mnemonics := createMnemonics()
	mnemonics["DATA"] = identifier.From("test", "DATA")

	docs := []any{
		&DocWithNestedBadSub{
			ID: []string{"test", "doc1"},
			Data: NestedWithBadSub{
				Value: "test",
				Count: 5,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)
}

func TestDocuments_EmbeddedNoValueInValueSearch(t *testing.T) {
	t.Parallel()

	// When extractValueClaim encounters an anonymous embedded struct with no value field, it continues.
	type EmbeddedNoValue struct {
		Sub string `property:"SUB"`
	}

	type OuterWithValueAndEmbedded struct {
		EmbeddedNoValue

		Value string `value:""`
	}

	type DocWithOuterEmbedded struct {
		ID    []string                  `documentid:""`
		Field OuterWithValueAndEmbedded `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")
	mnemonics["SUB"] = identifier.From("test", "SUB")

	docs := []any{
		&DocWithOuterEmbedded{
			ID: []string{"test", "doc1"},
			Field: OuterWithValueAndEmbedded{
				EmbeddedNoValue: EmbeddedNoValue{Sub: "sub-value"},
				Value:           "main-value",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "main-value", doc.Claims.String[0].String)
}

func TestDocuments_EmbeddedValueClaimError(t *testing.T) {
	t.Parallel()

	// When an embedded struct's value field has a conflicting type tag, the error propagates.
	type EmbeddedWithConflict struct {
		Value core.Identifier `type:"iri" value:""`
	}

	type OuterWithConflict struct {
		EmbeddedWithConflict
	}

	type DocWithConflictEmbedded struct {
		ID   []string          `documentid:""`
		Item OuterWithConflict `              property:"ITEM"`
	}

	mnemonics := createMnemonics()
	mnemonics["ITEM"] = identifier.From("test", "ITEM")

	docs := []any{
		&DocWithConflictEmbedded{
			ID: []string{"test", "doc1"},
			Item: OuterWithConflict{
				EmbeddedWithConflict: EmbeddedWithConflict{
					Value: "test-id",
				},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "identifier field used with conflicting tag")
}

func TestDocuments_CoreIdentifierType(t *testing.T) {
	t.Parallel()

	// A core.Identifier field (non-empty, no conflicting type) creates an IdentifierClaim.
	type DocWithCoreIdentifier struct {
		ID   []string        `documentid:""`
		Code core.Identifier `              property:"CODE"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithCoreIdentifier{
			ID:   []string{"test", "doc1"},
			Code: "Q12345",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Identifier, 1)
	assert.Equal(t, "Q12345", doc.Claims.Identifier[0].Value)
	assert.Equal(t, identifier.From("test", "doc1", "CODE", "0"), doc.Claims.Identifier[0].ID)
}

func TestDocuments_EmptyCoreHTMLField(t *testing.T) {
	t.Parallel()

	// An empty core.HTML field produces no claim (claimNotMadeError).
	type DocWithCoreHTML struct {
		ID      []string  `documentid:""`
		Content core.HTML `              property:"HTML"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithCoreHTML{
			ID:      []string{"test", "doc1"},
			Content: "",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	assert.Equal(t, 0, doc.Claims.Size())
}

func TestDocuments_EmptyCoreRawHTMLField(t *testing.T) {
	t.Parallel()

	// An empty core.RawHTML field produces no claim (claimNotMadeError).
	type DocWithCoreRawHTML struct {
		ID      []string     `documentid:""`
		Content core.RawHTML `              property:"RAW_HTML"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithCoreRawHTML{
			ID:      []string{"test", "doc1"},
			Content: "",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	assert.Equal(t, 0, doc.Claims.Size())
}

func TestDocuments_IntervalToPrecisionHigher(t *testing.T) {
	t.Parallel()

	// When interval.To.Precision > interval.From.Precision, the result uses To's precision.
	type DocWithIntervalPrecision struct {
		ID       []string                 `documentid:""`
		Duration core.Interval[core.Time] `              property:"PERIOD"`
	}

	mnemonics := createMnemonics()

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 12, 31, 12, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithIntervalPrecision{
			ID: []string{"test", "doc1"},
			Duration: core.Interval[core.Time]{ //nolint:exhaustruct
				From: &core.Time{
					Time:      start,
					Precision: document.TimePrecisionDay,
				},
				To: &core.Time{
					Time:      end,
					Precision: document.TimePrecisionSecond,
				},
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	periodProperty := mnemonics["PERIOD"]
	periodClaims := results[0].Get(periodProperty)
	require.Len(t, periodClaims, 1)

	timeRangeClaim, ok := periodClaims[0].(*document.TimeIntervalClaim)
	require.True(t, ok)
	// Each bound retains its own precision.
	require.NotNil(t, timeRangeClaim.ToPrecision)
	assert.Equal(t, document.TimePrecisionSecond, *timeRangeClaim.ToPrecision)
}

func TestDocuments_BooleanWithHighCardinality(t *testing.T) {
	t.Parallel()

	// A boolean field with cardinality allowing more than 1 value is invalid.
	// The single value check fires before the boolean check since bool is also a single value.
	type DocWithBoolHighCard struct {
		ID   []string `                  documentid:""`
		Flag bool     `cardinality:"1.."               property:"HIDDEN"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithBoolHighCard{
			ID:   []string{"test", "doc1"},
			Flag: true,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "single value field cannot have max cardinality greater than 1")
}

func TestExtractDocumentID(t *testing.T) {
	t.Parallel()

	t.Run("BasicStruct", func(t *testing.T) {
		t.Parallel()

		type DocWithID struct {
			ID   []string `documentid:""`
			Name string
		}

		id, errE := transform.ExtractDocumentID(&DocWithID{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, []string{"test", "doc1"}, id)
	})

	t.Run("PointerToStruct", func(t *testing.T) {
		t.Parallel()

		type DocWithID struct {
			ID []string `documentid:""`
		}

		doc := &DocWithID{ID: []string{"a", "b"}}
		id, errE := transform.ExtractDocumentID(doc)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, []string{"a", "b"}, id)
	})

	t.Run("NonStruct", func(t *testing.T) {
		t.Parallel()

		_, errE := transform.ExtractDocumentID("not a struct")
		require.Error(t, errE)
		assert.EqualError(t, errE, "expected struct")
	})

	t.Run("MissingDocumentID", func(t *testing.T) {
		t.Parallel()

		type DocNoID struct {
			Name string
		}

		_, errE := transform.ExtractDocumentID(&DocNoID{Name: "Test"})
		require.Error(t, errE)
		assert.ErrorIs(t, errE, transform.ErrDocumentIDNotFound)
	})

	t.Run("StructValue", func(t *testing.T) {
		t.Parallel()

		type DocWithID struct {
			ID []string `documentid:""`
		}

		// Pass a struct value (not pointer).
		id, errE := transform.ExtractDocumentID(DocWithID{ID: []string{"x"}})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, []string{"x"}, id)
	})
}

func TestDocuments_EmbeddedDocIDError(t *testing.T) {
	t.Parallel()

	// An embedded struct with an invalid documentid field type causes an error to propagate.
	type EmbeddedBadID struct {
		ID int `documentid:""`
	}

	type OuterWithBadEmbeddedID struct {
		EmbeddedBadID
	}

	type DocWrapper struct {
		OuterWithBadEmbeddedID
	}

	_, errE := transform.Documents(t.Context(), map[string]identifier.Identifier{}, []any{
		&DocWrapper{
			OuterWithBadEmbeddedID: OuterWithBadEmbeddedID{
				EmbeddedBadID: EmbeddedBadID{ID: 42},
			},
		},
	})
	require.Error(t, errE)
	assert.EqualError(t, errE, "document ID field is not a string slice")
}

func TestDocuments_NumericPrecision(t *testing.T) {
	t.Parallel()

	// Test that precision tag is set correctly on AmountClaim.
	type DocWithPrecision struct {
		ID       []string `documentid:""`
		Width    float64  `              precision:"0.01" property:"WIDTH"`
		Height   int      `              precision:"1"    property:"HEIGHT"`
		Count    uint     `              precision:"10"   property:"COUNT"`
		SmallVal float32  `              precision:"0.5"  property:"SMALL_VAL"`
	}

	mnemonics := createMnemonics()
	mnemonics["SMALL_VAL"] = identifier.From("test", "SMALL_VAL")

	docs := []any{
		&DocWithPrecision{
			ID:       []string{"test", "doc1"},
			Width:    1.75,
			Height:   200,
			Count:    42,
			SmallVal: 3.14,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Amount, 4)

	assert.Equal(t, document.Amount("1.75"), doc.Claims.Amount[0].Amount)
	assert.Equal(t, 0.01, doc.Claims.Amount[0].Precision) //nolint:testifylint
	assert.Equal(t, document.Amount("200"), doc.Claims.Amount[1].Amount)
	assert.Equal(t, 1.0, doc.Claims.Amount[1].Precision) //nolint:testifylint
	assert.Equal(t, document.Amount("40"), doc.Claims.Amount[2].Amount)
	assert.Equal(t, 10.0, doc.Claims.Amount[2].Precision) //nolint:testifylint
	assert.Equal(t, document.Amount("3.0"), doc.Claims.Amount[3].Amount)
	assert.Equal(t, 0.5, doc.Claims.Amount[3].Precision) //nolint:testifylint
}

func TestDocuments_NumericMissingPrecision(t *testing.T) {
	t.Parallel()

	// Test that missing precision tag on numeric field returns an error.
	type DocNoPrecision struct {
		ID    []string `documentid:""`
		Width float64  `              property:"WIDTH"` // Missing precision tag.
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocNoPrecision{
			ID:    []string{"test", "doc1"},
			Width: 1.5,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is required for numeric fields")
}

func TestDocuments_NumericInvalidPrecision(t *testing.T) {
	t.Parallel()

	// Test that invalid precision tag on numeric field returns an error.
	type DocInvalidPrecision struct {
		ID    []string `documentid:""`
		Width float64  `              precision:"not-a-number" property:"WIDTH"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocInvalidPrecision{
			ID:    []string{"test", "doc1"},
			Width: 1.5,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "invalid precision value for numeric field")
}

func TestDocuments_NumericPrecisionZero(t *testing.T) {
	t.Parallel()

	// Test that precision tag of "0" is invalid.
	type DocZeroPrecision struct {
		ID    []string `documentid:""`
		Count int      `              precision:"0" property:"COUNT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocZeroPrecision{
			ID:    []string{"test", "doc1"},
			Count: 7,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision must be finite positive number")
}

func TestDocuments_NumericPrecisionNegative(t *testing.T) {
	t.Parallel()

	// Test that negative precision tag is invalid.
	type DocNegativePrecision struct {
		ID    []string `documentid:""`
		Count int      `              precision:"-1" property:"COUNT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocNegativePrecision{
			ID:    []string{"test", "doc1"},
			Count: 7,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision must be finite positive number")
}

func TestDocuments_NumericPrecisionOnCoreAmountErrors(t *testing.T) {
	t.Parallel()

	// Test that using precision tag with core.Amount[T] returns an error.
	type DocCoreAmountWithPrecision struct {
		ID    []string             `documentid:""`
		Width core.Amount[float64] `              precision:"0.01" property:"WIDTH"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocCoreAmountWithPrecision{
			ID:    []string{"test", "doc1"},
			Width: core.Amount[float64]{Amount: 1.5}, //nolint:exhaustruct
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Amount[T] fields; precision is part of core.Amount")
}

func TestDocuments_GoTimeClaim(t *testing.T) {
	t.Parallel()

	// Test that time.Time creates a TimeClaim with the given precision.
	type DocWithGoTime struct {
		ID      []string  `documentid:""`
		Created time.Time `              precision:"d" property:"CREATED"`
		Born    time.Time `              precision:"y" property:"BORN"`
	}

	mnemonics := createMnemonics()
	created := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	born := time.Date(1990, 6, 20, 0, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithGoTime{
			ID:      []string{"test", "doc1"},
			Created: created,
			Born:    born,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Time, 2)

	// Day precision truncates to YYYY-MM-DD.
	assert.Equal(t, document.TimePrecisionDay, doc.Claims.Time[0].Precision)
	assert.Equal(t, document.Time("2024-03-15"), doc.Claims.Time[0].Time)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0"), doc.Claims.Time[0].ID)

	// Year precision truncates to YYYY.
	assert.Equal(t, document.TimePrecisionYear, doc.Claims.Time[1].Precision)
	assert.Equal(t, document.Time("1990"), doc.Claims.Time[1].Time)
	assert.Equal(t, identifier.From("test", "doc1", "BORN", "0"), doc.Claims.Time[1].ID)
}

func TestDocuments_GoTimeZeroValue(t *testing.T) {
	t.Parallel()

	// Test that a zero time.Time skips claim creation.
	type DocWithGoTime struct {
		ID      []string  `documentid:""`
		Created time.Time `              precision:"s" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithGoTime{
			ID:      []string{"test", "doc1"},
			Created: time.Time{}, // Zero value - no claim.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	assert.Equal(t, 0, doc.Claims.Size())
}

func TestDocuments_GoTimeMissingPrecision(t *testing.T) {
	t.Parallel()

	// Test that missing precision tag on time.Time returns an error.
	type DocNoPrecisionTime struct {
		ID      []string  `documentid:""`
		Created time.Time `              property:"CREATED"` // Missing precision tag.
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocNoPrecisionTime{
			ID:      []string{"test", "doc1"},
			Created: time.Now(),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is required for time.Time fields")
}

func TestDocuments_GoTimeInvalidPrecision(t *testing.T) {
	t.Parallel()

	// Test that invalid precision tag on time.Time returns an error.
	type DocInvalidPrecisionTime struct {
		ID      []string  `documentid:""`
		Created time.Time `              precision:"invalid" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocInvalidPrecisionTime{
			ID:      []string{"test", "doc1"},
			Created: time.Now(),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "unknown time precision")
}

func TestDocuments_GoTimePrecisionOnCoreTimeErrors(t *testing.T) {
	t.Parallel()

	// Test that using precision tag with core.Time returns an error.
	type DocCoreTimeWithPrecision struct {
		ID      []string  `documentid:""`
		Created core.Time `              precision:"d" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocCoreTimeWithPrecision{
			ID:      []string{"test", "doc1"},
			Created: core.Time{},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Time fields; precision is part of core.Time")
}

func TestDocuments_PrecisionOnStringErrors(t *testing.T) {
	t.Parallel()

	// Test that precision tag on a string field returns an error.
	type DocPrecisionOnString struct {
		ID   []string `documentid:""`
		Name string   `              precision:"1" property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocPrecisionOnString{
			ID:   []string{"test", "doc1"},
			Name: "test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for string fields")
}

func TestDocuments_PrecisionOnBoolErrors(t *testing.T) {
	t.Parallel()

	// Test that precision tag on a bool field returns an error.
	type DocPrecisionOnBool struct {
		ID        []string `documentid:""`
		Published bool     `              precision:"1" property:"PUBLISHED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocPrecisionOnBool{
			ID:        []string{"test", "doc1"},
			Published: true,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for bool fields")
}

func TestDocuments_GoTimeSlice(t *testing.T) {
	t.Parallel()

	// Test that []time.Time works with precision tag.
	type DocWithGoTimeSlice struct {
		ID    []string    `documentid:""`
		Dates []time.Time `              precision:"d" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	d1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithGoTimeSlice{
			ID:    []string{"test", "doc1"},
			Dates: []time.Time{d1, d2},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Time, 2)
	assert.Equal(t, document.Time("2024-01-01"), doc.Claims.Time[0].Time)
	assert.Equal(t, document.Time("2024-06-15"), doc.Claims.Time[1].Time)
}

func TestDocuments_GoTimePointer(t *testing.T) {
	t.Parallel()

	// Test that *time.Time works with precision tag.
	type DocWithGoTimePointer struct {
		ID      []string   `documentid:""`
		Created *time.Time `              precision:"s" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	ts := time.Date(2024, 3, 15, 10, 30, 45, 0, time.UTC)

	docs := []any{
		&DocWithGoTimePointer{
			ID:      []string{"test", "doc1"},
			Created: &ts,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Time, 1)
	assert.Equal(t, document.TimePrecisionSecond, doc.Claims.Time[0].Precision)
	assert.Equal(t, document.Time("2024-03-15 10:30:45"), doc.Claims.Time[0].Time)
}

func TestDocuments_GoTimeValueField(t *testing.T) {
	t.Parallel()

	// Test time.Time as a value field in a nested struct.
	type TimeWithNote struct {
		Value time.Time `precision:"d"                 value:""`
		Note  string    `              property:"NOTE"`
	}

	type DocWithTimeValue struct {
		ID      []string     `documentid:""`
		Created TimeWithNote `              property:"CREATED"`
	}

	mnemonics := createMnemonics()
	mnemonics["CREATED"] = identifier.From("test", "CREATED")
	ts := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

	docs := []any{
		&DocWithTimeValue{
			ID: []string{"test", "doc1"},
			Created: TimeWithNote{
				Value: ts,
				Note:  "birth date",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Time, 1)
	assert.Equal(t, document.TimePrecisionDay, doc.Claims.Time[0].Precision)
	assert.Equal(t, document.Time("2024-03-15"), doc.Claims.Time[0].Time)

	// Should have sub-claim.
	require.NotNil(t, doc.Claims.Time[0].Sub)
	assert.Len(t, doc.Claims.Time[0].Sub.String, 1)
	assert.Equal(t, "birth date", doc.Claims.Time[0].Sub.String[0].String)
}

func TestDocuments_PrecisionOnRefErrors(t *testing.T) {
	t.Parallel()

	// Test that precision tag on a core.Ref field returns an error.
	type DocPrecisionOnRef struct {
		ID     []string `documentid:""`
		Parent core.Ref `              precision:"1" property:"PARENT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocPrecisionOnRef{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"ref", "1"}},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Ref fields")
}

func TestDocuments_LocationOnGoTime(t *testing.T) {
	t.Parallel()

	// 2025-01-15 15:30:45 UTC = 2025-01-15 10:30:45 EST (UTC-5).
	type DocGoTimeLocation struct {
		ID      []string  `documentid:""`
		Created time.Time `              location:"America/New_York" precision:"s" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocGoTimeLocation{
			ID:      []string{"test", "doc1"},
			Created: time.Date(2025, 1, 15, 15, 30, 45, 0, time.UTC),
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results[0].Claims.Time, 1)
	assert.Equal(t, document.TimePrecisionSecond, results[0].Claims.Time[0].Precision)
	assert.Equal(t, document.Time("2025-01-15 10:30:45"), results[0].Claims.Time[0].Time)
}

func TestDocuments_LocationOnCoreTime(t *testing.T) {
	t.Parallel()

	// 2025-03-15 00:30:45 UTC = 2025-03-15 09:30:45 JST (UTC+9).
	type DocCoreTimeLocation struct {
		ID      []string  `documentid:""`
		Created core.Time `              location:"Asia/Tokyo" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocCoreTimeLocation{
			ID: []string{"test", "doc1"},
			Created: core.Time{
				Time:      time.Date(2025, 3, 15, 0, 30, 45, 0, time.UTC),
				Precision: document.TimePrecisionSecond,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results[0].Claims.Time, 1)
	assert.Equal(t, document.TimePrecisionSecond, results[0].Claims.Time[0].Precision)
	assert.Equal(t, document.Time("2025-03-15 09:30:45"), results[0].Claims.Time[0].Time)
}

func TestDocuments_LocationOnTimeInterval(t *testing.T) {
	t.Parallel()

	// 2025-01-15 15:00 UTC = 2025-01-15 10:00 EST (UTC-5).
	// 2025-01-15 20:00 UTC = 2025-01-15 15:00 EST.
	type DocIntervalLocation struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              location:"America/New_York" property:"PERIOD"`
	}

	from := core.Time{
		Time:      time.Date(2025, 1, 15, 15, 0, 0, 0, time.UTC),
		Precision: document.TimePrecisionMinute,
	}
	to := core.Time{
		Time:      time.Date(2025, 1, 15, 20, 0, 0, 0, time.UTC),
		Precision: document.TimePrecisionMinute,
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocIntervalLocation{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{
				From:          &from,
				FromIsOpen:    false,
				FromIsUnknown: false,
				FromIsNone:    false,
				To:            &to,
				ToIsClosed:    false,
				ToIsUnknown:   false,
				ToIsNone:      false,
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results[0].Claims.TimeInterval, 1)
	claim := results[0].Claims.TimeInterval[0]
	require.NotNil(t, claim.From)
	require.NotNil(t, claim.To)
	assert.Equal(t, document.Time("2025-01-15 10:00"), *claim.From)
	assert.Equal(t, document.Time("2025-01-15 15:00"), *claim.To)
}

func TestDocuments_LocationOnGoTimeValueField(t *testing.T) {
	t.Parallel()

	// 2025-01-15 15:30:45 UTC = 2025-01-15 10:30:45 EST.
	type TimeWithNoteAndLocation struct {
		Value time.Time `location:"America/New_York" precision:"s"                 value:""`
		Note  string    `                                          property:"NOTE"`
	}

	type DocGoTimeLocationValue struct {
		ID      []string                `documentid:""`
		Created TimeWithNoteAndLocation `              property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocGoTimeLocationValue{
			ID: []string{"test", "doc1"},
			Created: TimeWithNoteAndLocation{
				Value: time.Date(2025, 1, 15, 15, 30, 45, 0, time.UTC),
				Note:  "birth date",
			},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results[0].Claims.Time, 1)
	assert.Equal(t, document.Time("2025-01-15 10:30:45"), results[0].Claims.Time[0].Time)
}

func TestDocuments_LocationInvalidValue(t *testing.T) {
	t.Parallel()

	type DocInvalidLocation struct {
		ID      []string  `documentid:""`
		Created time.Time `              location:"Invalid/Zone" precision:"s" property:"CREATED"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocInvalidLocation{
			ID:      []string{"test", "doc1"},
			Created: time.Date(2025, 1, 15, 15, 30, 45, 0, time.UTC),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "invalid location")
}

func TestDocuments_LocationOnRefErrors(t *testing.T) {
	t.Parallel()

	type DocLocationOnRef struct {
		ID     []string `documentid:""`
		Parent core.Ref `              location:"UTC" property:"PARENT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocLocationOnRef{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"ref", "1"}},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Ref fields")
}

func TestDocuments_LocationOnStringErrors(t *testing.T) {
	t.Parallel()

	type DocLocationOnString struct {
		ID   []string `documentid:""`
		Name string   `              location:"UTC" property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocLocationOnString{
			ID:   []string{"test", "doc1"},
			Name: "test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for string fields")
}

func TestDocuments_ConfidenceDefault(t *testing.T) {
	t.Parallel()

	// Test that without confidence tag, HighConfidence (1.0) is used.
	mnemonics := createMnemonics()
	docs := []any{
		&SimpleDoc{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.HighConfidence, doc.Claims.String[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceCustom(t *testing.T) {
	t.Parallel()

	// Test that confidence tag sets the claim confidence.
	type DocWithConfidence struct {
		ID   []string `                  documentid:""`
		Name string   `confidence:"0.75"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.Confidence(0.75), doc.Claims.String[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceNegative(t *testing.T) {
	t.Parallel()

	// Test that negative confidence (negation) is supported.
	type DocWithNegConfidence struct {
		ID   []string `                  documentid:""`
		Name string   `confidence:"-0.5"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithNegConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.Confidence(-0.5), doc.Claims.String[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceZero(t *testing.T) {
	t.Parallel()

	// Test that zero confidence is a valid value.
	type DocWithZeroConfidence struct {
		ID   []string `               documentid:""`
		Name string   `confidence:"0"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithZeroConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.NoConfidence, doc.Claims.String[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceNegativeOne(t *testing.T) {
	t.Parallel()

	// Test that -1.0 (high negation confidence) is valid.
	type DocWithNegOneConfidence struct {
		ID   []string `                documentid:""`
		Name string   `confidence:"-1"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithNegOneConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.HighNegationConfidence, doc.Claims.String[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceInvalidFloat(t *testing.T) {
	t.Parallel()

	// Test that non-float confidence tag returns an error.
	type DocWithInvalidConfidence struct {
		ID   []string `                         documentid:""`
		Name string   `confidence:"not-a-float"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithInvalidConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "confidence tag value is not a valid float")
}

func TestDocuments_ConfidenceOutOfRangeHigh(t *testing.T) {
	t.Parallel()

	// Test that confidence > 1 returns an error.
	type DocWithHighConfidence struct {
		ID   []string `                 documentid:""`
		Name string   `confidence:"1.5"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithHighConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "confidence tag value out of range [-1, 1]")
}

func TestDocuments_ConfidenceOutOfRangeLow(t *testing.T) {
	t.Parallel()

	// Test that confidence < -1 returns an error.
	type DocWithLowConfidence struct {
		ID   []string `                  documentid:""`
		Name string   `confidence:"-1.5"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithLowConfidence{
			ID:   []string{"test", "doc1"},
			Name: "Test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "confidence tag value out of range [-1, 1]")
}

func TestDocuments_ConfidenceOnBoolField(t *testing.T) {
	t.Parallel()

	// Test that confidence tag works on bool fields (HasClaim, NoneClaim, UnknownClaim).
	type DocWithBoolConfidence struct {
		ID             []string `                 documentid:""`
		Published      bool     `confidence:"0.5"               property:"PUBLISHED"`
		AgeIsUnknown   bool     `confidence:"0.5"               property:"AGE"       type:"unknown"`
		HeightIsAbsent bool     `confidence:"0.5"               property:"HEIGHT"    type:"none"`
	}

	mnemonics := createMnemonics()
	// Add HEIGHT to mnemonics.
	mnemonics["HEIGHT"] = identifier.From("test", "HEIGHT")

	docs := []any{
		&DocWithBoolConfidence{
			ID:             []string{"test", "doc1"},
			Published:      true,
			AgeIsUnknown:   true,
			HeightIsAbsent: true,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Has, 1)
	assert.Equal(t, document.Confidence(0.5), doc.Claims.Has[0].Confidence) //nolint:testifylint

	require.Len(t, doc.Claims.Unknown, 1)
	assert.Equal(t, document.Confidence(0.5), doc.Claims.Unknown[0].Confidence) //nolint:testifylint

	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, document.Confidence(0.5), doc.Claims.None[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceOnRefField(t *testing.T) {
	t.Parallel()

	// Test that confidence tag works on core.Ref fields (ReferenceClaim).
	type DocWithRefConfidence struct {
		ID     []string `                 documentid:""`
		Parent core.Ref `confidence:"0.6"               property:"PARENT"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithRefConfidence{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"ref", "parent1"}},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Reference, 1)
	assert.Equal(t, document.Confidence(0.6), doc.Claims.Reference[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceOnAmountField(t *testing.T) {
	t.Parallel()

	// Test that confidence tag works on numeric fields (AmountClaim).
	type DocWithAmountConfidence struct {
		ID    []string `                 documentid:""`
		Width float64  `confidence:"0.8"               precision:"0.01" property:"WIDTH"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithAmountConfidence{
			ID:    []string{"test", "doc1"},
			Width: 1.5,
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Amount, 1)
	assert.Equal(t, document.Confidence(0.8), doc.Claims.Amount[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceOnSlice(t *testing.T) {
	t.Parallel()

	// Test that confidence tag applies to all claims from a slice field.
	type DocWithSliceConfidence struct {
		ID    []string `                 documentid:""`
		Codes []string `confidence:"0.7"               property:"CODES" type:"id"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithSliceConfidence{
			ID:    []string{"test", "doc1"},
			Codes: []string{"A", "B", "C"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.Identifier, 3)
	for _, claim := range doc.Claims.Identifier {
		assert.Equal(t, document.Confidence(0.7), claim.Confidence) //nolint:testifylint
	}
}

func TestDocuments_ConfidenceOnDefaultFallback(t *testing.T) {
	t.Parallel()

	// Test that confidence tag applies to fallback none/unknown claims from cardinality default.
	type DocWithDefaultConfidence struct {
		ID   []string `                                                documentid:""`
		Name string   `cardinality:"1" confidence:"0.4" default:"none"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithDefaultConfidence{
			ID:   []string{"test", "doc1"},
			Name: "", // Empty, triggering default.
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.None, 1)
	assert.Equal(t, document.Confidence(0.4), doc.Claims.None[0].Confidence) //nolint:testifylint
}

func TestDocuments_ConfidenceOnNestedStruct(t *testing.T) {
	t.Parallel()

	// Test that confidence on a field with nested struct applies to the value claim.
	type NameWithNote struct {
		Value string `                value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithNestedConfidence struct {
		ID   []string     `                 documentid:""`
		Name NameWithNote `confidence:"0.6"               property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithNestedConfidence{
			ID:   []string{"test", "doc1"},
			Name: NameWithNote{Value: "Alice", Note: "primary name"},
		},
	}

	results, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, document.Confidence(0.6), doc.Claims.String[0].Confidence) //nolint:testifylint
	assert.Equal(t, "Alice", doc.Claims.String[0].String)
}

func TestDocuments_ConfidenceOnValueTagForbidden(t *testing.T) {
	t.Parallel()

	// Test that using confidence tag on value field returns an error.
	type NameWithConfidence struct {
		Value string `confidence:"0.5"                 value:""`
		Note  string `                 property:"NOTE"`
	}

	type DocWithValueConfidence struct {
		ID   []string           `documentid:""`
		Name NameWithConfidence `              property:"NAME"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithValueConfidence{
			ID:   []string{"test", "doc1"},
			Name: NameWithConfidence{Value: "Alice", Note: "primary"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "confidence tag cannot be used with value tag")
}

func TestDocuments_TypeTagOnRefErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string `documentid:""`
		Parent core.Ref `              property:"PARENT" type:"id"`
	}

	docs := []any{
		&Doc{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"parent", "id"}},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for core.Ref fields")
}

func TestDocuments_TypeTagOnGoTimeErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID      []string  `documentid:""`
		Created time.Time `              precision:"s" property:"CREATED" type:"id"`
	}

	docs := []any{
		&Doc{
			ID:      []string{"test", "doc1"},
			Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for time.Time fields")
}

func TestDocuments_TypeTagOnCoreTimeErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID      []string  `documentid:""`
		Created core.Time `              property:"CREATED" type:"id"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Created: core.Time{
				Time:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Precision: document.TimePrecisionDay,
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for core.Time fields")
}

func TestDocuments_PrecisionOnCoreTimeErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID      []string  `documentid:""`
		Created core.Time `              precision:"s" property:"CREATED"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Created: core.Time{
				Time:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Precision: document.TimePrecisionDay,
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Time fields; precision is part of core.Time")
}

func TestDocuments_TypeTagOnTimeIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              property:"PERIOD" type:"id"`
	}

	now := time.Now()
	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From: &core.Time{Time: now, Precision: document.TimePrecisionDay},
				To:   &core.Time{Time: now.Add(time.Hour), Precision: document.TimePrecisionDay},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for core.Interval[core.Time] fields")
}

func TestDocuments_PrecisionOnTimeIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              precision:"s" property:"PERIOD"`
	}

	now := time.Now()
	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From: &core.Time{Time: now, Precision: document.TimePrecisionDay},
				To:   &core.Time{Time: now.Add(time.Hour), Precision: document.TimePrecisionDay},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Interval[core.Time] fields; precision is part of core.Time")
}

func TestDocuments_LocationOnTimeIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              location:"Invalid/Zone" property:"PERIOD"`
	}

	now := time.Now()
	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From: &core.Time{Time: now, Precision: document.TimePrecisionDay},
				To:   &core.Time{Time: now.Add(time.Hour), Precision: document.TimePrecisionDay},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.ErrorContains(t, errE, "invalid location")
}

func TestDocuments_TimeIntervalFromIsNone(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              property:"PERIOD"`
	}

	now := time.Now()
	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				FromIsNone: true,
				To:         &core.Time{Time: now, Precision: document.TimePrecisionDay},
			},
		},
	}

	result, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)

	periodClaims := result[0].Get(mnemonics["PERIOD"])
	require.Len(t, periodClaims, 1)
	timeRange, ok := periodClaims[0].(*document.TimeIntervalClaim)
	require.True(t, ok)
	assert.True(t, timeRange.FromIsNone)
	assert.Nil(t, timeRange.From)
}

func TestDocuments_TimeIntervalToMissing(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string                 `documentid:""`
		Period core.Interval[core.Time] `              property:"PERIOD"`
	}

	now := time.Now()
	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Period: core.Interval[core.Time]{ //nolint:exhaustruct
				From: &core.Time{Time: now, Precision: document.TimePrecisionDay},
				// To is nil with no flags.
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "to" bound is not set`)
}

func TestDocuments_TypeTagOnAmountIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD" type:"id"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From: &core.Amount[int]{Amount: 1, Precision: 1},
				To:   &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for core.Interval[core.Amount[T]] fields")
}

func TestDocuments_PrecisionOnAmountIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              precision:"1" property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From: &core.Amount[int]{Amount: 1, Precision: 1},
				To:   &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Interval[core.Amount[T]] fields; precision is part of core.Amount")
}

func TestDocuments_LocationOnAmountIntervalErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              location:"UTC" property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From: &core.Amount[int]{Amount: 1, Precision: 1},
				To:   &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Interval[core.Amount[T]] fields")
}

func TestDocuments_AmountIntervalEmptySkipped(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{},
		},
	}

	result, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	// Empty interval should be skipped.
	assert.Empty(t, result[0].Get(mnemonics["PERIOD"]))
}

func TestDocuments_AmountIntervalFromMissing(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				// From is nil with no flags.
				To: &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "from" bound is not set`)
}

func TestDocuments_AmountIntervalFromIsNone(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				FromIsNone: true,
				To:         &core.Amount[int]{Amount: 10, Precision: 1},
			},
		},
	}

	result, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)

	periodClaims := result[0].Get(mnemonics["PERIOD"])
	require.Len(t, periodClaims, 1)
	amountRange, ok := periodClaims[0].(*document.AmountIntervalClaim)
	require.True(t, ok)
	assert.True(t, amountRange.FromIsNone)
	assert.Nil(t, amountRange.From)
}

//nolint:dupl
func TestDocuments_AmountIntervalToIsNone(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From:     &core.Amount[int]{Amount: 1, Precision: 1},
				ToIsNone: true,
			},
		},
	}

	result, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)

	periodClaims := result[0].Get(mnemonics["PERIOD"])
	require.Len(t, periodClaims, 1)
	amountRange, ok := periodClaims[0].(*document.AmountIntervalClaim)
	require.True(t, ok)
	assert.True(t, amountRange.ToIsNone)
	assert.Nil(t, amountRange.To)
}

//nolint:dupl
func TestDocuments_AmountIntervalToIsUnknown(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                        `documentid:""`
		Range core.Interval[core.Amount[int]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[int]]{ //nolint:exhaustruct
				From:        &core.Amount[int]{Amount: 1, Precision: 1},
				ToIsUnknown: true,
			},
		},
	}

	result, errE := transform.Documents(t.Context(), mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)

	periodClaims := result[0].Get(mnemonics["PERIOD"])
	require.Len(t, periodClaims, 1)
	amountRange, ok := periodClaims[0].(*document.AmountIntervalClaim)
	require.True(t, ok)
	assert.True(t, amountRange.ToIsUnknown)
	assert.Nil(t, amountRange.To)
}

func TestDocuments_AmountIntervalInfinityFromErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                            `documentid:""`
		Range core.Interval[core.Amount[float64]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[float64]]{ //nolint:exhaustruct
				From: &core.Amount[float64]{Amount: math.Inf(1), Precision: 1},
				To:   &core.Amount[float64]{Amount: 10, Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "from" is infinity or not a number`)
}

func TestDocuments_AmountIntervalInfinityToErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string                            `documentid:""`
		Range core.Interval[core.Amount[float64]] `              property:"PERIOD"`
	}

	docs := []any{
		&Doc{
			ID: []string{"test", "doc1"},
			Range: core.Interval[core.Amount[float64]]{ //nolint:exhaustruct
				From: &core.Amount[float64]{Amount: 1, Precision: 1},
				To:   &core.Amount[float64]{Amount: math.Inf(-1), Precision: 1},
			},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `interval's "to" is infinity or not a number`)
}

func TestDocuments_TypeTagOnCoreAmountErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string         `documentid:""`
		Width core.Amount[int] `              property:"WIDTH" type:"id"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Width: core.Amount[int]{Amount: 10, Precision: 1},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for core.Amount[T] fields")
}

func TestDocuments_LocationOnCoreAmountErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string         `documentid:""`
		Width core.Amount[int] `              location:"UTC" property:"WIDTH"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Width: core.Amount[int]{Amount: 10, Precision: 1},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Amount[T] fields")
}

func TestDocuments_CoreAmountInfinityErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string             `documentid:""`
		Width core.Amount[float64] `              property:"WIDTH"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Width: core.Amount[float64]{Amount: math.Inf(1), Precision: 1},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "value is infinity or not a number")
}

func TestDocuments_PrecisionOnIdentifierErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string        `documentid:""`
		Code core.Identifier `              precision:"1" property:"CODE"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Code: core.Identifier("ABC"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Identifier fields")
}

func TestDocuments_LocationOnIdentifierErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string        `documentid:""`
		Code core.Identifier `              location:"UTC" property:"CODE"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Code: core.Identifier("ABC"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Identifier fields")
}

func TestDocuments_PrecisionOnIRIErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID  []string `documentid:""`
		URL core.IRI `              precision:"1" property:"HOMEPAGE"`
	}

	docs := []any{
		&Doc{
			ID:  []string{"test", "doc1"},
			URL: core.IRI("https://example.com"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.IRI fields")
}

func TestDocuments_LocationOnIRIErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID  []string `documentid:""`
		URL core.IRI `              location:"UTC" property:"HOMEPAGE"`
	}

	docs := []any{
		&Doc{
			ID:  []string{"test", "doc1"},
			URL: core.IRI("https://example.com"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.IRI fields")
}

func TestDocuments_PrecisionOnHTMLErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string  `documentid:""`
		Text core.HTML `              precision:"1" property:"HTML"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Text: core.HTML("<p>hello</p>"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.HTML fields")
}

func TestDocuments_LocationOnHTMLErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string  `documentid:""`
		Text core.HTML `              location:"UTC" property:"HTML"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Text: core.HTML("<p>hello</p>"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.HTML fields")
}

func TestDocuments_PrecisionOnRawHTMLErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string     `documentid:""`
		Text core.RawHTML `              precision:"1" property:"RAW_HTML"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Text: core.RawHTML("<p>hello</p>"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.RawHTML fields")
}

func TestDocuments_LocationOnRawHTMLErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string     `documentid:""`
		Text core.RawHTML `              location:"UTC" property:"RAW_HTML"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Text: core.RawHTML("<p>hello</p>"),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.RawHTML fields")
}

func TestDocuments_PrecisionOnNoneErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID       []string  `documentid:""`
		IsActive core.None `              precision:"1" property:"HIDDEN"`
	}

	docs := []any{
		&Doc{
			ID:       []string{"test", "doc1"},
			IsActive: core.None(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.None fields")
}

func TestDocuments_LocationOnNoneErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID       []string  `documentid:""`
		IsActive core.None `              location:"UTC" property:"HIDDEN"`
	}

	docs := []any{
		&Doc{
			ID:       []string{"test", "doc1"},
			IsActive: core.None(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.None fields")
}

func TestDocuments_PrecisionOnUnknownErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID  []string     `documentid:""`
		Age core.Unknown `              precision:"1" property:"AGE"`
	}

	docs := []any{
		&Doc{
			ID:  []string{"test", "doc1"},
			Age: core.Unknown(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for core.Unknown fields")
}

func TestDocuments_LocationOnUnknownErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID  []string     `documentid:""`
		Age core.Unknown `              location:"UTC" property:"AGE"`
	}

	docs := []any{
		&Doc{
			ID:  []string{"test", "doc1"},
			Age: core.Unknown(true),
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Unknown fields")
}

func TestDocuments_StringUnsupportedTypeTag(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string `documentid:""`
		Name string   `              property:"NAME" type:"unsupported"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Name: "test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "string field used with unsupported type tag")
}

func TestDocuments_BoolUnsupportedTypeTag(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string `documentid:""`
		Active bool     `              property:"HIDDEN" type:"unsupported"`
	}

	docs := []any{
		&Doc{
			ID:     []string{"test", "doc1"},
			Active: true,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "bool field used with unsupported type tag")
}

func TestDocuments_LocationOnStringFieldErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string `documentid:""`
		Name string   `              location:"UTC" property:"NAME"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Name: "test",
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for string fields")
}

func TestDocuments_LocationOnBoolErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string `documentid:""`
		Active bool     `              location:"UTC" property:"HIDDEN"`
	}

	docs := []any{
		&Doc{
			ID:     []string{"test", "doc1"},
			Active: true,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for bool fields")
}

func TestDocuments_LocationOnNumericErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string `documentid:""`
		Width float64  `              location:"UTC" precision:"0.01" property:"WIDTH"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Width: 1.5,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for numeric fields")
}

func TestDocuments_PrecisionOnStructErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Inner struct {
		Value string `value:""`
	}

	type Doc struct {
		ID   []string `documentid:""`
		Data Inner    `              precision:"1" property:"NAME"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Data: Inner{Value: "hello"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "precision tag is not supported for struct field types")
}

func TestDocuments_LocationOnStructErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Inner struct {
		Value string `value:""`
	}

	type Doc struct {
		ID   []string `documentid:""`
		Data Inner    `              location:"UTC" property:"NAME"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Data: Inner{Value: "hello"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for struct field types")
}

func TestDocuments_LocationOnRefFieldErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID     []string `documentid:""`
		Parent core.Ref `              location:"UTC" property:"PARENT"`
	}

	docs := []any{
		&Doc{
			ID:     []string{"test", "doc1"},
			Parent: core.Ref{ID: []string{"parent", "id"}},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "location tag is not supported for core.Ref fields")
}

func TestDocuments_TypeOnNumericErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID    []string `documentid:""`
		Width int      `              precision:"1" property:"WIDTH" type:"id"`
	}

	docs := []any{
		&Doc{
			ID:    []string{"test", "doc1"},
			Width: 10,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, "type tag is not supported for numeric fields")
}

func TestDocuments_InvalidDefaultTagErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Doc struct {
		ID   []string `                                documentid:""`
		Name []string `cardinality:"1.." default:"foo"               property:"NAME"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Name: nil,
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `default tag must be "none" or "unknown"`)
}

func TestDocuments_InvalidDefaultTagOnValueFieldErrors(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()

	type Inner struct {
		Value string `default:"foo" value:""`
	}

	type Doc struct {
		ID   []string `documentid:""`
		Name Inner    `              property:"NAME"`
	}

	docs := []any{
		&Doc{
			ID:   []string{"test", "doc1"},
			Name: Inner{Value: "hello"},
		},
	}

	_, errE := transform.Documents(t.Context(), mnemonics, docs)
	assert.EqualError(t, errE, `default tag must be "none" or "unknown"`)
}
