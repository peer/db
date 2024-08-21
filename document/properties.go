package document

import (
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

//nolint:gochecknoglobals
var (
	// TODO: Determine automatically.
	// "none" and "unknown" are not listed here because they can take any property.
	claimTypes = []string{
		"identifier",
		"reference",
		"text",
		"string",
		"amount",
		"amount range",
		"relation",
		"file",
		"time",
		"time range",
	}

	builtinProperties = []struct {
		Name            string
		ExtraNames      []string
		DescriptionHTML string
		Types           []string
	}{
		{
			"type",
			[]string{"is", "kind", "form", "category"},
			"Type of a document.",
			[]string{`"relation" claim type`},
		},
		{
			"label",
			[]string{"tag"},
			"A document has a label.",
			[]string{`"relation" claim type`},
		},
		{
			"property",
			[]string{"attribute", "characteristic"},
			"A document describes a property.",
			nil,
		},
		{
			"item",
			nil,
			"A document describes an item.",
			nil,
		},
		{
			"file",
			[]string{"electronic file", "file on a computer"},
			"A document describes a file.",
			nil,
		},
		{
			"file URL",
			nil,
			"URL of a file.",
			[]string{`"reference" claim type`},
		},
		{
			"preview URL",
			nil,
			"URL of a preview.",
			[]string{`"reference" claim type`},
		},
		{
			"unit",
			[]string{"unit of measurement", "measurement unit", "unit of measure"},
			"Unit associated with an amount.",
			nil,
		},
		{
			"claim type",
			nil,
			"A property maps to a supported claim type.",
			nil,
		},
		{
			"description",
			nil,
			"Textual description.",
			[]string{`"text" claim type`},
		},
		{
			"article",
			nil,
			"A longer textual article.",
			[]string{`"text" claim type`},
		},
		{
			"has article",
			nil,
			"A document has an article.",
			nil,
		},
		{
			"name",
			[]string{"label"},
			"A name of a document.",
			[]string{`"text" claim type`},
		},
		{
			"list",
			nil,
			"A list has an unique ID, even a list with just one element. All elements of a list share this ID.",
			[]string{`"identifier" claim type`},
		},
		{
			"order",
			nil,
			"Order of an element inside its list. Smaller numbers are closer to the beginning of a list.",
			[]string{`"amount" claim type`},
		},
		{
			// TODO: How to define a property (type of relation) between parent and child?
			"child",
			nil,
			"List elements might have other lists as children. This is an ID of the child list.",
			[]string{`"identifier" claim type`},
		},
		{
			"page count",
			[]string{"number of pages"},
			"Number of pages an object or file has.",
			[]string{`"amount" claim type`},
		},
		{
			"file size",
			nil,
			"A size a file has.",
			[]string{`"amount" claim type`},
		},
		{
			"length",
			nil,
			"A length of an object.",
			[]string{`"amount" claim type`},
		},
		{
			"width",
			[]string{"breadth"},
			"A width of an object or a file.",
			[]string{`"amount" claim type`},
		},
		{
			"height",
			[]string{"height difference"},
			"A height of an object or a file.",
			[]string{`"amount" claim type`},
		},
		{
			"depth",
			nil,
			"A depth of an object.",
			[]string{`"amount" claim type`},
		},
		{
			"weight",
			[]string{"gravitational weight"},
			"A weight of an object.",
			[]string{`"amount" claim type`},
		},
		{
			"diameter",
			[]string{"diametre"},
			"A diameter of an object.",
			[]string{`"amount" claim type`},
		},
		{
			"circumference",
			[]string{"perimeter of a circle or ellipse"},
			"A circumference of an object.",
			[]string{`"amount" claim type`},
		},
		{
			"duration",
			[]string{"length of time", "time", "length", "elapsed time", "amount of time", "period"},
			"A duration a recording or file has.",
			[]string{`"amount" claim type`},
		},
		{
			"media type",
			[]string{"MIME type", "Internet media type", "IMT", "content type"},
			"Media (MIME) type of a file.",
			[]string{`"string" claim type`},
		},
	}

	nameSpaceCoreProperties = uuid.MustParse("34cd10b4-5731-46b8-a6dd-45444680ca62")

	// TODO: Use sync.Map?

	// CoreProperties is a map from a core property ID to a document describing it.
	CoreProperties = map[identifier.Identifier]D{}
)

func GetCorePropertyReference(mnemonic string) Reference {
	property, ok := CoreProperties[GetCorePropertyID(mnemonic)]
	if !ok {
		panic(errors.Errorf(`core property for mnemonic "%s" cannot be found`, mnemonic))
	}
	return property.Reference()
}

func getMnemonic(data string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(data), " ", "_"), `"`, "")
}

