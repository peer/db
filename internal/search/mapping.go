package search

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"maps"
	"text/template"

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

const multiLanguageString = `{
	"properties": {
		"en": {
			"type": "text",
			"analyzer": "english_string"
		},
		"sl": {
			"type": "text",
			"analyzer": "slovenian_string"
		},
		"pt": {
			"type": "text",
			"analyzer": "portuguese_string"
		},
		"und": {
			"type": "text",
			"analyzer": "standard_string"
		}
	}
}`

// We use display paths so that we can sort documents based on display labels shown to users which represent hierarchies they are in.
// It works together with idPath, idPath groups results and then we sort by displayPath.
const displayPath = `{
	"properties": {
		"en": {
			"type": "keyword",
			"normalizer": "display_label_normalizer"
		},
		"sl": {
			"type": "keyword",
			"normalizer": "display_label_normalizer"
		},
		"pt": {
			"type": "keyword",
			"normalizer": "display_label_normalizer"
		},
		"und": {
			"type": "keyword",
			"normalizer": "display_label_normalizer"
		}
	}
}`

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
// So we use standard_string analyzer for all languages here and not language-specific analyzers.
// We use a multi-field to define also a keyword field which is better for sorting.
const propDisplay = `{
	"properties": {
		"en": {
			"type": "text",
			"analyzer": "standard_string",
			"fields": {
				"keyword": {
					"type": "keyword",
					"normalizer": "display_label_normalizer"
				}
			}
		},
		"sl": {
			"type": "text",
			"analyzer": "standard_string",
			"fields": {
				"keyword": {
					"type": "keyword",
					"normalizer": "display_label_normalizer"
				}
			}
		},
		"pt": {
			"type": "text",
			"analyzer": "standard_string",
			"fields": {
				"keyword": {
					"type": "keyword",
					"normalizer": "display_label_normalizer"
				}
			}
		},
		"und": {
			"type": "text",
			"analyzer": "standard_string",
			"fields": {
				"keyword": {
					"type": "keyword",
					"normalizer": "display_label_normalizer"
				}
			}
		}
	}
}`

// TODO: Generate automatically from the Document struct.
var claimTypes = []claimType{ //nolint:gochecknoglobals
	{
		"amount",
		[]field{
			{
				"prop",
				relationID,
			},
			{
				"propDisplay",
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
					"analyzer": "standard_string"
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
					"analyzer": "standard_string"
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
					"analyzer": "standard_string"
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
					"analyzer": "standard_string"
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
			},
			{
				"to",
				relationID,
			},
			{
				"toDisplay",
				propDisplay,
			},
			{
				"toNaming",
				multiLanguageString,
			},
			{
				"toPath",
				idPath,
			},
			{
				"toDisplayPath",
				displayPath,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
			},
			{
				"to",
				relationID,
			},
			{
				"toDisplay",
				propDisplay,
			},
			{
				"toNaming",
				multiLanguageString,
			},
			{
				"toPath",
				idPath,
			},
			{
				"toDisplayPath",
				displayPath,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
					"analyzer": "standard_string"
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
					"analyzer": "standard_string"
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
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
					"analyzer": "standard_string"
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
					"analyzer": "standard_string"
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
			},
		},
	},
}

// TODO: Generate index configuration automatically from document structs?

// Mapping generates PeerDB ElasticSearch mapping.
func Mapping() ([]byte, errors.E) {
	t, err := template.New("indexTemplate").Parse(mappingTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, claimTypes)
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
