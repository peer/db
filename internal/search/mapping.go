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

// SupportedLanguages is the set of supported languages in ElasticSearch mapping
// and is the default when a site does not specify its own LanguagePriority.
// Includes the undetermined language ("und") for content without a specific language.
//
// Sites that set LanguagePriority enable only the languages listed in its keys (plus "und").
var SupportedLanguages = map[string]bool{ //nolint:gochecknoglobals
	"en":  true,
	"sl":  true,
	"pt":  true,
	"und": true,
}

// codeToLingua maps our language codes to lingua languages for language detection. Only
// languages lingua supports appear here; "und" has no entry. Used to build the detector
// and to map detection results back to our codes.
var codeToLingua = map[string]lingua.Language{ //nolint:gochecknoglobals
	"en": lingua.English,
	"sl": lingua.Slovene,
	"pt": lingua.Portuguese,
}

// enabledLanguagesFromLanguagePriority returns the set of enabled languages and the per-language
// fallback chains to use for display label resolution, given its LanguagePriority configuration.
//
// When priority is non-empty, the enabled set is its keys plus "und", and the returned fallback
// chains are priority verbatim. When priority is nil/empty, the default SupportedLanguages set is
// enabled and each non-"und" language falls back to "und".
func enabledLanguagesFromLanguagePriority(priority map[string][]string) (map[string]bool, map[string][]string) {
	if len(priority) == 0 {
		out := make(map[string]bool, len(SupportedLanguages))
		maps.Copy(out, SupportedLanguages)
		fullPriority := make(map[string][]string, len(SupportedLanguages))
		for lang := range SupportedLanguages {
			if lang != document.UndeterminedLanguage {
				fullPriority[lang] = []string{document.UndeterminedLanguage}
			} else {
				fullPriority[lang] = nil
			}
		}
		return out, fullPriority
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

type claimType struct {
	Name   string
	Fields []field
}

// We do not use any normalizer here because we store only identifiers here.
// We do not need to trim it nor we want to lowercase it. We also do not worry about value length.
const relationID = `{
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
// (en_text, sl_text, ...). Used for naming-string fields.
func multiLanguageText(langs []string) string {
	return langProperties(langs, func(lang string) string {
		return fmt.Sprintf(`{"type":"text","analyzer":"%s_text"}`, lang)
	})
}

// We use display paths so that we can sort documents based on display labels shown to users which represent hierarchies they are in.
// It works together with idPath, idPath groups results and then we sort by displayPath.
func displayPath(langs []string) string {
	return langProperties(langs, func(string) string {
		return `{"type":"keyword","normalizer":"display_label_normalizer"}`
	})
}

// We use ID path to be able to group documents based on their ID paths which represent hierarchies they are in.
// It works together with displayPath, idPath groups results and then we sort by displayPath.
const idPath = `{
	"type": "keyword",
	"normalizer": "id_path_normalizer"
}`

// TODO: Maybe we can in the future track which languages were used to construct a display label and then put it into a suitable language-specific field.
//       For example, we could track in which language each template fragment is and then see if result is from fragments of only one language.
//       Also if display label comes directly from naming strings, then we also know the language of the display label.

// We use display labels for two purposes: to search over them and to sort by them.
// But they might contain text from different languages (they might be rendered from a template which
// pulled data from different languages), even if they are stored under a particular target language.
// So we use und_text analyzer for all languages here and not language-specific analyzers.
// We use a multi-field to define also a keyword field which is better for sorting.
func propDisplay(langs []string) string {
	return langProperties(langs, func(string) string {
		return `{"type":"text","analyzer":"und_text","fields":{"keyword":{"type":"keyword","normalizer":"display_label_normalizer"}}}`
	})
}

// indexPrefixes accelerates trailing-prefix (analyze_wildcard) queries: ElasticSearch indexes
// term prefixes into a managed prefix sub-index, so prefix queries skip the term-dictionary
// walk. It is added only to the fields the analyze_wildcard search clauses target.
const indexPrefixes = `"index_prefixes":{"min_chars":1,"max_chars":8}`

// displayProperties builds the top-level "display" field: a per-language text field with
// the und_text analyzer (display labels may mix languages) and an exact sub-field for
// diacritic-preserved matching. The main field carries index_prefixes because the display
// search clause routes wildcards to it.
func displayProperties(langs []string) string {
	return langProperties(langs, func(string) string {
		return `{"type":"text","analyzer":"und_text",` + indexPrefixes + `,"fields":{"exact":{"type":"text","analyzer":"exact_text"}}}`
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

// TODO: Generate automatically from the Document struct.
func buildClaimTypes(langs []string) []claimType { //nolint:maintidx
	return []claimType{
		{
			"id",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					// Identifier values have no language, so they use the und_text analyzer, matching
					// the "und" bucket of the top-level text field they are also folded into.
					"value",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			"string",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"string",
					multiLanguageText(langs),
				},
			},
		},
		{
			"html",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					// HTML is converted to plain text in Go before indexing, so it uses the
					// per-language text analyzers like string, not an HTML-stripping analyzer.
					"html",
					multiLanguageText(langs),
				},
			},
		},
		{
			"amount",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"unit",
					relationID,
				},
				{
					"range",
					`{
						"type": "double_range"
					}`,
				},
				{
					"from",
					//nolint:goconst
					`{
						"type": "double"
					}`,
				},
				{
					"fromDisplay",
					// We do not use "propDisplay" here. We do not need a multi-field here because we only search
					// over display here and we do not sort by it. There are no languages either.
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
				{
					"to",
					`{
						"type": "double"
					}`,
				},
				{
					"toDisplay",
					// We do not use "propDisplay" here. We do not need a multi-field here because we only search
					// over display here and we do not sort by it. There are no languages either.
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			"time",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"range",
					`{
						"type": "double_range"
					}`,
				},
				{
					"from",
					`{
						"type": "double"
					}`,
				},
				{
					"fromDisplay",
					// We do not use "propDisplay" here. We do not need a multi-field here because we only search
					// over display here and we do not sort by it. There are no languages either.
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
				{
					"to",
					`{
						"type": "double"
					}`,
				},
				{
					"toDisplay",
					// We do not use "propDisplay" here. We do not need a multi-field here because we only search
					// over display here and we do not sort by it. There are no languages either.
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			"link",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					// IRIs have no language, so they use the und_text analyzer, matching the "und" bucket
					// of the top-level text field they are also folded into.
					"iri",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			"ref",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"to",
					relationID,
				},
				{
					"toDisplay",
					propDisplay(langs),
				},
				{
					"toNaming",
					multiLanguageText(langs),
				},
				{
					"toPath",
					idPath,
				},
				{
					"toDisplayPath",
					displayPath(langs),
				},
				{
					"isLeaf",
					boolean,
				},
			},
		},
		{
			// claims.has only contains simple has claims (those with no
			// sub-claims). Sub-claims of a has claim are flattened into the
			// matching claims.sub* records with parentTo=__HAS__.
			"has",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
			},
		},
		{
			"none",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
			},
		},
		{
			"unknown",
			[]field{
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
			},
		},
		{
			// Synthetic claim type indexing nested reference sub-claims from
			// parent claims (ref, has, none, unknown).
			"subRef",
			[]field{
				{
					"parentProp",
					relationID,
				},
				{
					"parentTo",
					relationID,
				},
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"to",
					relationID,
				},
				{
					"toDisplay",
					propDisplay(langs),
				},
				{
					"toNaming",
					multiLanguageText(langs),
				},
				{
					"toPath",
					idPath,
				},
				{
					"toDisplayPath",
					displayPath(langs),
				},
				{
					"isLeaf",
					boolean,
				},
			},
		},
		{
			// Synthetic claim type indexing nested amount sub-claims (including
			// AmountInterval sources mapped to a range) from parent claims.
			"subAmount",
			[]field{
				{
					"parentProp",
					relationID,
				},
				{
					"parentTo",
					relationID,
				},
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"unit",
					relationID,
				},
				{
					"range",
					`{
						"type": "double_range"
					}`,
				},
				{
					"from",
					`{
						"type": "double"
					}`,
				},
				{
					"fromDisplay",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
				{
					"to",
					`{
						"type": "double"
					}`,
				},
				{
					"toDisplay",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			// Synthetic claim type indexing nested time sub-claims (including
			// TimeInterval sources mapped to a range) from parent claims.
			"subTime",
			[]field{
				{
					"parentProp",
					relationID,
				},
				{
					"parentTo",
					relationID,
				},
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
				{
					"range",
					`{
						"type": "double_range"
					}`,
				},
				{
					"from",
					`{
						"type": "double"
					}`,
				},
				{
					"fromDisplay",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
				{
					"to",
					`{
						"type": "double"
					}`,
				},
				{
					"toDisplay",
					`{
						"type": "text",
						"analyzer": "und_text"
					}`,
				},
			},
		},
		{
			// Synthetic claim type indexing simple has sub-claims (those with no
			// sub-claims of their own) from parent claims.
			"subHas",
			[]field{
				{
					"parentProp",
					relationID,
				},
				{
					"parentTo",
					relationID,
				},
				{
					"prop",
					relationID,
				},
				{
					"propDisplay",
					propDisplay(langs),
				},
				{
					"propNaming",
					multiLanguageText(langs),
				},
			},
		},
	}
}

// TODO: Generate index configuration automatically from document structs?

// EnabledLanguages returns the sorted set of languages a site indexes, derived from its
// LanguagePriority (its keys plus "und") or the default SupportedLanguages when unset.
func EnabledLanguages(languagePriority map[string][]string) []string {
	enabled, _ := enabledLanguagesFromLanguagePriority(languagePriority)
	return slices.Sorted(maps.Keys(enabled))
}

// mappingData is the data passed to the mapping template. Display and Text hold the
// prebuilt per-language top-level property blocks; ClaimTypes drives the claims block.
type mappingData struct {
	Display    string
	Text       string
	ClaimTypes []claimType
}

// Mapping generates PeerDB ElasticSearch mapping for the languages a site enables, derived
// from its LanguagePriority (nil yields the default all-language mapping).
func Mapping(languagePriority map[string][]string) ([]byte, errors.E) {
	langs := EnabledLanguages(languagePriority)

	t, err := template.New("indexTemplate").Parse(mappingTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, mappingData{
		Display:    displayProperties(langs),
		Text:       textProperties(langs),
		ClaimTypes: buildClaimTypes(langs),
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
