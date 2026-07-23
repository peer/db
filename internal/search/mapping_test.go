package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

func TestMapping(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
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

	// Build an all-language priority from SupportedLanguages (minus the undetermined language) so the
	// per-language assertions below cover every supported language; an empty priority enables only the
	// default language.
	priority := map[string][]string{}
	for lang := range internalSearch.SupportedLanguages {
		if lang != document.UndeterminedLanguage {
			priority[lang] = []string{document.UndeterminedLanguage}
		}
	}
	data, errE := internalSearch.Mapping(priority)
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

	// The claims collections: rel holds the four target-or-nothing claim types (ref, has,
	// none, unknown) in one claimType-discriminated collection; the textual collections (id, string,
	// html, link) have per-claim nested ES records for structured per-property queries, in
	// addition to being folded into the top-level "text" field.
	expectedTypes := []string{"rel", "amount", "time", "id", "string", "html", "link"}
	for _, ct := range expectedTypes {
		assert.Contains(t, claimProps, ct, "missing claims collection: %s", ct)
	}
	// The four claim types share claims.rel, and sub-claims nest inside their parent record,
	// so neither per-claim-type collections nor flat sub collections exist.
	for _, ct := range []string{"ref", "has", "none", "unknown", "subRef", "subAmount", "subTime", "subHas"} {
		assert.NotContains(t, claimProps, ct, "unexpected claims collection left: %s", ct)
	}

	// id and link have no language, so their value/iri fields use the und_text analyzer, matching
	// the "und" bucket of the top-level text field they are also folded into.
	for _, tf := range []struct{ claimType, field string }{{"id", "value"}, {"link", "iri"}} {
		ct, ctOK := claimProps[tf.claimType].(map[string]any)
		require.True(t, ctOK, "missing claim type: %s", tf.claimType)
		props, propsOK := ct["properties"].(map[string]any)
		require.True(t, propsOK)
		f, fOK := props[tf.field].(map[string]any)
		require.True(t, fOK, "missing %s.%s field", tf.claimType, tf.field)
		assert.Equal(t, "und_text", f["analyzer"], "%s.%s should use und_text analyzer", tf.claimType, tf.field)
	}

	// string and html are per-language, each language using its own text analyzer (en -> en_text).
	// html is converted to text in Go before indexing, so it uses the same text analyzers as string,
	// not an HTML-stripping analyzer.
	for _, tf := range []struct{ claimType, field string }{{"string", "string"}, {"html", "html"}} {
		ct, ctOK := claimProps[tf.claimType].(map[string]any)
		require.True(t, ctOK, "missing claim type: %s", tf.claimType)
		ctProps, ctPropsOK := ct["properties"].(map[string]any)
		require.True(t, ctPropsOK)
		langField, langFieldOK := ctProps[tf.field].(map[string]any)
		require.True(t, langFieldOK, "missing %s.%s field", tf.claimType, tf.field)
		langProps, langOK := langField["properties"].(map[string]any)
		require.True(t, langOK, "%s.%s should be a per-language object", tf.claimType, tf.field)
		en, enOK := langProps["en"].(map[string]any)
		require.True(t, enOK, "missing %s.%s.en", tf.claimType, tf.field)
		assert.Equal(t, "en_text", en["analyzer"], "%s.%s.en should use en_text analyzer", tf.claimType, tf.field)
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
	// .unstemmed sub-field (und_text, no stemming, for analyzed-wildcard
	// routing) and an .exact sub-field (exact_text, diacritic-preserved, for
	// quote_field_suffix routing). text.und only needs .exact because its main
	// analyzer is already und_text.
	for lang := range internalSearch.SupportedLanguages {
		entry, entryOK := textProps[lang].(map[string]any)
		require.True(t, entryOK, "missing text.%s entry", lang)
		fields, fieldsOK := entry["fields"].(map[string]any)
		require.True(t, fieldsOK, "missing text.%s.fields multi-field block", lang)
		assert.Contains(t, fields, "exact", "missing text.%s.exact sub-field", lang)
		if lang == document.UndeterminedLanguage {
			assert.NotContains(t, fields, "unstemmed", "text.und should not have .unstemmed (would be identical to main analyzer)")
			continue
		}
		assert.Contains(t, fields, "unstemmed", "missing text.%s.unstemmed sub-field", lang)
	}
}

// TestMappingPerSiteLanguages verifies that Mapping emits per-language field blocks only
// for the languages a site enables (its LanguagePriority keys plus "und"), while the
// analyzer definitions stay hardcoded for all supported languages.
func TestMappingPerSiteLanguages(t *testing.T) {
	t.Parallel()

	// Site enables "en" only; "sl" is a fallback target, so it is not indexed.
	data, errE := internalSearch.Mapping(map[string][]string{"en": {"sl", "und"}})
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)

	for _, fieldName := range []string{"text", "display"} {
		field, fieldOK := properties[fieldName].(map[string]any)
		require.True(t, fieldOK, "missing %s field", fieldName)
		props, propsOK := field["properties"].(map[string]any)
		require.True(t, propsOK)
		assert.Contains(t, props, "en", "%s should have enabled language en", fieldName)
		assert.Contains(t, props, "und", "%s should always have und", fieldName)
		assert.NotContains(t, props, "sl", "%s should not have fallback-only language sl", fieldName)
		assert.NotContains(t, props, "pt", "%s should not have non-enabled language pt", fieldName)
	}

	// Analyzers stay hardcoded for all supported languages even when unused.
	settings, ok := parsed["settings"].(map[string]any)
	require.True(t, ok)
	analysis, ok := settings["analysis"].(map[string]any)
	require.True(t, ok)
	analyzers, ok := analysis["analyzer"].(map[string]any)
	require.True(t, ok)
	for _, a := range []string{"en_text", "sl_text", "pt_text", "und_text", "exact_text"} {
		assert.Contains(t, analyzers, a, "missing analyzer %s", a)
	}
}

