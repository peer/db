package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

func TestMapping(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, data)

	// Should be valid JSON.
	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should have settings and mappings top-level keys.
	assert.Contains(t, parsed, "settings")
	assert.Contains(t, parsed, "mappings")
}

func TestMappingContainsClaimTypes(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)
	claims, ok := properties["claims"].(map[string]any)
	require.True(t, ok)
	claimProps, ok := claims["properties"].(map[string]any)
	require.True(t, ok)

	// Textual claim types (id, string, html, link) no longer have per-claim ES
	// records; their content is folded into the top-level "text" field instead.
	expectedTypes := []string{"amount", "time", "ref", "has", "none", "unknown"}
	for _, ct := range expectedTypes {
		assert.Contains(t, claimProps, ct, "missing claim type: %s", ct)
	}
	for _, ct := range []string{"id", "string", "html", "link"} {
		assert.NotContains(t, claimProps, ct, "unexpected per-claim type left in mapping: %s", ct)
	}

	// Top-level text field with per-language sub-properties.
	text, ok := properties["text"].(map[string]any)
	require.True(t, ok, "missing top-level text field")
	textProps, ok := text["properties"].(map[string]any)
	require.True(t, ok)
	for lang := range internalSearch.SupportedLanguages {
		assert.Contains(t, textProps, lang, "missing text.%s sub-property", lang)
	}

	// Each text.<lang> is a multi-field. The stemmed languages have both an
	// .unstemmed sub-field (standard_string, no stemming, for analyzed-wildcard
	// routing) and an .exact sub-field (exact_string, diacritic-preserved, for
	// quote_field_suffix routing). text.und only needs .exact because its main
	// analyzer is already standard_string.
	for lang := range internalSearch.SupportedLanguages {
		entry, entryOK := textProps[lang].(map[string]any)
		require.True(t, entryOK, "missing text.%s entry", lang)
		fields, fieldsOK := entry["fields"].(map[string]any)
		require.True(t, fieldsOK, "missing text.%s.fields multi-field block", lang)
		assert.Contains(t, fields, "exact", "missing text.%s.exact sub-field", lang)
		if lang == "und" {
			assert.NotContains(t, fields, "unstemmed", "text.und should not have .unstemmed (would be identical to main analyzer)")
			continue
		}
		assert.Contains(t, fields, "unstemmed", "missing text.%s.unstemmed sub-field", lang)
	}
}

func TestMappingContainsAnalyzers(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	settings, ok := parsed["settings"].(map[string]any)
	require.True(t, ok)
	analysis, ok := settings["analysis"].(map[string]any)
	require.True(t, ok)
	analyzers, ok := analysis["analyzer"].(map[string]any)
	require.True(t, ok)

	// *_html analyzers have been removed: HTML stripping happens in Go before
	// the value reaches ES, and the top-level text field uses the *_string
	// analyzers like everything else.
	expectedAnalyzers := []string{
		"standard_string", "english_string", "slovenian_string", "portuguese_string",
		"exact_string",
	}
	for _, a := range expectedAnalyzers {
		assert.Contains(t, analyzers, a, "missing analyzer: %s", a)
	}
	for _, a := range []string{"standard_html", "english_html", "slovenian_html", "portuguese_html"} {
		assert.NotContains(t, analyzers, a, "unexpected analyzer left: %s", a)
	}
}

func TestMappingIsIndented(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	str := string(data)
	// Should end with a newline.
	assert.Equal(t, byte('\n'), str[len(str)-1])
	// Should contain indentation.
	assert.Contains(t, str, "  ")
}

func TestMappingNestedReference(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)
	claims, ok := properties["claims"].(map[string]any)
	require.True(t, ok)
	claimProps, ok := claims["properties"].(map[string]any)
	require.True(t, ok)

	// Check that claims.subRef exists as a nested field.
	subRef, ok := claimProps["subRef"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "nested", subRef["type"])
	subRefProps, ok := subRef["properties"].(map[string]any)
	require.True(t, ok)
	_, ok = subRefProps["parentProp"]
	assert.True(t, ok)
	_, ok = subRefProps["parentTo"]
	assert.True(t, ok)
	_, ok = subRefProps["prop"]
	assert.True(t, ok)
	_, ok = subRefProps["to"]
	assert.True(t, ok)

	// Check claims.subAmount, claims.subTime, claims.subHas are nested fields
	// with parentProp / parentTo / prop indexed for cross-filter matching.
	for _, name := range []string{"subAmount", "subTime", "subHas"} {
		sub, ok := claimProps[name].(map[string]any)
		require.True(t, ok, "missing claims.%s", name)
		assert.Equal(t, "nested", sub["type"], "claims.%s should be a nested field", name)
		subProps, ok := sub["properties"].(map[string]any)
		require.True(t, ok)
		for _, f := range []string{"parentProp", "parentTo", "prop"} {
			_, ok = subProps[f]
			assert.True(t, ok, "missing claims.%s.%s", name, f)
		}
	}

	// subAmount and subTime also expose a range field for numeric filtering.
	for _, name := range []string{"subAmount", "subTime"} {
		sub := claimProps[name].(map[string]any)       //nolint:errcheck,forcetypeassert
		subProps := sub["properties"].(map[string]any) //nolint:errcheck,forcetypeassert
		rangeField, ok := subProps["range"].(map[string]any)
		require.True(t, ok, "missing claims.%s.range", name)
		assert.Equal(t, "double_range", rangeField["type"], "claims.%s.range should be a double_range", name)
	}
}

func TestMappingDynamicDisabled(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, mappings["dynamic"])
}

func TestMappingSourceDisabled(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping()
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	source, ok := mappings["_source"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, source["enabled"])
}
