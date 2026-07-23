package search

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/pemistahl/lingua-go"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/document"
)

// SupportedLanguages is the set of languages for which the ElasticSearch mapping has analyzers.
// Includes the undetermined language ("und") for content without a specific language.
//
// Sites that set LanguagePriority enable only the languages listed in its keys (plus "und").
// A site that sets no LanguagePriority enables only DefaultEnabledLanguage (plus "und").
var SupportedLanguages = map[string]bool{ //nolint:gochecknoglobals
	"en":  true,
	"sl":  true,
	"pt":  true,
	"und": true,
}

// DefaultEnabledLanguage is the only language enabled when a site configures no LanguagePriority
// ("und" is its fallback). The frontend mirrors this constant so the empty-priority default is
// identical on both sides.
const DefaultEnabledLanguage = "en"

// codeToLingua maps our language codes to lingua languages for language detection. Only
// languages lingua supports appear here; "und" has no entry. Used to build the detector
// and to map detection results back to our codes.
var codeToLingua = map[string]lingua.Language{ //nolint:gochecknoglobals
	"en": lingua.English,
	"sl": lingua.Slovene,
	"pt": lingua.Portuguese,
}

// EnabledLanguagesFromLanguagePriority returns the set of enabled languages and the per-language
// fallback chains to use for display label resolution, given its LanguagePriority configuration.
//
// When priority is non-empty, the enabled set is its keys plus "und", and the returned fallback
// chains are priority verbatim. When priority is nil/empty, only DefaultEnabledLanguage is enabled
// (plus "und"), with "und" as its fallback.
func EnabledLanguagesFromLanguagePriority(priority map[string][]string) (map[string]bool, map[string][]string) {
	if len(priority) == 0 {
		enabled := map[string]bool{DefaultEnabledLanguage: true, document.UndeterminedLanguage: true}
		fallback := map[string][]string{DefaultEnabledLanguage: {document.UndeterminedLanguage}}
		return enabled, fallback
	}
	out := make(map[string]bool, len(priority)+1)
	for lang := range priority {
		out[lang] = true
	}
	out[document.UndeterminedLanguage] = true
	return out, priority
}

//go:embed mapping.tmpl
var mappingTemplate string

type field struct {
	Name       string
	Definition string
}

// collection describes one nested claims collection in the mapping: its name under
// "claims" (and under a parent record's "sub") and its field definitions.
type collection struct {
	Name   string
	Fields []field
}

// We do not use any normalizer here because we store only identifiers here.
// We do not need to trim it nor we want to lowercase it. We also do not worry about value length.
const relationID = `{
	"type": "keyword"
}`

// keywordField is a plain keyword mapping for small controlled vocabularies (e.g., the claimType
// discriminator values).
const keywordField = `{
	"type": "keyword"
}`

const boolean = `{
	"type": "boolean"
}`

// langProperties builds a JSON object with a property per enabled language, using perLang
// to render each language's definition.
func langProperties(langs []string, perLang func(lang string) string) string {
	props := make([]string, 0, len(langs))
	for _, lang := range langs {
		props = append(props, fmt.Sprintf("%q:%s", lang, perLang(lang)))
	}
	return `{"properties":{` + strings.Join(props, ",") + `}}`
}

// multiLanguageText builds a per-language text property using each language's own analyzer
// (en_text, sl_text, ...). Used for naming-string fields. It mirrors the top-level text field's prefix setup
// (see textProperties): the stemmed main field does full-word recall, and an unstemmed sub-field (und_text)
// with index_prefixes carries analyze_wildcard prefix-as-you-type. "und" is already unstemmed, so it takes
// index_prefixes on the main field instead of a sub-field.
func multiLanguageText(langs []string) string {
	return langProperties(langs, func(lang string) string {
		if lang == document.UndeterminedLanguage {
			return `{"type":"text","analyzer":"und_text",` + indexPrefixes + `}`
		}

		return fmt.Sprintf(`{"type":"text","analyzer":"%s_text","fields":{"unstemmed":{"type":"text","analyzer":"und_text",`+indexPrefixes+`}}}`, lang)
	})
}

// sortKey builds the per-language keyword (sort_key_normalizer) mapping shared by the sort-key fields.
// The normalizer folds the label for case/diacritic-insensitive ordering while leaving any hex id half intact.
func sortKey(langs []string) string {
	return langProperties(langs, func(string) string {
		return `{"type":"keyword","normalizer":"sort_key_normalizer"}`
	})
}

// idPath is the keyword mapping for a value's ID hierarchy paths.
const idPath = `{
	"type": "keyword",
	"normalizer": "id_path_normalizer"
}`

