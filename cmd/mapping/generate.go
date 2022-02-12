package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"text/template"

	"gitlab.com/tozd/go/errors"
)

//go:embed index.tmpl
var indexTemplate string

type field struct {
	Name       string
	EmbeddedID string
	Definition string
}

type claimType struct {
	Name   string
	Fields []field
}

// TODO: Generate automatically from the Document struct.
var claimTypes = []claimType{
	{
		"id",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"id",
				"",
				`{
					"type": "keyword",
					"normalizer": "id_normalizer"
				}`,
			},
		},
	},
	{
		"ref",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"iri",
				"",
				`{
					"type": "keyword",
					"doc_values": false
				}`,
			},
		},
	},
	{
		"text",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"html",
				"",
				`{
					"properties": {
						"en": {
							"type": "text",
							"analyzer": "english_html"
						}
					}
				}`,
			},
		},
	},
	{
		"string",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"string",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"label",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
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
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"amount",
				"",
				`{
					"type": "double"
				}`,
			},
			{
				"unit",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"amountRange",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"lower",
				"",
				`{
					"type": "double"
				}`,
			},
			{
				"upper",
				"",
				`{
					"type": "double"
				}`,
			},
			{
				"unit",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"enum",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"enum",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"rel",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"to",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
		},
	},
	{
		"file",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"type",
				"",
				`{
					"type": "keyword"
				}`,
			},
			{
				"url",
				"",
				`{
					"type": "keyword",
					"doc_values": false
				}`,
			},
		},
	},
	{
		"none",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
		},
	},
	{
		"unknown",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
		},
	},
	{
		"time",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"timestamp",
				"",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"precision",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"timeRange",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"lower",
				"",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"upper",
				"",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"precision",
				"",
				`{
					"type": "keyword"
				}`,
			},
		},
	},
	{
		"is",
		[]field{
			{
				"to",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
		},
	},
	{
		"list",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"el",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword",
							"doc_values": false
						}
					}
				}`,
			},
			{
				"list",
				"",
				`{
					"type": "keyword",
					"doc_values": false
				}`,
			},
		},
	},
}

func generate() errors.E {
	t, err := template.New("indexTemplate").Parse(indexTemplate)
	if err != nil {
		return errors.WithStack(err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, claimTypes)
	if err != nil {
		return errors.WithStack(err)
	}

	var res bytes.Buffer
	err = json.Indent(&res, b.Bytes(), "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}
	res.WriteString("\n")

	_, err = io.Copy(os.Stdout, &res)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
