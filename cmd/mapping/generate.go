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
	Definition string
}

type claimType struct {
	Name   string
	Fields []field
}

// TODO: Generate automatically from the Document struct.
var claimTypes = []claimType{ //nolint:gochecknoglobals
	{
		"id",
		[]field{
			{
				"prop",
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"id",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"iri",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"html",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"string",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"amount",
				`{
					"type": "double"
				}`,
			},
			{
				"unit",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"lower",
				`{
					"type": "double"
				}`,
			},
			{
				"upper",
				`{
					"type": "double"
				}`,
			},
			{
				"unit",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"enum",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"to",
				`{
					"properties": {
						"id": {
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"type",
				`{
					"type": "keyword"
				}`,
			},
			{
				"url",
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
				`{
					"properties": {
						"id": {
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
				`{
					"properties": {
						"id": {
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"timestamp",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"precision",
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
				`{
					"properties": {
						"id": {
							"type": "keyword"
						}
					}
				}`,
			},
			{
				"lower",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"upper",
				`{
					"type": "date",
					"format": "uuuu-MM-dd'T'HH:mm:ssX",
					"ignore_malformed": true
				}`,
			},
			{
				"precision",
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
