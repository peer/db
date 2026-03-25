package search

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"

	"gitlab.com/tozd/go/errors"
)

// SupportedLanguages is a set of supported languages in ElasticSearch mapping.
// Includes the undetermined language ("und") for content without a specific language.
var SupportedLanguages = map[string]bool{ //nolint:gochecknoglobals
	"en":  true,
	"sl":  true,
	"pt":  true,
	"und": true,
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

// We currently have display and naming fields which enable us to search and sort by them if we need that.
// We currently do not plan to group by them, so we do not have "toPath" or "toDisplayPath" fields.
const nestedRef = `{
	"type": "nested",
	"properties": {
		"prop": ` + relationID + `,
		"propDisplay": ` + propDisplay + `,
		"propNaming": ` + multiLanguageString + `,
		"to": ` + relationID + `,
		"toDisplay": ` + propDisplay + `,
		"toNaming": ` + multiLanguageString + `
	}
}`

// TODO: Generate automatically from the Document struct.
var claimTypes = []claimType{ //nolint:gochecknoglobals
	{
		"id",
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
				"value",
				`{
					"type": "keyword",
					"doc_values": false,
					"split_queries_on_whitespace": true,
					"normalizer": "id_normalizer"
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
			},
			{
				"string",
				multiLanguageString,
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
				propDisplay,
			},
			{
				"propNaming",
				multiLanguageString,
			},
			{
				"html",
				`{
					"properties": {
						"en": {
							"type": "text",
							"analyzer": "english_html"
						},
						"sl": {
							"type": "text",
							"analyzer": "slovenian_html"
						},
						"pt": {
							"type": "text",
							"analyzer": "portuguese_html"
						},
						"und": {
							"type": "text",
							"analyzer": "standard_html"
						}
					}
				}`,
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
		"link",
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
				"iri",
				`{
					"type": "keyword",
					"doc_values": false,
					"split_queries_on_whitespace": true,
					"normalizer": "iri_normalizer"
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
			{
				"ref",
				nestedRef,
			},
		},
	},
	{
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
			{
				"ref",
				nestedRef,
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