// TODO: Maybe we can in the future track which languages were used to construct a display label and then put it into a suitable language-specific field.
//       For example, we could track in which language each template fragment is and then see if result is from fragments of only one language.
//       Also if display label comes directly from naming strings, then we also know the language of the display label.

// indexPrefixes accelerates trailing-prefix (analyze_wildcard) queries: ElasticSearch indexes
// term prefixes into a managed prefix sub-index, so prefix queries skip the term-dictionary
// walk. It is added to the text and display-label fields that prefix search clauses target.
const indexPrefixes = `"index_prefixes":{"min_chars":1,"max_chars":8}`

// Display labels might contain text from different languages (they may be rendered from a template
// that pulled data from different languages), even when stored under a particular target language,
// so we use the und_text analyzer for all languages here and not language-specific analyzers.
//
// displayLabelProperty is the per-language mapping for a claim's propDisplay/toDisplay fields: the
// und_text main field carries index_prefixes for trailing-prefix (analyze_wildcard) queries and an
// "exact" sub-field (exact_text) for diacritic-preserved matching.
const displayLabelProperty = `{"type":"text","analyzer":"und_text",` + indexPrefixes + `,"fields":{"exact":{"type":"text","analyzer":"exact_text"}}}`

// valueBoundDisplay is the mapping for amount or time claim's fromDisplay/toDisplay fields, the
// formatted display of its value bounds. It is a flat und_text field: the bounds carry no language and
// are not sorted on, so there are no per-language buckets, and the content is numeric/temporal text with
// no diacritics, so no "exact" sub-field is needed. index_prefixes accelerates the trailing-prefix
// (analyze_wildcard) queries the filter pane runs over it.
const valueBoundDisplay = `{"type":"text","analyzer":"und_text",` + indexPrefixes + `}`

// propDisplay builds the per-language display-label mapping for a claim's propDisplay and toDisplay fields.
func propDisplay(langs []string) string {
	return langProperties(langs, func(string) string {
		return displayLabelProperty
	})
}

// displayProperties builds the top-level "display" field: a per-language und_text text field with
// index_prefixes and an "exact" sub-field, but no "keyword" sub-field. "display" is multi-valued (it
// also holds ancestor hierarchy-path labels for recall), so sorting by the display label uses the
// single-valued displaySort field instead.
func displayProperties(langs []string) string {
	return langProperties(langs, func(string) string {
		return `{"type":"text","analyzer":"und_text",` + indexPrefixes + `,"fields":{"exact":{"type":"text","analyzer":"exact_text"}}}`
	})
}

// displaySortProperties builds the top-level "displaySort" field: per enabled language (except und) a
// single keyword (sort_key_normalizer) holding the document's primary resolved display label, then
// SortKeySeparator, then the hex-encoded document id, so results sort by the label shown to the user with
// the document's own id as a stable tiebreaker. It is single-valued (no ancestor path labels). The und
// (language-neutral) bucket is omitted: results sort only by the session's language (never und), and that
// language's displaySort already carries the und value through the fallback chain, so a displaySort.und
// would never be read.
func displaySortProperties(langs []string) string {
	nonUnd := make([]string, 0, len(langs))
	for _, lang := range langs {
		if lang == document.UndeterminedLanguage {
			continue
		}
		nonUnd = append(nonUnd, lang)
	}
	return langProperties(nonUnd, func(string) string {
		return `{"type":"keyword","normalizer":"sort_key_normalizer"}`
	})
}

// textProperties builds the top-level "text" field: each language uses its own analyzer with
// an exact sub-field, and non-und languages also get an unstemmed (und_text) sub-field.
// "und" needs no unstemmed sub-field because its main analyzer already is the unstemmed one.
// index_prefixes is added to the analyze_wildcard targets: text.und (main) and the per-language
// .unstemmed sub-field; the stemmed main field and .exact sub-field are not wildcard targets.
func textProperties(langs []string) string {
	return langProperties(langs, func(lang string) string {
		if lang == document.UndeterminedLanguage {
			return `{"type":"text","analyzer":"und_text",` + indexPrefixes + `,"fields":{"exact":{"type":"text","analyzer":"exact_text"}}}`
		}
		//nolint:lll
		return fmt.Sprintf(`{"type":"text","analyzer":"%s_text","fields":{"unstemmed":{"type":"text","analyzer":"und_text",`+indexPrefixes+`},"exact":{"type":"text","analyzer":"exact_text"}}}`, lang)
	})
}

// propIdentityFields returns the field definitions every claims collection shares: the property id
// and its denormalized display, sort-key, and naming labels.
func propIdentityFields(langs []string) []field {
	return []field{
		{"prop", relationID},
		{"propDisplay", propDisplay(langs)},
		{"propSortKey", sortKey(langs)},
		{"propNaming", multiLanguageText(langs)},
	}
}

