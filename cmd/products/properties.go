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
		"A product category.",
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
		"A country in which a product is marketed.",
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

	// EPREL specific properties here.
	{
		"washer drier",
		[]string{"washer dryer", "washer-dryer", "washer-drier"},
		"A document is about a washer drier product.",
		[]string{`item`},
	},

	{
		"EPREL contact ID",
		nil,
		`A unique identifier for contact information.`,
		[]string{`"identifier" claim type`},
	},
	{
		"ecolabel registration number",
		nil,
		`The registration number of the EU eco-label.`,
		[]string{`"identifier" claim type`},
	},

	{
		"energy class",
		nil,
		`Letter of the energy efficiency class.`,
		[]string{`"string" claim type`},
	},
	{
		"energy class image",
		nil,
		`URL for the file image for the arrow of the energy class.`,
		[]string{`"file" claim type`},
	},
	{
		"energy class image with scale",
		nil,
		`URL for the file image for the arrow of the energy class, with scale.`,
		[]string{`"file" claim type`},
	},
	{
		"energy class range",
		nil,
		`Range of energy classes, represented by the energy class letter.`,
		[]string{`"string" claim type`},
	},
	{
		"energy label id",
		nil,
		`Internal identifier to EPREL that corresponds to the energy label.`,
		[]string{`"identifier" claim type`},
	},
	{
		"eprel registration number",
		nil,
		`Unique identifier determined at registration time by the EPREL system.`,
		[]string{`"identifier" claim type`},
	},
	{
		"implementing act",
		nil,
		`Delegated act number.`,
		[]string{`"string" claim type`},
	},
	{
		"model identifier",
		nil,
		`Supplier's model identifier.`,
		[]string{`"identifier" claim type`},
	},
	{
		"supplier or trademark",
		nil,
		`Supplier's name or trademark.`,
		[]string{`"string" claim type`},
	},
	{
		"energy annual wash",
		nil,
		`Annual Energy consumption washing and spinning (washing cycle)`,
		[]string{`"amount" claim type`},
	},
	{
		"energy annual wash and dry",
		nil,
		`Annual Energy consumption washing, spinning and drying  (complete operating cycle)`,
		[]string{`"amount" claim type`},
	},
	{
		"noise dry",
		nil,
		`Noise (Drying phase)`,
		[]string{`"Amount" claim type`},
	},
	{
		"noise spin",
		nil,
		`Noise (Spinning phase)`,
		[]string{`"Amount" claim type`},
	},
	{
		"noise wash",
		nil,
		`Noise (Washing phase)`,
		[]string{`"Amount" claim type`},
	},
	{
		"water annual wash",
		nil,
		`Annual Water consumption washing and spinning (washing cycle), in liters`,
		[]string{`"Amount" claim type`},
	},
	{
		"water annual wash and dry",
		nil,
		`Annual water consumption washing, spinning and drying  (complete operating cycle), in liters`,
		[]string{`"Amount" claim type`},
	},

	{
		"on market end date",
		nil,
		`Timestamp the last model is placed on the market, in epochs. ` +
			`Optional field, could be empty. It is used internally to verify if Basic filter ` +
			`"Include models not placed on the market anymore" applies to the model.`,
		[]string{`"Time" claim type`},
	},
	{
		"on market start date",
		nil,
		`Date the first model is placed on the market, in epochs. ` +
			`It marks also the date the model becomes Published and appears ont the EPREL site.` +
			`A model can be Published many times, due to changes introduced by supplier ` +
			`that creates a new version of the model, or due to technical modifications ` +
			`that makes necessary that a model is Published again and re-exported to Public ` +
			`site. One of these changes can be on the “On market start date” field. This + ` +
			`field stores the last on market start date that model had on last Publication. ` +
			`(Normally these 2 dates must be the same, changes in on market start date are not normal)`,
		[]string{`"Time" claim type`},
	},

	/* The following properties are not currently mapped within the eprel_api file.

	// { // need to check if this field is non null for any of the other models so that I know how to type it
	// 	"other identifiers",
	// 	nil,
	// 	`Other model idenfitiers in the form of EAN codes. Can be multiple.`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// 	"product model core id",
	// 	nil,
	// 	`Internal id of product model for EPREL.`,
	// 	[]string{`"identifier" claim type`},
	// },
	// {
	// 	"registrant nature",
	// 	nil,
	// 	`The role with which the supplier organisation has registered the model. Roles can be: Manufacturer, Importer, or Authorised representative.`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// { // this is an array of strings, how should I type it?
	// 	"uploaded labels",
	// 	nil,
	// 	`A list of the filenames of the uploaded labels.`,
	// 	[]string{`"string" claim type`},
	// },

	// END EPREL API properties
	*/
}

func init() { //nolint:gochecknoinits
	document.GenerateCoreProperties(productsProperties)
}
