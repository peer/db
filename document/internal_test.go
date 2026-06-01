package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"
)

func TestGetFallbackLanguages(t *testing.T) {
	t.Parallel()

	// With priority set.
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
		"pt": {},
	}

	// Language with explicit fallbacks.
	assert.Equal(t, []string{"sl", "und"}, getFallbackLanguages("en", priority))
	assert.Equal(t, []string{"en"}, getFallbackLanguages("sl", priority))

	// Language with empty fallback list: no fallback at all.
	assert.Empty(t, getFallbackLanguages("pt", priority))

	// Language not in priority: fallback to "und".
	assert.Equal(t, []string{"und"}, getFallbackLanguages("fr", priority))

	// "und" not in priority: no fallback (it's already undetermined).
	assert.Nil(t, getFallbackLanguages("und", priority))

	// With nil priority.
	assert.Equal(t, []string{"und"}, getFallbackLanguages("en", nil))
	assert.Nil(t, getFallbackLanguages("und", nil))
}

func TestIsRecognizedLanguage(t *testing.T) {
	t.Parallel()

	// "en" and "sl" are keys; "und" appears only as a fallback target.
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
	}

	// Keys are recognized.
	assert.True(t, isRecognizedLanguage("en", priority))
	assert.True(t, isRecognizedLanguage("sl", priority))

	// A fallback target that is not a key is still recognized.
	assert.True(t, isRecognizedLanguage("und", priority))

	// A language that is neither a key nor a fallback target is not recognized.
	assert.False(t, isRecognizedLanguage("pt", priority))

	// Nothing is recognized in an empty priority.
	assert.False(t, isRecognizedLanguage("en", nil))
	assert.False(t, isRecognizedLanguage("en", map[string][]string{}))
}

// TestGetClaimsOfType tests the generic GetClaimsOfType function.
func TestGetClaimsOfType(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	ct := &ClaimTypes{
		String: StringClaims{
			{
				CoreClaim: CoreClaim{ID: id1, Confidence: 0.5},
				Prop:      Reference{ID: prop},
				String:    "s1",
			},
			{
				CoreClaim: CoreClaim{ID: id2, Confidence: 1.0},
				Prop:      Reference{ID: prop},
				String:    "s2",
			},
		},
		None: NoneClaims{
			{
				CoreClaim: CoreClaim{ID: id3, Confidence: 0.75},
				Prop:      Reference{ID: prop},
			},
		},
	}

	// Get only StringClaims for prop.
	strings := getClaimsOfType[StringClaim](ct, prop)
	require.Len(t, strings, 2)
	assert.Equal(t, "s2", strings[0].String) // Higher confidence first.
	assert.Equal(t, "s1", strings[1].String)

	// Get NoneClaims for prop.
	nones := getClaimsOfType[NoneClaim](ct, prop)
	require.Len(t, nones, 1)

	// No AmountClaims for prop.
	amounts := getClaimsOfType[AmountClaim](ct, prop)
	assert.Empty(t, amounts)
}

// TestGetAllClaimsOfType tests GetAllClaimsOfType returns claims of the requested type sorted by decreasing confidence.
func TestGetAllClaimsOfType(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()

	doc := &D{}

	// Empty document returns nil.
	assert.Nil(t, getAllClaimsOfType[StringClaim](doc))

	// Add string claims on different properties with varying confidence.
	errE := doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 0.5},
		Prop:      Reference{ID: prop1},
		String:    "low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      Reference{ID: prop2},
		String:    "high",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 0.75},
		Prop:      Reference{ID: prop1},
		String:    "medium",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a different claim type to verify type filtering.
	errE = doc.Add(&HTMLClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      Reference{ID: prop1},
		HTML:      "<p>html</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// GetAllClaimsOfType returns only string claims, across all properties.
	strings := getAllClaimsOfType[StringClaim](doc)
	assert.Len(t, strings, 3)

	// Sorted by decreasing confidence.
	assert.Equal(t, "high", strings[0].String)
	assert.Equal(t, "medium", strings[1].String)
	assert.Equal(t, "low", strings[2].String)

	// GetAllClaimsOfType for HTML returns only the HTML claim.
	htmls := getAllClaimsOfType[HTMLClaim](doc)
	assert.Len(t, htmls, 1)
	assert.Equal(t, "<p>html</p>", htmls[0].HTML)

	// A type with no claims returns nil.
	assert.Nil(t, getAllClaimsOfType[ReferenceClaim](doc))
}

// TestGetAllClaimsOfTypeNilClaims tests GetAllClaimsOfType on a nil ClaimTypes.
func TestGetAllClaimsOfTypeNilClaims(t *testing.T) {
	t.Parallel()

	var claims *ClaimTypes
	assert.Nil(t, getAllClaimsOfType[StringClaim](claims))
}

// TestGetAllClaimsOfTypeWithConfidence tests GetAllClaimsOfTypeWithConfidence filters by confidence threshold.
func TestGetAllClaimsOfTypeWithConfidence(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()

	doc := &D{}

	// Empty document returns empty.
	assert.Empty(t, GetAllClaimsOfTypeWithConfidence[StringClaim](doc, LowConfidence))

	errE := doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 0.3},
		Prop:      Reference{ID: prop1},
		String:    "below-low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 0.5},
		Prop:      Reference{ID: prop2},
		String:    "at-low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 0.75},
		Prop:      Reference{ID: prop1},
		String:    "at-medium",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&StringClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      Reference{ID: prop2},
		String:    "at-high",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a different type to verify filtering.
	errE = doc.Add(&HTMLClaim{
		CoreClaim: CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      Reference{ID: prop1},
		HTML:      "<p>html</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// LowConfidence (0.5) filters out below-low (0.3).
	low := GetAllClaimsOfTypeWithConfidence[StringClaim](doc, LowConfidence)
	assert.Len(t, low, 3)
	assert.Equal(t, "at-high", low[0].String)
	assert.Equal(t, "at-medium", low[1].String)
	assert.Equal(t, "at-low", low[2].String)

	// MediumConfidence (0.75) filters out at-low and below-low.
	medium := GetAllClaimsOfTypeWithConfidence[StringClaim](doc, MediumConfidence)
	assert.Len(t, medium, 2)
	assert.Equal(t, "at-high", medium[0].String)
	assert.Equal(t, "at-medium", medium[1].String)

	// HighConfidence (1.0) keeps only at-high.
	high := GetAllClaimsOfTypeWithConfidence[StringClaim](doc, HighConfidence)
	assert.Len(t, high, 1)
	assert.Equal(t, "at-high", high[0].String)

	// Zero confidence defaults to LowConfidence.
	zero := GetAllClaimsOfTypeWithConfidence[StringClaim](doc, 0)
	assert.Equal(t, low, zero)
}

// TestGetAllClaimsOfTypeWithConfidenceNilClaims tests GetAllClaimsOfTypeWithConfidence on a nil ClaimTypes.
func TestGetAllClaimsOfTypeWithConfidenceNilClaims(t *testing.T) {
	t.Parallel()

	var claims *ClaimTypes
	assert.Empty(t, GetAllClaimsOfTypeWithConfidence[ReferenceClaim](claims, LowConfidence))
}