// relCollectionFields returns the field definitions of the rel collection: the four
// target-or-nothing claim types (ref, has, none, unknown) discriminated by claimType,
// with the To* fields present only on ref records.
func relCollectionFields(langs []string) []field {
	fields := []field{
		{"claimType", keywordField},
	}
	fields = append(fields, propIdentityFields(langs)...)
	return append(fields,
		field{"to", relationID},
		field{"toDisplay", propDisplay(langs)},
		field{"toSortKey", sortKey(langs)},
		field{"toNaming", multiLanguageText(langs)},
		field{"toPath", idPath},
		// toParent is a value id (a value's immediate parents), so it uses the same relationID mapping as "to".
		field{"toParent", relationID},
		field{"toPathSortKey", sortKey(langs)},
		field{"isLeaf", boolean},
	)
}

// valueBoundFields returns the range and boundary field definitions shared by the amount and time
// collections.
func valueBoundFields() []field {
	return []field{
		{"range", `{
			"type": "double_range"
		}`},
		{"from", `{
			"type": "double"
		}`},
		{"fromDisplay", valueBoundDisplay},
		{"to", `{
			"type": "double"
		}`},
		{"toDisplay", valueBoundDisplay},
	}
}

// amountCollectionFields returns the field definitions of the amount collection.
func amountCollectionFields(langs []string) []field {
	fields := propIdentityFields(langs)
	fields = append(fields, field{"unit", relationID})
	return append(fields, valueBoundFields()...)
}

// timeCollectionFields returns the field definitions of the time collection.
func timeCollectionFields(langs []string) []field {
	return append(propIdentityFields(langs), valueBoundFields()...)
}

// identifierCollectionFields returns the field definitions of the id collection.
func identifierCollectionFields(langs []string) []field {
	return append(propIdentityFields(langs), field{
		// Identifier values have no language, so they use the und_text analyzer, matching
		// the "und" bucket of the top-level text field they are also folded into.
		"value",
		`{
			"type": "text",
			"analyzer": "und_text"
		}`,
	})
}

// stringCollectionFields returns the field definitions of the string collection.
func stringCollectionFields(langs []string) []field {
	return append(propIdentityFields(langs), field{"string", multiLanguageText(langs)})
}

// htmlCollectionFields returns the field definitions of the html collection.
func htmlCollectionFields(langs []string) []field {
	return append(propIdentityFields(langs), field{
		// HTML is converted to plain text in Go before indexing, so it uses the
		// per-language text analyzers like string, not an HTML-stripping analyzer.
		"html",
		multiLanguageText(langs),
	})
}

// linkCollectionFields returns the field definitions of the link collection.
func linkCollectionFields(langs []string) []field {
	return append(propIdentityFields(langs), field{
		// IRIs have no language, so they use the und_text analyzer, matching the "und" bucket
		// of the top-level text field they are also folded into.
		"iri",
		`{
			"type": "text",
			"analyzer": "und_text"
		}`,
	})
}

// collectionFieldBuilders maps each claims collection name to its field builder, in the order the
// collections appear in the mapping.
func collectionFieldBuilders() []struct {
	Name   string
	Fields func(langs []string) []field
} {
	return []struct {
		Name   string
		Fields func(langs []string) []field
	}{
		{"rel", relCollectionFields},
		{"amount", amountCollectionFields},
		{"time", timeCollectionFields},
		{"id", identifierCollectionFields},
		{"string", stringCollectionFields},
		{"html", htmlCollectionFields},
		{"link", linkCollectionFields},
	}
}

// ParentCollections returns the names of the claims collections in mapping order. Each of them
// hosts a "sub" container with the same collection shapes nested inside, so query builders that
// must consider every possible parent of a sub-claim iterate this list.
func ParentCollections() []string {
	builders := collectionFieldBuilders()
	out := make([]string, 0, len(builders))
	for _, b := range builders {
		out = append(out, b.Name)
	}
	return out
}

// fieldsProperties renders a field list into a JSON properties object ({"name":definition,...}).
func fieldsProperties(fields []field) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%q:%s", f.Name, f.Definition))
	}
	return `{` + strings.Join(parts, ",") + `}`
}

// subFieldDefinition builds the definition of a parent record's "sub" field: an object holding one
// nested collection per claims collection shape, so a record's sub-claims are nested one level
// inside it (ElasticSearch multi-level nested). Sub collections have the same fields as their
// top-level counterparts and no "sub" field of their own: the mapping indexes a single sub level.
func subFieldDefinition(langs []string) string {
	parts := make([]string, 0, len(collectionFieldBuilders()))
	for _, b := range collectionFieldBuilders() {
		parts = append(parts, fmt.Sprintf(`%q:{"type":"nested","properties":%s}`, b.Name, fieldsProperties(b.Fields(langs))))
	}
	return `{"properties":{` + strings.Join(parts, ",") + `}}`
}

