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
		`URL for the file image for the arrow of the energy class, used by UI to show the arrow, in svg format`,
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
		`Supplier's model identifier`,
		[]string{`"identifier" claim type`},
	},
	{
		"supplier or trademark",
		nil,
		`Supplier's name or trademark`,
		[]string{`"string" claim type`},
	},

	/* The following properties are not currently mapped within the eprel_api file.
	// {
	// 	"allow eprel label generation",
	// 	nil,
	// 	`Set to true if the supplier chooses to use EPREL labels for this model. Set to false if the supplier has uploaded their own label.`,
	// 	[]string{`"Boolean" claim type`},
	// },
	// { // skipped - bool  -- skipped in eprel_api.go
	// 	"blocked",
	// 	nil,
	// 	`Set to true if the model was reported as containing inappropriate content by EPREL. It will not be visible to the public in any way.`,
	// 	[]string{`"Boolean" claim type`},
	// },
	// { // skipped - json blob  -- skipped in eprel_api.go
	// 	"contact details",
	// 	nil,
	// 	`None`,
	// 	[]string{`"None" claim type`},
	// },
	// { // skipped - json blob  -- skipped in eprel_api.go
	// 	"cycles",
	// 	nil,
	// 	`None`,
	// 	[]string{`"None" claim type`},
	// },
	// { // skipped - boolean  -- skipped in eprel_api.go
	// 	"eco label",
	// 	nil,
	// 	`Set to true if the model has an EU eco-label. False otherwise.`,
	// 	[]string{`"Boolean" claim type`},
	// },
	// { // skipped - need to add support for kWh unit  -- skipped in eprel_api.go
	// 	"energy annual wash",
	// 	nil,
	// 	`Annual Energy consumption washing and spinning (washing cycle)`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // skipped - need to add support for kWh unit  -- skipped in eprel_api.go
	// 	"energy annual wash and dry",
	// 	nil,
	// 	`Annual Energy consumption washing, spinning and drying  (complete operating cycle)`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // the time unit is epochs, is that an issue? -- skipped in eprel_api.go
	// 	"export date ts",
	// 	nil,
	// 	`Datetime of export of the model data to EPREL site, in epochs`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date is expressed here as an array of integers [YYYY, MM, DD]  -- skipped in eprel_api.go
	// 	"first publication date",
	// 	nil,
	// 	`Date the first version of a model is published and appears on the EPREL site. Expressed in the format [YYYY, MM, DD].`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // do I need to do anything special for epochs? -- skipped in eprel_api.go
	// 	"first publication date ts",
	// 	nil,
	// 	`Datetime the first version of a model is published and appears on the EPREL site, in epochs`,
	// 	[]string{`"Date" claim type`},
	// },
	// { // not existent in the API docs -- all values in data are null? Need to check for other categories what this data type is  -- skipped in eprel_api.go
	// 	"generated labels",
	// 	nil,
	// 	`None`,
	// 	[]string{`"None" claim type`},
	// },
	// { // measurement in decibels, do I need to add that?
	// 	"noise dry",
	// 	nil,
	// 	`Noise (Drying phase)`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // measurement in decibles, do I need to add that?
	// 	"noise spin",
	// 	nil,
	// 	`Noise (Spinning phase)`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // measurement in decibels, do I need to add that?
	// 	"noise wash",
	// 	nil,
	// 	`Noise (Washing phase)`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // date in the format of [YYYY, MM, DD]
	// 	"on market end date",
	// 	nil,
	// 	`Date the last model is placed on the market, in the format of [YYYY, MM, DD]. ` +
	// 		` Optional field, could be empty. It is used internally to verify if Basic filter ` +
	// 		`"Include models not placed on the market anymore" applies to the model.`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the format of epoch
	// 	"on market end date ts",
	// 	nil,
	// 	`Timestamp the last model is placed on the market, in epochs. ` +
	// 		`Optional field, could be empty. It is used internally to verify if Basic filter ` +
	// 		`"Include models not placed on the market anymore" applies to the model.`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the format of [YYYY, MM, DD]
	// 	"on market first start date",
	// 	nil,
	// 	`Date the first version of a model is placed on the market, in the format of [YYYY, MM, DD]. ` +
	// 		`It also marks the date the model becomes Published and appears on the EPREL site.`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the format of epoch
	// 	"on market first start date ts",
	// 	nil,
	// 	`Date the first version of a model is placed on the market, in epochs. It marks also the date the model becomes Published and appears on the EPREL site.`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the format of [YYYY, MM, DD]
	// 	"on market start date",
	// 	nil,
	// 	`Date the first model is placed on the market, in the format of [YYYY, MM, DD]. It marks also the date the model becomes Published and appears ont the EPREL site.`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the format of epoch
	// 	"on market start date ts",
	// 	nil,
	// 	`Date the first model is placed on the market, in epochs. It marks also the date the model becomes Published and appears ont the EPREL site.`,
	// 	[]string{`"Time" claim type`},
	// },
	// {
	// 	"org verification status",
	// 	nil,
	// 	`All the supplier organisations are obliged to pass a procedure of verification. The status of the verification is provided in this field.`,
	// 	[]string{`"string" claim type`},
	// },
	// { // skip, JSON blob
	// 	"organisation",
	// 	nil,
	// 	`Information about the organization that registered the model.`,
	// 	[]string{`"None" claim type`},
	// },
	// { // need to check if this field is non null for any of the other models so that I know how to type it
	// 	"other identifiers",
	// 	nil,
	// 	`Other model idenfitiers in the form of EAN codes. Can be multiple.`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// 	"imported on",
	// 	nil,
	// 	`Timestamp the data was imported, in epochs`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // skipped - boolean  -- skipped in eprel_api.go
	// 	"last version",
	// 	nil,
	// 	`This field will be always TRUE. Only last versions are published.`,
	// 	[]string{`"Boolean" claim type`},
	// },
	// { // This is in the form of a list of json blobs, how to type? Example: [{'country': 'AT', 'orderNumber': 1}, {'country': 'BE', 'orderNumber': 2}...]
	// 	"placement countries",
	// 	nil,
	// 	`None`,
	// 	[]string{`"None" claim type`},
	// },
	// { // could probably be aliased to the "Category" type that's already defined.
	// 	"product group",
	// 	nil,
	// 	`Product group name`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// 	"product model core id",
	// 	nil,
	// 	`Internal id of product model for EPREL.`,
	// 	[]string{`"identifier" claim type`},
	// },
	// { // date in the form of [YYYY, MM, DD]
	// 	"published on date",
	// 	nil,
	// 	`Date the data was published, in the form of [YYYY, MM, DD].`,
	// 	[]string{`"Time" claim type`},
	// },
	// { // date in the form of epoch
	// 	"published on date ts",
	// 	nil,
	// 	`Timestamp the data was published, in epochs.`,
	// 	[]string{`"Time" claim type`},
	// },
	// {
	// 	"registrant nature",
	// 	nil,
	// 	`The role with which the supplier organisation has registered the model. Roles can be: Manufacturer, Importer, or Authorised representative.`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// 	"status",
	// 	nil,
	// 	`Publication status. Only Published products are available in Public site`,
	// 	[]string{`"string" claim type`},
	// },
	// {

	// {
	// 	"trademark id",
	// 	nil,
	// 	`Supplier's name or trademark reference - ` +
	// 		`If Supplier's name or trademark is declared by reference in the supplier's organisation, the id of the reference is provided.`,
	// 	[]string{`"identifier" claim type`},
	// },
	// { // need to check if this is non-null for any of the other product categories
	// 	"trademark owner",
	// 	nil,
	// 	`The owner of the trademark.`,
	// 	[]string{`"string" claim type`},
	// },
	// { // need to check if this is non-null for any of the other product categories
	// 	"trademark verification status",
	// 	nil,
	// 	`The verification status of the trademark.`,
	// 	[]string{`"string" claim type`},
	// },
	// { // this is an array of strings, how should I type it?
	// 	"uploaded labels",
	// 	nil,
	// 	`A list of the filenames of the uploaded labels.`,
	// 	[]string{`"string" claim type`},
	// },
	// {
	// 	"version id",
	// 	nil,
	// 	`Internal identifier of the version number of the model.`,
	// 	[]string{`"identifier" claim type`},
	// },
	// {
	// 	"version number",
	// 	nil,
	// 	`When a model is published, the supplier can still make changes. To track these changes a new version number is created for each change.`,
	// 	[]string{`"string" claim type`},
	// },
	// { // Skip - boolean
	// 	"visible to united kingdom market surveillance authority",
	// 	nil,
	// 	`Compliance data visible to United Kingdom Market Surveillance Authority - ` +
	// 		`Optional flag to indicate if the product compliance information (technical documentation, ` +
	// 		` equivalents and ICSMS data) should be visible to the Market Surveillance Authority for the United Kingdom.
	// 	The handling of the flag is the following:
	// 	(1) For suppliers based at UK/Northern Ireland: ` +
	// 		`if the flag is omitted, it is considered as being "true" by default. If the flag is sent as "false", an error will occur.
	// 	(2) For suppliers based at an EU country: if the flag is omitted, it is considered as being "false" by default.`,
	// 	[]string{`"Boolean" claim type`},
	// },
	// { // Amount in liters
	// 	"water annual wash",
	// 	nil,
	// 	`Annual Water consumption washing and spinning (washing cycle), in liters`,
	// 	[]string{`"Amount" claim type`},
	// },
	// { // Amount in liters
	// 	"water annual wash and dry",
	// 	nil,
	// 	`Annual water consumption washing, spinning and drying  (complete operating cycle), in liters`,
	// 	[]string{`"Amount" claim type`},
	// },
	// END EPREL API properties
	*/
}

func init() { //nolint:gochecknoinits
	document.GenerateCoreProperties(productsProperties)
}
