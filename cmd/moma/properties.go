package main

import (
	"gitlab.com/peerdb/peerdb/document"
)

//nolint:gochecknoglobals
var momaProperties = []struct {
	Name            string
	ExtraNames      []string
	DescriptionHTML string
	Types           []string
}{
	{
		"artist",
		nil,
		"A document is about an artist.",
		[]string{`item`},
	},
	{
		"artwork",
		nil,
		"A document is about an artwork.",
		[]string{`item`},
	},
	{
		"by artist",
		nil,
		"An artist who made an artwork.",
		[]string{`"relation" claim type`},
	},
	{
		"MoMA constituent id",
		nil,
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) constituent identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"MoMA constituent page",
		nil,
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) constituent page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"MoMA object id",
		nil,
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) object identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"MoMA object page",
		nil,
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) object page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"nationality",
		nil,
		`A nationality of an artist.`,
		[]string{`"string" claim type`},
	},
	{
		"gender",
		nil,
		`A gender of an artist.`,
		[]string{`"string" claim type`},
	},
	{
		"begin date",
		nil,
		`Begin date.`,
		[]string{`"time" claim type`},
	},
	{
		"end date",
		nil,
		`End date.`,
		[]string{`"time" claim type`},
	},
	{
		"Wikidata item id",
		nil,
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikidata item page",
		nil,
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"ULAN id",
		nil,
		`<a href="https://www.getty.edu/research/tools/vocabularies/ulan/index.html">Union List of Artist Names</a> identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"ULAN page",
		nil,
		`<a href="https://www.getty.edu/research/tools/vocabularies/ulan/index.html">Union List of Artist Names</a> page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"date",
		nil,
		`Date.`,
		[]string{`"string" claim type`},
	},
	{
		"medium",
		nil,
		`Medium.`,
		[]string{`"string" claim type`},
	},
	{
		"dimensions",
		nil,
		`Dimensions.`,
		[]string{`"string" claim type`},
	},
	{
		"credit",
		nil,
		`Credit.`,
		[]string{`"string" claim type`},
	},
	{
		"MoMA accession number",
		nil,
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) accession number.`,
		[]string{`"identifier" claim type`},
	},
	{
		"classification",
		nil,
		`Classification.`,
		[]string{`"string" claim type`},
	},
	{
		"department",
		nil,
		`Department.`,
		[]string{`"string" claim type`},
	},
	{
		"date acquired",
		nil,
		`Date acquired.`,
		[]string{`"time" claim type`},
	},
	{
		"cataloged",
		nil,
		`Cataloged.`,
		nil,
	},
	{
		"image",
		nil,
		`Image.`,
		[]string{`"file" claim type`},
	},
}

func init() { //nolint:gochecknoinits
	document.GenerateCoreProperties(momaProperties)
}