func GetID(namespace uuid.UUID, args ...interface{}) identifier.Identifier {
	res := namespace
	for _, arg := range args {
		res = uuid.NewSHA1(res, []byte(fmt.Sprint(arg)))
	}
	return identifier.FromUUID(res)
}

func GetCorePropertyID(mnemonic string) identifier.Identifier {
	return GetID(nameSpaceCoreProperties, mnemonic)
}

func getPointer(id identifier.Identifier) *identifier.Identifier {
	return &id
}

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int, args ...interface{}) identifier.Identifier {
	a := []interface{}{}
	a = append(a, propertyMnemonic, claimMnemonic, i)
	a = append(a, args...)
	return GetID(nameSpaceCoreProperties, a...)
}

func GenerateCoreProperties(properties []struct {
	Name            string
	ExtraNames      []string
	DescriptionHTML string
	Types           []string
},
) {
	for _, property := range properties {
		mnemonic := getMnemonic(property.Name)
		id := GetCorePropertyID(mnemonic)
		CoreProperties[id] = D{
			CoreDocument: CoreDocument{
				ID:    id,
				Score: LowConfidence,
			},
			Mnemonic: Mnemonic(mnemonic),
			Claims: &ClaimTypes{
				Text: TextClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "NAME", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("NAME")),
						},
						HTML: TranslatableHTMLString{
							"en": html.EscapeString(property.Name),
						},
					},
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("DESCRIPTION")),
						},
						HTML: TranslatableHTMLString{
							"en": property.DescriptionHTML,
						},
					},
				},
				Relation: RelationClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "TYPE", 0, "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("TYPE")),
						},
						To: Reference{
							ID: getPointer(GetCorePropertyID("PROPERTY")),
						},
					},
				},
			},
		}

		for i, extraName := range property.ExtraNames {
			CoreProperties[id].Claims.Text = append(CoreProperties[id].Claims.Text, TextClaim{
				CoreClaim: CoreClaim{
					ID:         getPropertyClaimID(mnemonic, "NAME", i+1),
					Confidence: 0.9, //nolint:gomnd
				},
				Prop: Reference{
					ID: getPointer(GetCorePropertyID("NAME")),
				},
				HTML: TranslatableHTMLString{
					"en": html.EscapeString(extraName),
				},
			})
		}

		for _, isClaim := range property.Types {
			isClaimMnemonic := getMnemonic(isClaim)
			CoreProperties[id].Claims.Relation = append(CoreProperties[id].Claims.Relation, RelationClaim{
				CoreClaim: CoreClaim{
					ID:         getPropertyClaimID(mnemonic, "TYPE", 0, isClaimMnemonic, 0),
					Confidence: 1.0,
				},
				Prop: Reference{
					ID: getPointer(GetCorePropertyID("TYPE")),
				},
				To: Reference{
					ID: getPointer(GetCorePropertyID(isClaimMnemonic)),
				},
			})
		}
	}
}

func generateAllCoreProperties() {
	GenerateCoreProperties(builtinProperties)

	for _, claimType := range claimTypes {
		name := fmt.Sprintf(`"%s" claim type`, claimType)
		mnemonic := getMnemonic(name)
		id := GetCorePropertyID(mnemonic)
		description := fmt.Sprintf(`The property is useful with the "%s" claim type.`, claimType)
		CoreProperties[id] = D{
			CoreDocument: CoreDocument{
				ID:    id,
				Score: LowConfidence,
			},
			Mnemonic: Mnemonic(mnemonic),
			Claims: &ClaimTypes{
				Text: TextClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "NAME", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("NAME")),
						},
						HTML: TranslatableHTMLString{
							"en": html.EscapeString(name),
						},
					},
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("DESCRIPTION")),
						},
						HTML: TranslatableHTMLString{
							"en": html.EscapeString(description),
						},
					},
				},
				Relation: RelationClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "TYPE", 0, "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("TYPE")),
						},
						To: Reference{
							ID: getPointer(GetCorePropertyID("PROPERTY")),
						},
					},
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "TYPE", 0, "CLAIM_TYPE", 0),
							Confidence: 1.0,
						},
						Prop: Reference{
							ID: getPointer(GetCorePropertyID("TYPE")),
						},
						To: Reference{
							ID: getPointer(GetCorePropertyID("CLAIM_TYPE")),
						},
					},
				},
			},
		}
	}
}

func init() { //nolint:gochecknoinits
	generateAllCoreProperties()
}
