package main

import (
	"gitlab.com/peerdb/peerdb/document"
)

//nolint:gochecknoglobals
var productsProperties = []struct {
	Name            string
	ExtraNames      []string
	DescriptionHTML string
	Types           []string
}{
	{
		"branded food",
		[]string{"food", "food product"},
		"A document is about a branded food product.",
		[]string{`item`},
	},
	{
		"FDCID",
		nil,
		`A FoodData Central identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"GTIN",
		nil,
		`A GTIN or UPC identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"data source",
		nil,
		"Data source of branded food product information.",
		[]string{`"string" claim type`},
	},
	{
		"category",
		nil,
		"A branded food product category.",
		[]string{`"string" claim type`},
	},
	{
		"publication date",
		nil,
		`When was the document about the food product published.`,
		[]string{`"time" claim type`},
	},
	{
		"available date",
		nil,
		`When has the food product been made available.`,
		[]string{`"time" claim type`},
	},
	{
		"modified date",
		nil,
		`When was the document about the food product last modified.`,
		[]string{`"time" claim type`},
	},
	{
		"discontinued date",
		nil,
		`When has the food product been discontinued.`,
		[]string{`"time" claim type`},
	},
	{
		"ingredient",
		nil,
		"An ingredient a food contains.",
		[]string{`"string" claim type`},
	},
	{
		"ingredients",
		nil,
		`A description of ingredients a branded food product contains.`,
		[]string{`"text" claim type`},
	},
	{
		"caffeine statement",
		nil,
		`Statement about caffeine in a branded food product.`,
		[]string{`"text" claim type`},
	},
	{
		"market country",
		nil,
		"A country in which a branded food product is marketed.",
		[]string{`"string" claim type`},
	},
	{
		"trade channel",
		nil,
		"A trade channel used for a branded food product.",
		[]string{`"string" claim type`},
	},
	{
		"brand owner",
		nil,
		`A brand owner of a branded food product.`,
		[]string{`"text" claim type`},
	},
	{
		"brand name",
		nil,
		`A brand name of a branded food product.`,
		[]string{`"text" claim type`},
	},
	{
		"subbrand name",
		nil,
		`A subbrand name of a branded food product.`,
		[]string{`"text" claim type`},
	},
	{
		"serving size",
		nil,
		`A suggested serving size of a branded food product.`,
		[]string{`"amount" claim type`},
	},
	{
		"serving size description",
		nil,
		`A description of suggested serving size of a branded food product.`,
		[]string{`"text" claim type`},
	},
	{
		"packaging size description",
		nil,
		`A description of packaging size of a branded food product.`,
		[]string{`"text" claim type`},
	},

	// FursEntry specific properties start here.
	{
		"VAT number",
		nil,
		`A company VAT number.`,
		[]string{`"identifier" claim type`},
	},
	{
		"Company registration number",
		nil,
		`A company registration number.`,
		[]string{`"identifier" claim type`},
	},
	{
		"SKD 2025",
		[]string{"Standard Classification of Activities 2025"},
		`National Standard Classification of Activities in Slovenia extending NACE Rev. 2.1..`,
		[]string{`"string" claim type`},
	},
	{
		"company",
		nil,
		"A document is about a company.",
		[]string{`item`},
	},
	{
		"address",
		nil,
		`An address.`,
		[]string{`"text" claim type`},
	},
	{
		"financial office",
		nil,
		`A financial office responsible for the company.`,
		[]string{`"string" claim type`},
	},
	{
		"country of incorporation",
		nil,
		`Country of incorporation.`,
		[]string{`"string" claim type`},
	},
	// Datakick specific properties start here.
	{
		"datakick id",
		nil,
		`A Datakick identifier.`,
		[]string{`"identifier" claim type`},
	},
	// PRS specific properties start here.
	{
		"HSEID",
		nil,
		`Slovenian unique ID of locations at the house number precision.`,
		[]string{`"identifier" claim type`},
	},
	{
		"company legal form",
		nil,
		`A legal form of a company.`,
		[]string{`"string" claim type`},
	},
	{
		"address street",
		nil,
		`A street - a part of an address.`,
		[]string{`"text" claim type`},
	},
	{
		"house number",
		nil,
		`A house number - a part of an address.`,
		[]string{`"text" claim type`},
	},
	{
		"house number addition",
		nil,
		`A house number addition - a part of an address.`,
		[]string{`"text" claim type`},
	},
	{
		"settlement",
		nil,
		`A settlement of an address.`,
		[]string{`"text" claim type`},
	},
	{
		"zip code",
		nil,
		`A zip code.`,
		[]string{`"text" claim type`},
	},
	{
		"postal office",
		nil,
		`A postal office responsible for an address.`,
		[]string{`"text" claim type`},
	},
	{
		"address country",
		nil,
		`Address country of a company.`,
		[]string{`"string" claim type`},
	},
}

func init() { //nolint:gochecknoinits
	document.GenerateCoreProperties(productsProperties)
}