// buildCollections builds the claims collections for the mapping: each of the seven collection
// shapes at the top level, every one carrying a "sub" field with all seven shapes nested inside.
func buildCollections(langs []string) []collection {
	sub := subFieldDefinition(langs)
	builders := collectionFieldBuilders()
	out := make([]collection, 0, len(builders))
	for _, b := range builders {
		fields := b.Fields(langs)
		fields = append(fields, field{"sub", sub})
		out = append(out, collection{Name: b.Name, Fields: fields})
	}
	return out
}

// TODO: Generate index configuration automatically from document structs?

// EnabledLanguages returns the sorted set of languages a site indexes, derived from its
// LanguagePriority (its keys plus "und"), or only DefaultEnabledLanguage (plus "und") when unset.
func EnabledLanguages(languagePriority map[string][]string) []string {
	enabled, _ := EnabledLanguagesFromLanguagePriority(languagePriority)
	return slices.Sorted(maps.Keys(enabled))
}

// ResolveLanguage resolves a session's language against a site's LanguagePriority. An empty language
// becomes the site default; a non-empty language must be an enabled language (a LanguagePriority key,
// so "und" is never accepted). It returns the resolved language, or an error when the language is not
// enabled (callers wrap it with their own validation error).
//
// When languagePriority is empty the site enables only DefaultEnabledLanguage (matching
// enabledLanguagesFromLanguagePriority), so an empty language resolves to DefaultEnabledLanguage and
// only DefaultEnabledLanguage is accepted.
func ResolveLanguage(language string, languagePriority map[string][]string, defaultLanguage string) (string, errors.E) {
	if len(languagePriority) == 0 {
		if language == "" || language == DefaultEnabledLanguage {
			return DefaultEnabledLanguage, nil
		}
		errE := errors.New("language is not enabled")
		errors.Details(errE)["language"] = language
		return "", errE
	}
	if language == "" {
		return defaultLanguage, nil
	}
	if _, ok := languagePriority[language]; !ok {
		errE := errors.New("language is not enabled")
		errors.Details(errE)["language"] = language
		return "", errE
	}
	return language, nil
}

// MaxInnerResultWindow is the index.max_inner_result_window setting: ElasticSearch's cap on the from+size of
// a top_hits (or inner_hits) aggregation. The grouped search collects up to this many documents per leaf
// group via top_hits, so the search package's per-group size (groupTopK) must not exceed it. ElasticSearch's
// default is 100.
const MaxInnerResultWindow = 1000

// NestedFieldsLimit is the index.mapping.nested_fields.limit setting. The mapping defines 7 top-level
// nested collections plus 7 x 7 sub collections (56 nested types), above ElasticSearch's default of 50,
// so the limit is raised with headroom.
const NestedFieldsLimit = 128

// TotalFieldsLimit is the index.mapping.total_fields.limit setting. The sub mirror multiplies the
// per-collection field definitions (each with per-language sub-fields) well past ElasticSearch's default
// of 1000, so the limit is raised with headroom. Unused sparse fields cost cluster state only, not
// per-document storage.
const TotalFieldsLimit = 6000

// mappingData is the data passed to the mapping template. Display and Text hold the
// prebuilt per-language top-level property blocks; Collections drives the claims block.
type mappingData struct {
	Display              string
	DisplaySort          string
	Text                 string
	Collections          []collection
	MaxInnerResultWindow int
	NestedFieldsLimit    int
	TotalFieldsLimit     int
}

// Mapping generates PeerDB ElasticSearch mapping for the languages a site enables, derived
// from its LanguagePriority (nil yields a mapping covering only DefaultEnabledLanguage plus
// the undetermined language).
func Mapping(languagePriority map[string][]string) ([]byte, errors.E) {
	langs := EnabledLanguages(languagePriority)

	t, err := template.New("indexTemplate").Parse(mappingTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, mappingData{
		Display:              displayProperties(langs),
		DisplaySort:          displaySortProperties(langs),
		Text:                 textProperties(langs),
		Collections:          buildCollections(langs),
		MaxInnerResultWindow: MaxInnerResultWindow,
		NestedFieldsLimit:    NestedFieldsLimit,
		TotalFieldsLimit:     TotalFieldsLimit,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var res bytes.Buffer
	data := b.Bytes()
	err = json.Indent(&res, data, "", "  ")
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["json"] = string(data)
		return nil, errE
	}
	res.WriteString("\n")

	return res.Bytes(), nil
}
