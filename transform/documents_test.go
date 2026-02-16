package transform_test

import (
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

type DocWithURL struct {
	ID       []string `documentid:""`
	Homepage string   `              property:"HOMEPAGE"  type:"url"`
	Links    []string `              property:"LINKS"     type:"url"`
	PlainURL core.URL `              property:"PLAIN_URL"`
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
	ID     []string      `documentid:""`
	Period core.Interval `              property:"PERIOD"`
}

type DocWithAmount struct {
	ID     []string `documentid:""`
	Width  float64  `              property:"WIDTH"  unit:"m"`
	Height int      `              property:"HEIGHT" unit:"m"`
	Count  uint     `              property:"COUNT"  unit:"1"`
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
	Value  string        `                  value:""`
	Period core.Interval `property:"PERIOD"`
	Note   string        `property:"NOTE"`
}

type DocWithNestedValue struct {
	ID          []string      `documentid:""`
	Title       NestedValue   `              property:"TITLE"`
	Description []NestedValue `              property:"DESCRIPTION"`
}

type NestedWithoutValue struct {
	Location core.Ref      `property:"LOCATION"`
	Period   core.Interval `property:"PERIOD"`
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

type DocMissingUnit struct {
	ID    []string `documentid:""`
	Width float64  `              property:"WIDTH"` // Missing unit tag.
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
		"PLAIN_URL":       identifier.From("test", "PLAIN_URL"),
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)

	doc := results[0]
	assert.Equal(t, identifier.From("test", "doc1"), doc.ID)

	// Check StringClaim.
	require.Len(t, doc.Claims.String, 1)

	claim := doc.Claims.String[0]
	assert.Equal(t, "Test Document", claim.String)

	propID := mnemonics["NAME"]
	assert.Equal(t, propID, *claim.Prop.ID)

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

	results, errE := transform.Documents(mnemonics, docs)
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

func TestDocuments_TextClaim(t *testing.T) {
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check TextClaims (Description + 2 Notes + HTMLText = 4).
	require.Len(t, doc.Claims.Text, 4)

	// Check HTML escaping.
	assert.Equal(t, "&lt;p&gt;Test&lt;/p&gt;", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "DESCRIPTION", "0"), doc.Claims.Text[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "NOTES", "0"), doc.Claims.Text[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "NOTES", "1"), doc.Claims.Text[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.Text[3].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Check RawTextClaims (RawDescription + 2 RawNotes + RawHTMLText = 4).
	require.Len(t, doc.Claims.Text, 4)

	// Check HTML is NOT escaped for rawhtml type.
	assert.Equal(t, "<p>Test</p>", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, "<b>Note 1</b>", doc.Claims.Text[1].HTML["en"])
	assert.Equal(t, "<i>Note 2</i>", doc.Claims.Text[2].HTML["en"])
	assert.Empty(t, doc.Claims.Text[3].HTML["en"])

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "RAW_DESCRIPTION", "0"), doc.Claims.Text[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "0"), doc.Claims.Text[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_NOTES", "1"), doc.Claims.Text[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "RAW_HTML", "0"), doc.Claims.Text[3].ID)
}

func TestDocuments_ReferenceClaim(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithURL{
			ID:       []string{"test", "doc1"},
			Homepage: "https://example.com",
			Links:    []string{"https://link1.com", "https://link2.com"},
			PlainURL: "https://plain.com",
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Homepage + 2 Links + PlainURL.
	require.Len(t, doc.Claims.Reference, 4)

	assert.Equal(t, "https://example.com", doc.Claims.Reference[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "HOMEPAGE", "0"), doc.Claims.Reference[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "LINKS", "0"), doc.Claims.Reference[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "LINKS", "1"), doc.Claims.Reference[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "PLAIN_URL", "0"), doc.Claims.Reference[3].ID)
}

func TestDocuments_RelationClaim(t *testing.T) {
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Parent + 2 Children = 3.
	require.Len(t, doc.Claims.Relation, 3)

	expectedParentID := identifier.From("parent", "id")
	assert.Equal(t, expectedParentID, *doc.Claims.Relation[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "PARENT", "0"), doc.Claims.Relation[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CHILDREN", "0"), doc.Claims.Relation[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CHILDREN", "1"), doc.Claims.Relation[2].ID)
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
				Timestamp: now,
				Precision: document.TimePrecisionSecond,
			},
			Modified: []core.Time{
				{Timestamp: later, Precision: document.TimePrecisionSecond},
			},
			Published: core.Time{
				Timestamp: now.Add(2 * time.Hour),
				Precision: document.TimePrecisionSecond,
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Time, 3)

	assert.Equal(t, now, time.Time(doc.Claims.Time[0].Timestamp))

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
			Period: core.Interval{
				From:          &core.Time{Timestamp: start, Precision: document.TimePrecisionDay},
				To:            &core.Time{Timestamp: end, Precision: document.TimePrecisionDay},
				FromIsUnknown: false,
				ToIsUnknown:   false,
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.TimeRange, 1)

	claim := doc.Claims.TimeRange[0]
	assert.Equal(t, start, time.Time(claim.Lower))
	assert.Equal(t, end, time.Time(claim.Upper))
	assert.Equal(t, identifier.From("test", "doc1", "PERIOD", "0"), claim.ID)
}

func TestDocuments_IntervalUnknown(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		&DocWithInterval{
			ID: []string{"test", "doc1"},
			Period: core.Interval{
				FromIsUnknown: true,
				ToIsUnknown:   true,
				From:          nil,
				To:            nil,
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Unknown intervals are seen as empty, so no claims are created.
	assert.Empty(t, doc.Claims.UnknownValue, "unknown intervals are skipped")
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Amount, 3)

	// Check Width (float).
	assert.Equal(t, 1.5, doc.Claims.Amount[0].Amount) //nolint:testifylint
	assert.Equal(t, document.AmountUnitMetre, doc.Claims.Amount[0].Unit)
	assert.Equal(t, identifier.From("test", "doc1", "WIDTH", "0"), doc.Claims.Amount[0].ID)

	// Check Height (int).
	assert.Equal(t, 200.0, doc.Claims.Amount[1].Amount) //nolint:testifylint
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[1].ID)

	// Check Count (uint with unit "1").
	assert.Equal(t, 42.0, doc.Claims.Amount[2].Amount) //nolint:testifylint
	assert.Equal(t, document.AmountUnitNone, doc.Claims.Amount[2].Unit)
	assert.Equal(t, identifier.From("test", "doc1", "COUNT", "0"), doc.Claims.Amount[2].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only Published=true should create NoValueClaim.
	// TODO: Make this better. We currently map true to NoValueClaim, but we should to HasClaim.
	require.Len(t, doc.Claims.NoValue, 1)

	propID := mnemonics["PUBLISHED"]
	assert.Equal(t, propID, *doc.Claims.NoValue[0].Prop.ID)
	assert.Equal(t, identifier.From("test", "doc1", "PUBLISHED", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create NoValueClaim for empty string with required cardinality and default:"none" tag.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create NoValueClaim for empty slice with required tag.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only AgeIsUnknown=true should create UnknownValueClaim.
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.UnknownValue[0].ID)

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
				Period: core.Interval{
					From:          &core.Time{Timestamp: start, Precision: document.TimePrecisionYear},
					To:            &core.Time{Timestamp: end, Precision: document.TimePrecisionYear},
					FromIsUnknown: false,
					ToIsUnknown:   false,
				},
				Note: "Important",
			},
			Description: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 string claim with meta claims.
	require.Len(t, doc.Claims.String, 1)

	claim := doc.Claims.String[0]
	assert.Equal(t, "Main Title", claim.String)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), claim.ID)

	// Check meta claims.
	require.NotNil(t, claim.Meta)

	// Should have 1 TimeRange and 1 String meta claim.
	assert.Len(t, claim.Meta.TimeRange, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "PERIOD", "0"), claim.Meta.TimeRange[0].ID)
	assert.Len(t, claim.Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), claim.Meta.String[0].ID)
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
				Period: core.Interval{
					From:          &core.Time{Timestamp: start, Precision: document.TimePrecisionYear},
					To:            &core.Time{Timestamp: end, Precision: document.TimePrecisionYear},
					FromIsUnknown: false,
					ToIsUnknown:   false,
				},
			},
			History: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nested struct without value field creates a NoValueClaim for the ADDRESS property,
	// and the Location and Period fields become meta claims on that NoValueClaim.
	// TODO: Make this better. We currently map true to NoValueClaim, but we should to HasClaim.
	require.Len(t, doc.Claims.NoValue, 1)

	noValueClaim := doc.Claims.NoValue[0]
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0"), noValueClaim.ID)
	require.NotNil(t, noValueClaim.Meta)

	// Should have 1 Relation and 1 TimeRange as meta claims.
	assert.Len(t, noValueClaim.Meta.Relation, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "LOCATION", "0"), noValueClaim.Meta.Relation[0].ID)
	assert.Len(t, noValueClaim.Meta.TimeRange, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "PERIOD", "0"), noValueClaim.Meta.TimeRange[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil pointer should be skipped.
	assert.Empty(t, doc.Claims.Relation)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Non-nil pointer should create claim.
	require.Len(t, doc.Claims.Relation, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.Relation[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Zero values are included as valid amounts.
	require.Len(t, doc.Claims.Amount, 3)

	// Verify all amounts are zero.
	for i, claim := range doc.Claims.Amount {
		assert.Equal(t, 0.0, claim.Amount, "claim %d", i) //nolint:testifylint
	}

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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)

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

	_, errE := transform.Documents(mnemonics, docs)
	assert.EqualError(t, errE, "mnemonic not found")
}

func TestDocuments_MissingUnitTag(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	mnemonics["WIDTH"] = identifier.From("test", "WIDTH")

	docs := []any{
		&DocMissingUnit{
			ID:    []string{"test", "doc1"},
			Width: 1.5, // Missing unit tag.
		},
	}

	_, errE := transform.Documents(mnemonics, docs)
	assert.EqualError(t, errE, `field has numeric type but is missing required "unit" tag`)
}

func TestDocuments_InvalidUnitTag(t *testing.T) {
	t.Parallel()

	type DocInvalidUnit struct {
		ID    []string `documentid:""`
		Width float64  `              property:"WIDTH" unit:"invalid"`
	}

	mnemonics := createMnemonics()
	docs := []any{
		&DocInvalidUnit{
			ID:    []string{"test", "doc1"},
			Width: 1.5,
		},
	}

	_, errE := transform.Documents(mnemonics, docs)
	assert.EqualError(t, errE, "unknown amount unit: invalid")
}

func TestDocuments_NotAStruct(t *testing.T) {
	t.Parallel()

	mnemonics := createMnemonics()
	docs := []any{
		"not a struct",
	}

	_, errE := transform.Documents(mnemonics, docs)
	assert.EqualError(t, errE, "expected struct")
}

func TestDocuments_VariousAmountUnits(t *testing.T) {
	t.Parallel()

	type DocWithVariousUnits struct {
		ID          []string `documentid:""`
		Distance    float64  `              property:"DISTANCE"    unit:"m"`
		Area        float64  `              property:"AREA"        unit:"m²"`
		Volume      float64  `              property:"VOLUME"      unit:"l"`
		Mass        float64  `              property:"MASS"        unit:"kg"`
		Duration    float64  `              property:"DURATION"    unit:"s"`
		Temperature float64  `              property:"TEMPERATURE" unit:"°C"`
		Frequency   float64  `              property:"FREQUENCY"   unit:"Hz"`
		Pressure    float64  `              property:"PRESSURE"    unit:"Pa"`
		Energy      float64  `              property:"ENERGY"      unit:"J"`
		Power       float64  `              property:"POWER"       unit:"W"`
		Voltage     float64  `              property:"VOLTAGE"     unit:"V"`
		Charge      float64  `              property:"CHARGE"      unit:"C"`
		Ratio       float64  `              property:"RATIO"       unit:"/"`
		Pixels      int      `              property:"PIXELS"      unit:"px"`
		Bytes       int      `              property:"BYTES"       unit:"B"`
		Count       int      `              property:"COUNT"       unit:"1"`
	}

	mnemonics := map[string]identifier.Identifier{
		"DISTANCE":    identifier.From("test", "DISTANCE"),
		"AREA":        identifier.From("test", "AREA"),
		"VOLUME":      identifier.From("test", "VOLUME"),
		"MASS":        identifier.From("test", "MASS"),
		"DURATION":    identifier.From("test", "DURATION"),
		"TEMPERATURE": identifier.From("test", "TEMPERATURE"),
		"FREQUENCY":   identifier.From("test", "FREQUENCY"),
		"PRESSURE":    identifier.From("test", "PRESSURE"),
		"ENERGY":      identifier.From("test", "ENERGY"),
		"POWER":       identifier.From("test", "POWER"),
		"VOLTAGE":     identifier.From("test", "VOLTAGE"),
		"CHARGE":      identifier.From("test", "CHARGE"),
		"RATIO":       identifier.From("test", "RATIO"),
		"PIXELS":      identifier.From("test", "PIXELS"),
		"BYTES":       identifier.From("test", "BYTES"),
		"COUNT":       identifier.From("test", "COUNT"),
	}

	docs := []any{
		&DocWithVariousUnits{
			ID:          []string{"test", "doc1"},
			Distance:    100.5,
			Area:        50.25,
			Volume:      2.5,
			Mass:        75.0,
			Duration:    3600.0,
			Temperature: 25.0,
			Frequency:   440.0,
			Pressure:    101325.0,
			Energy:      1000.0,
			Power:       500.0,
			Voltage:     220.0,
			Charge:      1.5,
			Ratio:       0.75,
			Pixels:      1920,
			Bytes:       1024,
			Count:       42,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Amount, 16)

	// Verify units are parsed correctly.
	expectedUnits := []document.AmountUnit{
		document.AmountUnitMetre,
		document.AmountUnitSquareMetre,
		document.AmountUnitLitre,
		document.AmountUnitKilogram,
		document.AmountUnitSecond,
		document.AmountUnitCelsius,
		document.AmountUnitHertz,
		document.AmountUnitPascal,
		document.AmountUnitJoule,
		document.AmountUnitWatt,
		document.AmountUnitVolt,
		document.AmountUnitCoulomb,
		document.AmountUnitRatio,
		document.AmountUnitPixel,
		document.AmountUnitByte,
		document.AmountUnitNone,
	}

	for i, expected := range expectedUnits {
		assert.Equal(t, expected, doc.Claims.Amount[i].Unit, "claim %d", i)
	}

	// Verify claim IDs.
	expectedProperties := []string{
		"DISTANCE", "AREA", "VOLUME", "MASS", "DURATION", "TEMPERATURE",
		"FREQUENCY", "PRESSURE", "ENERGY", "POWER", "VOLTAGE", "CHARGE",
		"RATIO", "PIXELS", "BYTES", "COUNT",
	}
	for i, prop := range expectedProperties {
		assert.Equal(t, identifier.From("test", "doc1", prop, "0"), doc.Claims.Amount[i].ID, "claim %d", i)
	}
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have Name and Extra as string claims.
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "EXTRA", "0"), doc.Claims.String[1].ID)

	// Should have Author as relation claim.
	require.Len(t, doc.Claims.Relation, 1)

	expectedAuthorID := identifier.From("author", "1")
	assert.Equal(t, expectedAuthorID, *doc.Claims.Relation[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "0"), doc.Claims.Relation[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
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
	require.Len(t, doc.Claims.Relation, 1)

	expectedInstanceOf := identifier.From("core", "PROPERTY")
	assert.Equal(t, expectedInstanceOf, *doc.Claims.Relation[0].To.ID)
	assert.Equal(t, identifier.From("test", "PROP", "INSTANCE_OF", "0"), doc.Claims.Relation[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	// Test that nil *core.Ref in value field creates NoValueClaim.
	type RefWithMeta struct {
		Ref  *core.Ref `                value:""`
		Note string    `property:"NOTE"`
	}

	type DocWithNilRefValue struct {
		ID       []string    `documentid:""`
		Optional RefWithMeta `              property:"OPTIONAL"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithNilRefValue{
			ID: []string{"test", "doc1"},
			Optional: RefWithMeta{
				Ref:  nil, // Nil ref.
				Note: "Some note",
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil *core.Ref value creates NoValueClaim for the struct.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.NoValue[0].ID)

	// Should have meta claim (Note).
	require.NotNil(t, doc.Claims.NoValue[0].Meta)
	assert.Len(t, doc.Claims.NoValue[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0", "NOTE", "0"), doc.Claims.NoValue[0].Meta.String[0].ID)
}

func TestDocuments_EmptyRefValue(t *testing.T) {
	t.Parallel()

	type RefWithMeta struct {
		Ref  core.Ref `                value:""`
		Note string   `property:"NOTE"`
	}

	type DocWithEmptyRefValue struct {
		ID       []string    `documentid:""`
		Optional RefWithMeta `              property:"OPTIONAL"`
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithEmptyRefValue{
			ID: []string{"test", "doc1"},
			Optional: RefWithMeta{
				Ref:  core.Ref{ID: []string{}}, // Empty ref.
				Note: "Some note",
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should create NoValueClaim for empty ref with meta claims.
	assert.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 2 relation claims (Author[0] and Artist[0]).
	require.Len(t, doc.Claims.Relation, 2)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "0"), doc.Claims.Relation[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_ARTIST", "0"), doc.Claims.Relation[1].ID)

	// Should have 2 unknown value claims (AuthorHasUnknown and ArtistHasUnknown).
	require.Len(t, doc.Claims.UnknownValue, 2)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_AUTHOR", "1"), doc.Claims.UnknownValue[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "HAS_ARTIST", "1"), doc.Claims.UnknownValue[1].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without html tag, should still be TextClaim.
	require.Len(t, doc.Claims.Text, 1)

	assert.Equal(t, "&lt;p&gt;HTML content&lt;/p&gt;", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.Text[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without rawhtml tag, should still be TextClaim with unescaped HTML.
	require.Len(t, doc.Claims.Text, 1)

	assert.Equal(t, "<p>HTML content</p>", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "HTML", "0"), doc.Claims.Text[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Text, 4)

	// Verify HTML is escaped.
	escapedExpected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	assert.Equal(t, escapedExpected, doc.Claims.Text[0].HTML["en"], "type:html should escape")
	assert.Equal(t, escapedExpected, doc.Claims.Text[2].HTML["en"], "core.HTML should escape")

	// Verify RawHTML is sanitized.
	assert.Empty(t, doc.Claims.Text[1].HTML["en"], "type:rawhtml should sanitize")
	assert.Empty(t, doc.Claims.Text[3].HTML["en"], "core.RawHTML should sanitize")

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "ESCAPED", "0"), doc.Claims.Text[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "UNESCAPED", "0"), doc.Claims.Text[1].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CORE_HTML", "0"), doc.Claims.Text[2].ID)
	assert.Equal(t, identifier.From("test", "doc1", "CORE_RAW", "0"), doc.Claims.Text[3].ID)
}

func TestDocuments_CoreURLWithoutTag(t *testing.T) {
	t.Parallel()

	// Test that core.URL without url tag is treated as URL.
	type DocWithPlainURL struct {
		ID       []string `documentid:""`
		PlainURL core.URL `              property:"PLAIN_URL"` // No url tag.
	}

	mnemonics := createMnemonics()

	docs := []any{
		&DocWithPlainURL{
			ID:       []string{"test", "doc1"},
			PlainURL: "https://example.com",
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without url tag, should still be ReferenceClaim.
	require.Len(t, doc.Claims.Reference, 1)

	assert.Equal(t, "https://example.com", doc.Claims.Reference[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "PLAIN_URL", "0"), doc.Claims.Reference[0].ID)
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
				Timestamp: now,
				Precision: document.TimePrecisionSecond,
			},
			InvalidTime: core.Time{}, // Zero value.
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
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
		ID            []string      `documentid:""`
		ValidPeriod   core.Interval `              property:"VALID_PERIOD"`
		InvalidPeriod core.Interval `              property:"INVALID_PERIOD"` // Will be empty.
	}

	mnemonics := createMnemonics()
	mnemonics["VALID_PERIOD"] = identifier.From("test", "VALID_PERIOD")
	mnemonics["INVALID_PERIOD"] = identifier.From("test", "INVALID_PERIOD")

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := []any{
		&DocWithEmptyInterval{
			ID: []string{"test", "doc1"},
			ValidPeriod: core.Interval{
				From:          &core.Time{Timestamp: start, Precision: document.TimePrecisionYear},
				To:            &core.Time{Timestamp: end, Precision: document.TimePrecisionYear},
				FromIsUnknown: false,
				ToIsUnknown:   false,
			},
			InvalidPeriod: core.Interval{
				From:          nil,
				To:            nil,
				FromIsUnknown: false,
				ToIsUnknown:   false,
			}, // Empty interval.
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only ValidPeriod should create a claim (empty interval is skipped).
	require.Len(t, doc.Claims.TimeRange, 1, "empty interval skipped")
	assert.Equal(t, identifier.From("test", "doc1", "VALID_PERIOD", "0"), doc.Claims.TimeRange[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 relation claim.
	require.Len(t, doc.Claims.Relation, 1)

	expectedRefID := identifier.From("target", "1")
	assert.Equal(t, expectedRefID, *doc.Claims.Relation[0].To.ID)
	assert.Equal(t, identifier.From("test", "doc1", "TARGET", "0"), doc.Claims.Relation[0].ID)

	// Should have meta claim (Note).
	require.NotNil(t, doc.Claims.Relation[0].Meta)
	assert.Len(t, doc.Claims.Relation[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TARGET", "0", "NOTE", "0"), doc.Claims.Relation[0].Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	require.Len(t, doc.Claims.Text, 1)

	// HTML should be escaped.
	expected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	assert.Equal(t, expected, doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.Text[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 string claim.
	require.Len(t, doc.Claims.String, 1)

	assert.Equal(t, "Test Title", doc.Claims.String[0].String)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.String[0].ID)

	// Should have meta claim (Note).
	require.NotNil(t, doc.Claims.String[0].Meta)
	assert.Len(t, doc.Claims.String[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), doc.Claims.String[0].Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil value without required creates NoValueClaim.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)

	// Should have meta claim.
	require.NotNil(t, doc.Claims.NoValue[0].Meta)
	assert.Len(t, doc.Claims.NoValue[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0", "NOTE", "0"), doc.Claims.NoValue[0].Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil required value creates NoValueClaim.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)
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
		Timestamp: now,
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 time claim.
	require.Len(t, doc.Claims.Time, 1)

	assert.Equal(t, now, time.Time(doc.Claims.Time[0].Timestamp))
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0"), doc.Claims.Time[0].ID)

	// Should have meta claim (Note).
	require.NotNil(t, doc.Claims.Time[0].Meta)
	assert.Len(t, doc.Claims.Time[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "CREATED", "0", "NOTE", "0"), doc.Claims.Time[0].Meta.String[0].ID)
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
					Timestamp: now,
					Precision: document.TimePrecisionSecond,
				},
				Note: "Important date",
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 text claim.
	require.Len(t, doc.Claims.Text, 1)

	// HTML should be escaped.
	assert.Equal(t, "&lt;p&gt;Test&lt;/p&gt;", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.Text[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 text claim.
	require.Len(t, doc.Claims.Text, 1)

	// HTML should NOT be escaped for rawhtml type.
	assert.Equal(t, "<p>Test</p>", doc.Claims.Text[0].HTML["en"])
	assert.Equal(t, identifier.From("test", "doc1", "CONTENT", "0"), doc.Claims.Text[0].ID)
}

func TestDocuments_ValueFieldWithURLTag(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test value:"" with type:"url" tag.
	type URLValue struct {
		Value string `                type:"url" value:""`
		Note  string `property:"NOTE"`
	}

	type DocWithURLValue struct {
		ID   []string `documentid:""`
		Link URLValue `              property:"LINK"`
	}

	mnemonics := createMnemonics()
	mnemonics["LINK"] = identifier.From("test", "LINK")

	docs := []any{
		&DocWithURLValue{
			ID: []string{"test", "doc1"},
			Link: URLValue{
				Value: "https://example.com",
				Note:  "Homepage",
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 reference claim.
	require.Len(t, doc.Claims.Reference, 1)

	assert.Equal(t, "https://example.com", doc.Claims.Reference[0].IRI)
	assert.Equal(t, identifier.From("test", "doc1", "LINK", "0"), doc.Claims.Reference[0].ID)
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
	_, errE := transform.Documents(mnemonics, docs)

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
	_, errE := transform.Documents(mnemonics, docs)

	assert.EqualError(t, errE, "field has unsupported or unexpected value type")
}

func TestDocuments_ValueFieldWithAmount(t *testing.T) {
	t.Parallel()

	// Test value:"" with numeric type and unit tag.
	type AmountValue struct {
		Value float64 `                unit:"m" value:""`
		Note  string  `property:"NOTE"`
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 1 amount claim.
	require.Len(t, doc.Claims.Amount, 1)

	assert.Equal(t, 1.75, doc.Claims.Amount[0].Amount) //nolint:testifylint

	assert.Equal(t, document.AmountUnitMetre, doc.Claims.Amount[0].Unit)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0"), doc.Claims.Amount[0].ID)

	// Should have meta claim.
	require.NotNil(t, doc.Claims.Amount[0].Meta)
	assert.Len(t, doc.Claims.Amount[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "HEIGHT", "0", "NOTE", "0"), doc.Claims.Amount[0].Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Bool true creates NoValueClaim.
	// TODO: Make this better. We currently map true to NoValueClaim, but we should to HasClaim.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0"), doc.Claims.NoValue[0].ID)

	// Should have meta claim.
	require.NotNil(t, doc.Claims.NoValue[0].Meta)
	assert.Len(t, doc.Claims.NoValue[0].Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0", "NOTE", "0"), doc.Claims.NoValue[0].Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Bool false is empty, so NoValueClaim for the struct.
	require.Len(t, doc.Claims.NoValue, 1, "false bool creates no value claim for struct")
	assert.Equal(t, identifier.From("test", "doc1", "ACTIVE", "0"), doc.Claims.NoValue[0].ID)
}

func TestDocuments_StructWithoutValueField(t *testing.T) {
	t.Parallel()

	// Test struct without value:"" field - all fields become meta claims.
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Without value field, creates NoValueClaim with meta claims.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0"), doc.Claims.NoValue[0].ID)

	noValueClaim := doc.Claims.NoValue[0]
	require.NotNil(t, noValueClaim.Meta)

	// Should have Location and Note as meta claims.
	assert.Len(t, noValueClaim.Meta.Relation, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "LOCATION", "0"), noValueClaim.Meta.Relation[0].ID)
	assert.Len(t, noValueClaim.Meta.String, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ADDRESS", "0", "NOTE", "0"), noValueClaim.Meta.String[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil slice with required creates NoValueClaim.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Nil pointer with required creates NoValueClaim.
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "OPTIONAL", "0"), doc.Claims.NoValue[0].ID)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Should have 2 string claims (one for each item).
	require.Len(t, doc.Claims.String, 2)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "1"), doc.Claims.String[1].ID)

	// Each should have meta claims.
	for i, claim := range doc.Claims.String {
		require.NotNil(t, claim.Meta, "claim %d", i)
		assert.Len(t, claim.Meta.String, 1, "claim %d", i)
	}
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "0", "NOTE", "0"), doc.Claims.String[0].Meta.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "ITEMS", "1", "NOTE", "0"), doc.Claims.String[1].Meta.String[0].ID)
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
	_, errE := transform.Documents(mnemonics, docs)

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
	_, errE := transform.Documents(mnemonics, docs)

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
	_, errE := transform.Documents(mnemonics, docs)

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
	_, errE := transform.Documents(mnemonics, docs)

	assert.EqualError(t, errE, "multiple document IDs found")
}

func TestDocuments_InfinityFloatError(t *testing.T) {
	t.Parallel()

	// Test that infinity float returns error.
	type DocWithInfinity struct {
		ID    []string `documentid:""`
		Value float64  `              property:"VALUE" unit:"1"`
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
	_, errE := transform.Documents(mnemonics, docs)

	assert.EqualError(t, errE, "value is infinity or not a number")
}

func TestDocuments_NaNFloatError(t *testing.T) {
	t.Parallel()

	// Test that NaN float returns error.
	type DocWithNaN struct {
		ID    []string `documentid:""`
		Value float64  `              property:"VALUE" unit:"1"`
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
	_, errE := transform.Documents(mnemonics, docs)

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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := results[0]

	// Only Name and Extra should be processed (embedded struct skipped).
	require.Len(t, doc.Claims.String, 2)

	// Verify the claims are Name and Extra, not Field1 or Field2.
	propIDs := []identifier.Identifier{
		*doc.Claims.String[0].Prop.ID,
		*doc.Claims.String[1].Prop.ID,
	}

	nameID := mnemonics["NAME"]
	extraID := mnemonics["EXTRA"]

	assert.True(t, (propIDs[0] == nameID && propIDs[1] == extraID) || (propIDs[0] == extraID && propIDs[1] == nameID), "expected Name and Extra properties, got %v", propIDs)

	// Verify claim IDs.
	assert.Equal(t, identifier.From("test", "doc1", "NAME", "0"), doc.Claims.String[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "EXTRA", "0"), doc.Claims.String[1].ID)
}

func TestDocuments_ComplexSharedPropertyWithMetaAndValue(t *testing.T) {
	t.Parallel()

	// Test complex scenario: multiple sources contributing to same property,
	// including multiple embedded structs, top-level fields, value fields, and meta fields.
	type EmbeddedSharedA struct {
		Field1 string `property:"SHARED"`
		Field2 string `property:"SHARED"`
	}

	type EmbeddedSharedB struct {
		Field3 []string `property:"SHARED"` // Slice in embedded struct.
	}

	type ValueWithMetaShared struct {
		Value      string   `                  value:""` // Value for SHARED property.
		MetaShared []string `property:"SHARED"`          // Meta field also for SHARED property.
		MetaNote   string   `property:"NOTE"`            // Meta field for different property.
	}

	type DocComplex struct {
		EmbeddedSharedA
		EmbeddedSharedB

		ID []string `documentid:""`

		TopShared1 string              `property:"SHARED"` // Top-level field for SHARED.
		TopShared2 []string            `property:"SHARED"` // Slice creates multiple claims.
		WithValue  ValueWithMetaShared `property:"SHARED"` // Value field for SHARED with meta.
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
			WithValue: ValueWithMetaShared{
				Value:      "From Value",
				MetaShared: []string{"Meta Shared 1", "Meta Shared 2"},
				MetaNote:   "Meta Note",
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
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
		assert.Equal(t, sharedPropID, *claim.Prop.ID, "claim %d", i)
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

	// Verify the value claim (claim 7) has meta claims.
	valueClaim := doc.Claims.String[7]
	require.NotNil(t, valueClaim.Meta)

	// Meta claims: MetaShared creates 2 string claims for SHARED, MetaNote creates 1 for NOTE.
	assert.Len(t, valueClaim.Meta.String, 3)

	// Meta claim IDs should start from 0 within the meta context.
	// Meta claims inherit parent path ["test", "complex", "SHARED", "7"] and add their property.
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "SHARED", "0"), valueClaim.Meta.String[0].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "SHARED", "1"), valueClaim.Meta.String[1].ID)
	assert.Equal(t, identifier.From("test", "complex", "SHARED", "7", "NOTE", "0"), valueClaim.Meta.String[2].ID)

	// Verify meta claims have correct property IDs.
	assert.Equal(t, sharedPropID, *valueClaim.Meta.String[0].Prop.ID)
	assert.Equal(t, sharedPropID, *valueClaim.Meta.String[1].Prop.ID)
	assert.Equal(t, mnemonics["NOTE"], *valueClaim.Meta.String[2].Prop.ID)
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

		result, errE := transform.Documents(mnemonics, docs)
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

		result, errE := transform.Documents(mnemonics, docs)
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
	// No value field, only meta claims.
	MetaField string `property:"META"`
}

type DocWithNestedStructEmptyMetaClaims struct {
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
		"META":    identifier.New(),
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
					{Timestamp: time.Now(), Precision: document.TimePrecisionSecond}, // Valid.
				},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
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

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, "field has unsupported or unexpected value type")
		assert.Nil(t, result)
	})

	t.Run("NestedStructNoValueWithMetaClaims", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNestedStructNoValue{
				ID: []string{"doc1"},
				Nested: NestedStructNoValue{
					MetaField: "meta-value",
				},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should create a NoValueClaim since there's no value field but there are meta claims.
		nestedProperty := mnemonics["NESTED"]
		nestedClaims := result[0].Get(nestedProperty)
		assert.Len(t, nestedClaims, 1)
		assert.IsType(t, &document.NoValueClaim{}, nestedClaims[0])
	})

	t.Run("NestedStructWithEmptyMetaClaims", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNestedStructEmptyMetaClaims{
				ID:     []string{"doc1"},
				Nested: NestedStructEmpty{},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should not create a claim since there's no value field and no meta claims.
		nestedProperty := mnemonics["NESTED"]
		nestedClaims := result[0].Get(nestedProperty)
		assert.Empty(t, nestedClaims)
	})
}

type DocWithConflictingTags struct {
	ID            []string        `documentid:""`
	IdentifierStr core.Identifier `              property:"ID_STR"   type:"html"` // Conflicting type tag.
	URLStr        core.URL        `              property:"URL_STR"  type:"id"`   // Conflicting type tag.
	HTMLStr       core.HTML       `              property:"HTML_STR" type:"id"`   // Conflicting type tag.
}

func TestDocuments_ConflictingTags(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"ID_STR":   identifier.New(),
		"URL_STR":  identifier.New(),
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

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, "identifier field used with conflicting tag")
		assert.Nil(t, result)
	})

	t.Run("URLWithConflictingTag", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithConflictingTags{ //nolint:exhaustruct
				ID:     []string{"doc1"},
				URLStr: "https://example.com",
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, "URL field used with conflicting tag")
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

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, "HTML field used with conflicting tag")
		assert.Nil(t, result)
	})
}

type DocWithNumericNoUnit struct {
	ID    []string `documentid:""`
	Count int      `              property:"COUNT"` // Missing required unit tag.
}

type DocWithInvalidDocID struct {
	ID string `documentid:""` // Should be []string, not string.
}

type DocWithInvalidFloatValue struct {
	ID    []string `documentid:""`
	Value float64  `              property:"VALUE" unit:"1"`
}

func TestDocuments_MoreEdgeCases(t *testing.T) {
	t.Parallel()

	mnemonics := map[string]identifier.Identifier{
		"COUNT": identifier.New(),
		"VALUE": identifier.New(),
	}

	t.Run("NumericFieldWithoutUnit", func(t *testing.T) {
		t.Parallel()

		docs := []any{
			&DocWithNumericNoUnit{
				ID:    []string{"doc1"},
				Count: 42,
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, `field has numeric type but is missing required "unit" tag`)
		assert.Nil(t, result)
	})

	t.Run("DocumentIDNotStringSlice", func(t *testing.T) {
		t.Parallel()

		// This will fail during transformation because ID should be []string.
		docs := []any{
			&DocWithInvalidDocID{
				ID: "not-a-slice",
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
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

		result, errE := transform.Documents(mnemonics, docs)
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

		result, errE := transform.Documents(mnemonics, docs)
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

		result, errE := transform.Documents(mnemonics, docs)
		assert.EqualError(t, errE, "property tag cannot be used with value tag")
		assert.Nil(t, result)
	})
}

type DocWithIntervalVariants struct {
	ID       []string      `documentid:""`
	Interval core.Interval `              property:"INTERVAL"`
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

		result, errE := transform.Documents(mnemonics, docs)
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
				Interval: core.Interval{ //nolint:exhaustruct
					From: &core.Time{
						Timestamp: start,
						Precision: document.TimePrecisionSecond,
					},
					To: &core.Time{
						Timestamp: end,
						Precision: document.TimePrecisionDay, // Different precision - should use higher.
					},
				},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		intervalProperty := mnemonics["INTERVAL"]
		intervalClaims := result[0].Get(intervalProperty)
		require.Len(t, intervalClaims, 1)

		timeRangeClaim, ok := intervalClaims[0].(*document.TimeRangeClaim)
		require.True(t, ok)
		// Verify that precision is set (the code uses the higher numerical value).
		assert.Positive(t, int(timeRangeClaim.Precision))
	})

	t.Run("IntervalWithNilFrom", func(t *testing.T) {
		t.Parallel()

		end := time.Now()

		docs := []any{
			&DocWithIntervalVariants{
				ID: []string{"doc1"},
				Interval: core.Interval{ //nolint:exhaustruct
					From: nil,
					To: &core.Time{
						Timestamp: end,
						Precision: document.TimePrecisionDay,
					},
				},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should create UnknownValueClaim for interval with nil From.
		// TODO: This is just temporary. Support unknown interval bounds.
		intervalProperty := mnemonics["INTERVAL"]
		intervalClaims := result[0].Get(intervalProperty)
		require.Len(t, intervalClaims, 1)
		assert.IsType(t, &document.UnknownValueClaim{}, intervalClaims[0])
	})

	t.Run("IntervalWithUnknownBounds", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		end := start.Add(24 * time.Hour)

		docs := []any{
			&DocWithIntervalVariants{
				ID: []string{"doc1"},
				Interval: core.Interval{ //nolint:exhaustruct
					From: &core.Time{
						Timestamp: start,
						Precision: document.TimePrecisionDay,
					},
					To: &core.Time{
						Timestamp: end,
						Precision: document.TimePrecisionDay,
					},
					FromIsUnknown: true, // Unknown bound.
				},
			},
		}

		result, errE := transform.Documents(mnemonics, docs)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, result, 1)

		// Should create UnknownValueClaim for interval with unknown bounds.
		// TODO: This is just temporary. Support unknown interval bounds.
		intervalProperty := mnemonics["INTERVAL"]
		intervalClaims := result[0].Get(intervalProperty)
		require.Len(t, intervalClaims, 1)
		assert.IsType(t, &document.UnknownValueClaim{}, intervalClaims[0])
	})
}

type DocFlatFields struct {
	ID          []string `documentid:""`
	Name        string   `              property:"NAME"`
	Description string   `              property:"DESCRIPTION" type:"html"`
	Age         int      `              property:"AGE"                     unit:"1"`
	IsActive    bool     `              property:"IS_ACTIVE"`
}

type CommonFields struct {
	Name        string `property:"NAME"`
	Description string `property:"DESCRIPTION" type:"html"`
	Age         int    `property:"AGE"                     unit:"1"`
	IsActive    bool   `property:"IS_ACTIVE"`
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

	flatResult, errE := transform.Documents(mnemonics, []any{flatDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, flatResult, 1)

	embeddedResult, errE := transform.Documents(mnemonics, []any{embeddedDoc})
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

	nestedResult, errE := transform.Documents(mnemonics, []any{nestedDoc})
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, nestedResult, 1)

	flatResult, errE := transform.Documents(mnemonics, []any{flatDoc})
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Empty(t, doc.Claims.String)
	require.Empty(t, doc.Claims.NoValue)

	// Test with one value - should succeed.
	hello := "Hello"
	docs = []any{
		&DocWithOptionalSingle{
			ID:    []string{"test", "doc2"},
			Title: &hello,
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
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

	// Test with empty slice - should add one NoValueClaim (due to default:"none").
	docs := []any{
		&DocWithOneOrMore{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)

	// Test with one value - should succeed.
	docs = []any{
		&DocWithOneOrMore{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
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

	results, errE = transform.Documents(mnemonics, docs)
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

	// Test with 0 values - should add 2 NoValueClaims (due to default:"none").
	docs := []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.NoValue[1].ID)

	// Test with 1 value - should add 1 NoValueClaim.
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.NoValue, 1)

	// Test with exactly 2 values - should succeed.
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 2)
	require.Empty(t, doc.Claims.NoValue)

	// Test with 3 values - should fail (exceeds max).
	docs = []any{
		&DocWithExactTwo{
			ID:     []string{"test", "doc4"},
			Titles: []string{"Title 1", "Title 2", "Title 3"},
		},
	}

	_, errE = transform.Documents(mnemonics, docs)
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

	// Test with 0 values - should add 2 NoValueClaims.
	docs := []any{
		&DocWithRange{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 2)

	// Test with 1 value - should add 1 NoValueClaim.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.NoValue, 1)

	// Test with 2 values - should succeed.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 2)
	require.Empty(t, doc.Claims.NoValue)

	// Test with 4 values - should succeed.
	docs = []any{
		&DocWithRange{
			ID:     []string{"test", "doc4"},
			Titles: []string{"Title 1", "Title 2", "Title 3", "Title 4"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
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

	_, errE = transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value exceeds maximum cardinality")
}

func TestDocuments_NoneTag_WithCardinality(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test that with default:"none", missing required values are filled with NoValueClaims.
	type DocWithNone struct {
		ID     []string `                               documentid:""`
		Titles []string `cardinality:"2" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 0 values, should add 2 NoValueClaims.
	docs := []any{
		&DocWithNone{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.NoValue[1].ID)

	// With 1 value, should add 1 NoValueClaim.
	docs = []any{
		&DocWithNone{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc2", "TITLE", "1"), doc.Claims.NoValue[0].ID)
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

	_, errE := transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")

	// With 1 value, should return error.
	docs = []any{
		&DocWithoutNone{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	_, errE = transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")

	// With 2 values, should succeed.
	docs = []any{
		&DocWithoutNone{
			ID:     []string{"test", "doc3"},
			Titles: []string{"Title 1", "Title 2"},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
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

	// With nil pointer, should add 1 NoValueClaim.
	docs := []any{
		&DocWithNonePointer{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "VALUE", "0"), doc.Claims.NoValue[0].ID)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	// With non-nil pointer.
	docs = []any{
		&DocWithPointerZeroOrOne{
			ID:    []string{"test", "doc2"},
			Value: &core.Ref{ID: []string{"test", "ref1"}},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.Relation, 1)
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

	// With nil pointer and default:"none" tag, should add NoValueClaim.
	docs := []any{
		&DocWithPointerAndNone{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
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

	results, errE = transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "test", doc.Claims.String[0].String)
}

func TestDocuments_EmptyStructAsValue(t *testing.T) {
	t.Parallel()

	// Test struct with no value field and no meta claims produces no claim.
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Empty struct with no value field and no meta claims produces no claim.
	fieldProperty := mnemonics["FIELD"]
	fieldClaims := doc.Get(fieldProperty)
	assert.Empty(t, fieldClaims)
}

func TestDocuments_EmbeddedStructWithValueClaim(t *testing.T) {
	t.Parallel()

	// Test embedded struct that contains a value field.
	type EmbeddedWithValue struct {
		Value string `                value:""`
		Meta  string `property:"META"`
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
	mnemonics["META"] = identifier.From("test", "META")

	docs := []any{
		&DocWithEmbeddedValue{
			ID: []string{"test", "doc1"},
			Field: OuterStruct{
				EmbeddedWithValue: EmbeddedWithValue{
					Value: "test-value",
					Meta:  "meta-value",
				},
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	assert.Equal(t, "test-value", doc.Claims.String[0].String)

	// Check meta claim.
	require.NotNil(t, doc.Claims.String[0].Meta)
	require.Len(t, doc.Claims.String[0].Meta.String, 1)
	assert.Equal(t, "meta-value", doc.Claims.String[0].Meta.String[0].String)
}

func TestDocuments_EmbeddedStructWithEmptyValue(t *testing.T) {
	t.Parallel()

	// Test embedded struct with empty value field but has meta claims.
	type EmbeddedWithEmptyValue struct {
		Value string `                value:""`
		Meta  string `property:"META"`
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
	mnemonics["META"] = identifier.From("test", "META")

	docs := []any{
		&DocWithEmbeddedEmptyValue{
			ID: []string{"test", "doc1"},
			Field: OuterStruct{
				EmbeddedWithEmptyValue: EmbeddedWithEmptyValue{
					Value: "", // Empty value.
					Meta:  "meta-value",
				},
			},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have NoValueClaim since value is empty but there are meta claims.
	fieldProperty := mnemonics["FIELD"]
	fieldClaims := doc.Get(fieldProperty)
	require.Len(t, fieldClaims, 1)
	assert.IsType(t, &document.NoValueClaim{}, fieldClaims[0])

	// Check meta claim exists.
	noValueClaim, ok := fieldClaims[0].(*document.NoValueClaim)
	require.True(t, ok)
	require.NotNil(t, noValueClaim.Meta)
	require.Len(t, noValueClaim.Meta.String, 1)
	assert.Equal(t, "meta-value", noValueClaim.Meta.String[0].String)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")
}

func TestDocuments_UnknownTag_WithCardinality(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test that with default:"unknown", missing required values are filled with UnknownValueClaims.
	type DocWithUnknownCardinality struct {
		ID     []string `                                  documentid:""`
		Titles []string `cardinality:"2" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 0 values, should add 2 UnknownValueClaims.
	docs := []any{
		&DocWithUnknownCardinality{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 2)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.UnknownValue[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "1"), doc.Claims.UnknownValue[1].ID)

	// With 1 value, should add 1 UnknownValueClaim.
	docs = []any{
		&DocWithUnknownCardinality{
			ID:     []string{"test", "doc2"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc2", "TITLE", "1"), doc.Claims.UnknownValue[0].ID)
}

func TestDocuments_UnknownTag_OneOrMore(t *testing.T) {
	t.Parallel()

	// Test default:"unknown" with cardinality "1..".
	type DocWithUnknownOneOrMore struct {
		ID     []string `                                    documentid:""`
		Titles []string `cardinality:"1.." default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With empty slice, should add one UnknownValueClaim.
	docs := []any{
		&DocWithUnknownOneOrMore{
			ID:     []string{"test", "doc1"},
			Titles: []string{},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.UnknownValue[0].ID)
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

	// With nil pointer, should add 1 UnknownValueClaim.
	docs := []any{
		&DocWithUnknownPointer{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "VALUE", "0"), doc.Claims.UnknownValue[0].ID)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	// With empty value, should add 1 UnknownValueClaim.
	docs := []any{
		&DocWithUnknownSingle{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.UnknownValue[0].ID)
}

func TestDocuments_UnknownTag_BooleanBehavior(t *testing.T) {
	t.Parallel()

	// Test that type:"unknown" on boolean field still works as before (creates UnknownValueClaim when true).
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

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have UnknownValueClaim for AGE (from boolean field).
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.UnknownValue[0].ID)
}

func TestDocuments_NoneTag_BooleanBehavior(t *testing.T) {
	t.Parallel()

	// Test that type:"none" on boolean field creates NoValueClaim when true.
	type DocWithNoneBool struct {
		ID               []string `documentid:""`
		Name             string   `              property:"NAME"`
		LastNameIsAbsent bool     `              property:"LAST_NAME" type:"none"`
	}

	mnemonics := createMnemonics()
	mnemonics["LAST_NAME"] = identifier.From("test", "LAST_NAME")

	// When true, should create NoValueClaim.
	docs := []any{
		&DocWithNoneBool{
			ID:               []string{"test", "doc1"},
			Name:             "John",
			LastNameIsAbsent: true,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have NoValueClaim for LAST_NAME (from boolean field).
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.NoValue[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithNoneBool{
			ID:               []string{"test", "doc2"},
			Name:             "Jane",
			LastNameIsAbsent: false,
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.NoValue)
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
			LastNameIsAbsent: true,       // Creates NoValueClaim (boolean mode).
			Titles:           []string{}, // Creates NoValueClaim (cardinality mode).
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	// Should have 2 NoValueClaims (one from boolean, one from cardinality).
	require.Len(t, doc.Claims.NoValue, 2)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.NoValue[0].ID)
	assert.Equal(t, identifier.From("test", "doc1", "TITLE", "0"), doc.Claims.NoValue[1].ID)
}

func TestDocuments_CoreNoneType(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test core.None type creates NoValueClaim when true.
	type DocWithCoreNone struct {
		ID               []string  `documentid:""`
		Name             string    `              property:"NAME"`
		LastNameIsAbsent core.None `              property:"LAST_NAME"`
	}

	mnemonics := createMnemonics()
	mnemonics["LAST_NAME"] = identifier.From("test", "LAST_NAME")

	// When true, should create NoValueClaim.
	docs := []any{
		&DocWithCoreNone{
			ID:               []string{"test", "doc1"},
			Name:             "John",
			LastNameIsAbsent: core.None(true),
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "LAST_NAME", "0"), doc.Claims.NoValue[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithCoreNone{
			ID:               []string{"test", "doc2"},
			Name:             "Jane",
			LastNameIsAbsent: core.None(false),
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.NoValue)
}

func TestDocuments_CoreUnknownType(t *testing.T) { //nolint:dupl
	t.Parallel()

	// Test core.Unknown type creates UnknownValueClaim when true.
	type DocWithCoreUnknown struct {
		ID           []string     `documentid:""`
		Name         string       `              property:"NAME"`
		AgeIsUnknown core.Unknown `              property:"AGE"`
	}

	mnemonics := createMnemonics()
	mnemonics["AGE"] = identifier.From("test", "AGE")

	// When true, should create UnknownValueClaim.
	docs := []any{
		&DocWithCoreUnknown{
			ID:           []string{"test", "doc1"},
			Name:         "John",
			AgeIsUnknown: core.Unknown(true),
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.UnknownValue[0].ID)

	// When false, should not create any claim.
	docs = []any{
		&DocWithCoreUnknown{
			ID:           []string{"test", "doc2"},
			Name:         "Jane",
			AgeIsUnknown: core.Unknown(false),
		},
	}

	results, errE = transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc = results[0]
	require.Empty(t, doc.Claims.UnknownValue)
}

func TestDocuments_BoolWithTypeNone(t *testing.T) {
	t.Parallel()

	// Test regular bool with type:"none" creates NoValueClaim when true.
	type DocWithBoolNone struct {
		ID       []string `documentid:""`
		Name     string   `              property:"NAME"`
		IsAbsent bool     `              property:"NOTE" type:"none"`
	}

	mnemonics := createMnemonics()
	mnemonics["NOTE"] = identifier.From("test", "NOTE")

	// When true, should create NoValueClaim.
	docs := []any{
		&DocWithBoolNone{
			ID:       []string{"test", "doc1"},
			Name:     "John",
			IsAbsent: true,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "NOTE", "0"), doc.Claims.NoValue[0].ID)
}

func TestDocuments_BoolWithTypeUnknown(t *testing.T) {
	t.Parallel()

	// Test regular bool with type:"unknown" creates UnknownValueClaim when true.
	type DocWithBoolUnknown struct {
		ID        []string `documentid:""`
		Name      string   `              property:"NAME"`
		IsUnknown bool     `              property:"AGE"  type:"unknown"`
	}

	mnemonics := createMnemonics()
	mnemonics["AGE"] = identifier.From("test", "AGE")

	// When true, should create UnknownValueClaim.
	docs := []any{
		&DocWithBoolUnknown{
			ID:        []string{"test", "doc1"},
			Name:      "John",
			IsUnknown: true,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
	assert.Equal(t, identifier.From("test", "doc1", "AGE", "0"), doc.Claims.UnknownValue[0].ID)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	// Should fail minimum cardinality since invalid default tag is ignored.
	assert.EqualError(t, errE, "field value does not satisfy minimum cardinality")
}

func TestDocuments_SliceWithDefaultNone(t *testing.T) {
	t.Parallel()

	// Test slice with multiple missing values filled by default:"none".
	type DocWithSliceDefault struct {
		ID     []string `                                  documentid:""`
		Titles []string `cardinality:"3..5" default:"none"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 1 value, should add 2 NoValueClaims.
	docs := []any{
		&DocWithSliceDefault{
			ID:     []string{"test", "doc1"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.NoValue, 2)
}

func TestDocuments_SliceWithDefaultUnknown(t *testing.T) {
	t.Parallel()

	// Test slice with multiple missing values filled by default:"unknown".
	type DocWithSliceDefaultUnknown struct {
		ID     []string `                                     documentid:""`
		Titles []string `cardinality:"3..5" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// With 1 value, should add 2 UnknownValueClaims.
	docs := []any{
		&DocWithSliceDefaultUnknown{
			ID:     []string{"test", "doc1"},
			Titles: []string{"Title 1"},
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.String, 1)
	require.Len(t, doc.Claims.UnknownValue, 2)
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

	// Nil pointer with min=1 and default:"none" should add NoValueClaim.
	docs := []any{
		&DocWithPointerDefault{
			ID:    []string{"test", "doc1"},
			Value: nil,
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.NoValue, 1)
}

func TestDocuments_SingleValueWithDefaultUnknown(t *testing.T) {
	t.Parallel()

	// Test single value field with default:"unknown".
	type DocWithSingleDefault struct {
		ID    []string `                                  documentid:""`
		Title string   `cardinality:"1" default:"unknown"               property:"TITLE"`
	}

	mnemonics := createMnemonics()

	// Empty string with min=1 and default:"unknown" should add UnknownValueClaim.
	docs := []any{
		&DocWithSingleDefault{
			ID:    []string{"test", "doc1"},
			Title: "",
		},
	}

	results, errE := transform.Documents(mnemonics, docs)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 1)

	doc := results[0]
	require.Len(t, doc.Claims.UnknownValue, 1)
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

	_, errE := transform.Documents(mnemonics, docs)
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

	_, errE := transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "cardinality tag cannot be used with value tag")
}

func TestDocuments_ValueFieldCannotHaveDefault(t *testing.T) {
	t.Parallel()

	// Test that value field cannot have default tag.
	type StructWithDefaultValue struct {
		Value string `default:"none" value:""`
	}

	type DocWithDefaultValue struct {
		ID    []string               `documentid:""`
		Field StructWithDefaultValue `              property:"FIELD"`
	}

	mnemonics := createMnemonics()
	mnemonics["FIELD"] = identifier.From("test", "FIELD")

	docs := []any{
		&DocWithDefaultValue{
			ID:    []string{"test", "doc1"},
			Field: StructWithDefaultValue{Value: "test"},
		},
	}

	_, errE := transform.Documents(mnemonics, docs)
	require.Error(t, errE)
	assert.EqualError(t, errE, "default tag cannot be used with value tag")
}
