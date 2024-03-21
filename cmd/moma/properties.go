package main

import (
	"gitlab.com/peerdb/peerdb"
)

//nolint:gochecknoglobals
var momaProperties = []struct {
	Name            string
	DescriptionHTML string
	Is              []string
}{
	{
		"artist",
		"The item is an artist.",
		[]string{`item`},
	},
	{
		"artwork",
		"The item is an artwork.",
		[]string{`item`},
	},
	{
		"by artist",
		"The artwork is by the artist.",
		[]string{`"relation" claim type`},
	},
	{
		"MoMA constituent id",
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) constituent identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"MoMA constituent page",
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) constituent page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"MoMA object id",
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) object identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"MoMA object page",
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) object page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"nationality",
		`Nationality.`,
		[]string{`"string" claim type`},
	},
	{
		"gender",
		`Gender.`,
		[]string{`"string" claim type`},
	},
	{
		"begin date",
		`Begin date.`,
		[]string{`"time" claim type`},
	},
	{
		"end date",
		`End date.`,
		[]string{`"time" claim type`},
	},
	{
		"Wikidata item id",
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikidata item page",
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"ULAN id",
		`<a href="https://www.getty.edu/research/tools/vocabularies/ulan/index.html">Union List of Artist Names</a> identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"ULAN page",
		`<a href="https://www.getty.edu/research/tools/vocabularies/ulan/index.html">Union List of Artist Names</a> page IRI.`,
		[]string{`"reference" claim type`},
	},
	{
		"date",
		`Date.`,
		[]string{`"string" claim type`},
	},
	{
		"medium",
		`Medium.`,
		[]string{`"string" claim type`},
	},
	{
		"dimensions",
		`Dimensions.`,
		[]string{`"string" claim type`},
	},
	{
		"credit",
		`Credit.`,
		[]string{`"string" claim type`},
	},
	{
		"MoMA accession number",
		`<a href="https://www.moma.org/">The Museum of Modern Art</a> (MoMA) accession number.`,
		[]string{`"identifier" claim type`},
	},
	{
		"classification",
		`Classification.`,
		[]string{`"string" claim type`},
	},
	{
		"department",
		`Department.`,
		[]string{`"string" claim type`},
	},
	{
		"date acquired",
		`Date acquired.`,
		[]string{`"time" claim type`},
	},
	{
		"cataloged",
		`Cataloged.`,
		nil,
	},
	{
		"image",
		`Image.`,
		[]string{`"file" claim type`},
	},
}

func init() { //nolint:gochecknoinits
	peerdb.GenerateCoreProperties(momaProperties)
}
