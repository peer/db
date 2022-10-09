package wikipedia

import (
	"gitlab.com/peerdb/search"
)

var wikipediaProperties = []struct {
	Name            string
	DescriptionHTML string
	Is              []string
}{
	{
		"Wikidata reference",
		"A temporary group of multiple Wikidata reference statements as meta claims for later processing.",
		[]string{`"text" claim type`},
	},
	{
		"Wikidata property id",
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikidata property page",
		`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property page IRI.`,
		[]string{`"reference" claim type`},
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
		"English Wikipedia page title",
		`<a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> page title.`,
		[]string{`"identifier" claim type`},
	},
	{
		"English Wikipedia page",
		`Reference to <a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> page.`,
		[]string{`"reference" claim type`},
	},
	{
		"English Wikipedia page id",
		`<a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> page identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikimedia Commons page title",
		`<a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> page title.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikimedia Commons page",
		`Reference to <a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> page.`,
		[]string{`"reference" claim type`},
	},
	{
		"English Wikipedia file name",
		`Reference to <a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> file name.`,
		[]string{`"identifier" claim type`},
	},
	{
		"English Wikipedia file",
		`Reference to <a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> file.`,
		[]string{`"reference" claim type`},
	},
	{
		"Wikimedia Commons entity id",
		`<a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> ` +
			`<a href="https://commons.wikimedia.org/wiki/Commons:Structured_data">structured data entity identifier</a>.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikimedia Commons page id",
		`<a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> page identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikimedia Commons file name",
		`Reference to <a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> file name.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Wikimedia Commons file",
		`Reference to <a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> file.`,
		[]string{`"reference" claim type`},
	},
	{
		"Mediawiki media type",
		`See possible <a href="https://www.mediawiki.org/wiki/Manual:Image_table#img_media_type">Mediawiki media types</a>, lowercase.`,
		[]string{`"string" claim type`},
	},
	{
		"uses English Wikipedia template",
		`Entity uses a <a href="https://en.wikipedia.org/wiki/Help:Templates">English Wikipedia template</a> in the source of its article or description.`,
		[]string{`"relation" claim type`},
	},
	{
		"uses Wikimedia Commons template",
		`Entity uses a <a href="https://commons.wikimedia.org/wiki/Help:Templates">Wikimedia Commons template</a> in the source of its article or description.`,
		[]string{`"relation" claim type`},
	},
	{
		"in English Wikipedia category",
		`Entity is in <a href="https://en.wikipedia.org/wiki/Help:Category">English Wikipedia category</a>.`,
		[]string{`"relation" claim type`},
	},
	{
		"in Wikimedia Commons category",
		`Entity is in <a href="https://commons.wikimedia.org/wiki/Commons:Categories">Wikimedia Commons category</a>.`,
		[]string{`"relation" claim type`},
	},
}

func init() {
	search.GenerateCoreProperties(wikipediaProperties)
}
