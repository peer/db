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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
		"amount",
		[]field{
			{
				"prop",
				"_id",
				`{
					"properties": {
						"_id": {
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
							"type": "keyword"
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
}

func generate(config *Config) errors.E {
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

	f, err := os.Create(config.Output)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	_, err = io.Copy(f, &res)
	if err != nil {
		return errors.WithStack(err)
	}

	config.Logger.Info().Msg("mapping generated successfully")

	return nil
}
