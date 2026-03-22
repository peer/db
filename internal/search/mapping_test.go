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

	expectedTypes := []string{"id", "string", "html", "amount", "time", "ref", "rel", "has", "none", "unknown"}
	for _, ct := range expectedTypes {
		assert.Contains(t, claimProps, ct, "missing claim type: %s", ct)
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

	expectedAnalyzers := []string{
		"standard_html", "standard_string",
		"english_html", "english_string",
		"slovenian_html", "slovenian_string",
		"portuguese_html", "portuguese_string",
	}
	for _, a := range expectedAnalyzers {
		assert.Contains(t, analyzers, a, "missing analyzer: %s", a)
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

func TestMappingNestedRelation(t *testing.T) {
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

	// Check that rel claim type has nested rel field.
	relClaim, ok := claimProps["rel"].(map[string]any)
	require.True(t, ok)
	relProps, ok := relClaim["properties"].(map[string]any)
	require.True(t, ok)
	nestedRel, ok := relProps["rel"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "nested", nestedRel["type"])
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