func TestMappingContainsAnalyzers(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
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
	// the value reaches ES, and the top-level text field uses the *_text
	// analyzers like everything else.
	expectedAnalyzers := []string{
		"und_text", "en_text", "sl_text", "pt_text",
		"exact_text",
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

	data, errE := internalSearch.Mapping(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	str := string(data)
	// Should end with a newline.
	assert.Equal(t, byte('\n'), str[len(str)-1])
	// Should contain indentation.
	assert.Contains(t, str, "  ")
}

// TestMappingNestedCollections verifies the nested layout of the claims block: seven top-level
// nested collections, each carrying a "sub" object with the same seven collection shapes nested
// inside (a single sub level), the claimType discriminator and the ref target fields on rel, and
// the range fields on the amount and time collections, mirrored into every sub container.
func TestMappingNestedCollections(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
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

	// ParentCollections lists the claims collections in mapping order, and the mapping has
	// exactly those collections.
	parents := internalSearch.ParentCollections()
	assert.Equal(t, []string{"rel", "amount", "time", "id", "string", "html", "link"}, parents)
	assert.Len(t, claimProps, len(parents))

	// requireCollection asserts that props[name] is a nested type and returns its properties.
	requireCollection := func(props map[string]any, name, path string) map[string]any {
		coll, collOK := props[name].(map[string]any)
		require.True(t, collOK, "missing %s", path)
		assert.Equal(t, "nested", coll["type"], "%s should be a nested field", path)
		collProps, collPropsOK := coll["properties"].(map[string]any)
		require.True(t, collPropsOK, "%s has no properties", path)
		return collProps
	}

	// checkCollectionShape asserts the per-collection fields shared by a collection's top-level
	// and sub variants: the property identity fields on every collection, the claimType
	// discriminator and the ref target fields on rel, and the range field on amount and time.
	checkCollectionShape := func(collProps map[string]any, name, path string) {
		for _, f := range []string{"prop", "propDisplay", "propSortKey", "propNaming"} {
			assert.Contains(t, collProps, f, "missing %s.%s", path, f)
		}
		if name == "rel" {
			claimType, claimTypeOK := collProps["claimType"].(map[string]any)
			require.True(t, claimTypeOK, "missing %s.claimType", path)
			assert.Equal(t, "keyword", claimType["type"], "%s.claimType should be a keyword", path)
			for _, f := range []string{"to", "toDisplay", "toSortKey", "toNaming", "toPath", "toParent", "toPathSortKey", "isLeaf"} {
				assert.Contains(t, collProps, f, "missing %s.%s", path, f)
			}
		}
		if name == "amount" || name == "time" {
			rangeField, rangeOK := collProps["range"].(map[string]any)
			require.True(t, rangeOK, "missing %s.range", path)
			assert.Equal(t, "double_range", rangeField["type"], "%s.range should be a double_range", path)
		}
	}

	for _, name := range parents {
		path := "claims." + name
		collProps := requireCollection(claimProps, name, path)
		checkCollectionShape(collProps, name, path)

		// Every top-level collection hosts a "sub" object with all seven collection shapes
		// nested inside.
		sub, subOK := collProps["sub"].(map[string]any)
		require.True(t, subOK, "missing %s.sub", path)
		subProps, subPropsOK := sub["properties"].(map[string]any)
		require.True(t, subPropsOK, "%s.sub has no properties", path)
		assert.Len(t, subProps, len(parents))
		for _, subName := range parents {
			subPath := path + ".sub." + subName
			subCollProps := requireCollection(subProps, subName, subPath)
			checkCollectionShape(subCollProps, subName, subPath)
			// The mapping indexes a single sub level: sub collections have no "sub" of their own.
			assert.NotContains(t, subCollProps, "sub", "%s must not have its own sub", subPath)
		}
	}
}

// TestMappingSettingsLimits verifies the index settings raised for the nested sub-claims layout:
// the nested type count (7 top-level collections plus 7 x 7 sub collections, 56 nested types) is
// over ElasticSearch's default nested_fields limit of 50, and the sub mirror multiplies the field
// count past the default total_fields limit of 1000, so both limits are raised with headroom.
func TestMappingSettingsLimits(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	settings, ok := parsed["settings"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, float64(internalSearch.MaxInnerResultWindow), settings["max_inner_result_window"]) //nolint:testifylint

	mapping, ok := settings["mapping"].(map[string]any)
	require.True(t, ok, "missing settings.mapping")
	nestedFields, ok := mapping["nested_fields"].(map[string]any)
	require.True(t, ok, "missing settings.mapping.nested_fields")
	assert.Equal(t, float64(internalSearch.NestedFieldsLimit), nestedFields["limit"]) //nolint:testifylint
	totalFields, ok := mapping["total_fields"].(map[string]any)
	require.True(t, ok, "missing settings.mapping.total_fields")
	assert.Equal(t, float64(internalSearch.TotalFieldsLimit), totalFields["limit"]) //nolint:testifylint

	// The constants pin the documented limits, and the mapping's 56 nested types fit under the
	// nested_fields limit.
	assert.Equal(t, 128, internalSearch.NestedFieldsLimit)
	assert.Equal(t, 6000, internalSearch.TotalFieldsLimit)
	nestedTypes := len(internalSearch.ParentCollections()) * (1 + len(internalSearch.ParentCollections()))
	assert.Equal(t, 56, nestedTypes)
	assert.LessOrEqual(t, nestedTypes, internalSearch.NestedFieldsLimit)
}

func TestMappingDynamicStrict(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	// strict makes ElasticSearch reject documents with fields not in the mapping,
	// catching schema drift instead of silently dropping data.
	assert.Equal(t, "strict", mappings["dynamic"])
}

func TestMappingTopLevelTime(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)

	// The top-level "time" field holds the document's earliest time and is the
	// same type as claims.time.from.
	timeField, ok := properties["time"].(map[string]any)
	require.True(t, ok, "missing top-level time field")
	assert.Equal(t, "double", timeField["type"])
}

func TestMappingCountFields(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	var parsed map[string]any
	errE = x.UnmarshalWithoutUnknownFields(data, &parsed)
	require.NoError(t, errE, "% -+#.1v", errE)

	mappings, ok := parsed["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)

	// references, claims and score are integer fields nested under counts.
	counts, ok := properties["counts"].(map[string]any)
	require.True(t, ok, "missing counts field")
	countsProperties, ok := counts["properties"].(map[string]any)
	require.True(t, ok, "counts has no properties")
	for _, name := range []string{"references", "claims", "score"} {
		field, fieldOK := countsProperties[name].(map[string]any)
		require.True(t, fieldOK, "missing counts.%s field", name)
		assert.Equal(t, "integer", field["type"])
	}
}

func TestMappingSourceDisabled(t *testing.T) {
	t.Parallel()

	data, errE := internalSearch.Mapping(nil)
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

func TestResolveLanguage(t *testing.T) {
	t.Parallel()

	priority := map[string][]string{"en": {"sl", "und"}, "sl": {"en", "und"}}

	for _, tc := range []struct {
		name      string
		language  string
		priority  map[string][]string
		def       string
		want      string
		wantError bool
	}{
		{"empty priority, empty language defaults to en", "", nil, "", internalSearch.DefaultEnabledLanguage, false},
		{"empty priority, en accepted", "en", nil, "", internalSearch.DefaultEnabledLanguage, false},
		{"empty priority, sl rejected", "sl", nil, "", "", true},
		{"empty priority, und rejected", "und", nil, "", "", true},
		{"priority, empty language uses default", "", priority, "sl", "sl", false},
		{"priority, en accepted", "en", priority, "sl", "en", false},
		{"priority, sl accepted", "sl", priority, "sl", "sl", false},
		{"priority, pt rejected (not a key)", "pt", priority, "sl", "", true},
		{"priority, und rejected", "und", priority, "sl", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, errE := internalSearch.ResolveLanguage(tc.language, tc.priority, tc.def)
			if tc.wantError {
				require.Error(t, errE)
				return
			}
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tc.want, got)
		})
	}
}
