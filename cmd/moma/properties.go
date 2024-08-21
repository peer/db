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
		[]string{"artworks", "work of art", "piece of art", "art work", "art object", "art piece", "artistic work", "art", "art works", "artistic works"},
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
		[]string{"citizenship"},
		`A nationality of an artist.`,
		[]string{`"string" claim type`},
	},
	{
		"gender",
		[]string{"sex"},
		`A gender of an artist.`,
		[]string{`"string" claim type`},
	},
	{
		"date of birth",
		[]string{"begin date", "birth date", "year of birth", "born", "time of birth", "DOB", "birthday", "birthdate", "birth", "b."},
		`When was an artist born.`,
		[]string{`"time" claim type`},
	},
	{
		"date of death",
		[]string{"end date", "death date", "year of death", "death", "time of death", "DOD", "died on"},
		`When did an artist die.`,
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
		"date created",
		nil,
		`A date when was an artwork created.`,
		[]string{`"string" claim type`},
	},
	{
		"medium",
		[]string{"art material", "material", "art media", "art medium", "artistic material", "artistic media", "artistic medium", "media", "medium", "art materials", "arts materials", "crafting material", "art tool", "art equipment", "art supply", "art supplies", "Art & Crafting Materials", "coloring supply", "oloring supplies"},
		`A medium an artwork has been made on or with.`,
		[]string{`"string" claim type`},
	},
	{
		"dimensions",
		nil,
		`Dimensions of an artwork.`,
		[]string{`"string" claim type`},
	},
	{
		"credit",
		[]string{"acknowledgement"},
		`From where or how was an artwork acquired.`,
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
		[]string{"classifying", "grouping", "class", "group"},
		`A classification of an artwork.`,
		[]string{`"string" claim type`},
	},
	{
		"department",
		[]string{"division", "unit", "branch"},
		`A department of an artwork.`,
		[]string{`"string" claim type`},
	},
	{
		"date acquired",
		[]string{"time acquired"},
		`A date when was an artwork acquired.`,
		[]string{`"time" claim type`},
	},
	{
		"cataloged",
		[]string{"recorded"},
		`A label that an artwork has been cataloged.`,
		nil,
	},
	{
		"image",
		[]string{"photo", "picture"},
		`An image of an artwork.`,
		[]string{`"file" claim type`},
	},
}

func init() { //nolint:gochecknoinits
	document.GenerateCoreProperties(momaProperties)
}
