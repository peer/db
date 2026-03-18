package search

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"

	"gitlab.com/tozd/go/errors"
)

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

const nestedRel = `{
	"type": "nested",
	"properties": {
		"prop": ` + relationID + `,
		"propDisplay": ` + multiLanguageString + `,
		"to": ` + relationID + `,
		"toDisplay": ` + multiLanguageString + `
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
				// We do not use keyword normalizer here because display is just a number.
				`{
					"type": "keyword",
					"doc_values": false,
					"split_queries_on_whitespace": true
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
				// We do not use keyword normalizer here because display is just a number.
				`{
					"type": "keyword",
					"doc_values": false,
					"split_queries_on_whitespace": true
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
				multiLanguageString,
			},
			{
				"range",
				`{
					"type": "long_range"
				}`,
			},
			{
				"from",
				`{
					"type": "long"
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
					"type": "long"
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
		"ref",
		[]field{
			{
				"prop",
				relationID,
			},
			{
				"propDisplay",
				multiLanguageString,
			},
			{
				"iri",
				`{
					"type": "keyword",
					"doc_values": false,
					"split_queries_on_whitespace": true,
					"normalizer": "keyword_normalizer"
				}`,
			},
		},
	},
	{
		"rel",
		[]field{
			{
				"prop",
				relationID,
			},
			{
				"propDisplay",
				multiLanguageString,
			},
			{
				"to",
				relationID,
			},
			{
				"toDisplay",
				multiLanguageString,
			},
			{
				"rel",
				nestedRel,
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
				multiLanguageString,
			},
			{
				"rel",
				nestedRel,
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
