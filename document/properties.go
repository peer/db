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
		DescriptionHTML string
		Is              []string
	}{
		{
			"type",
			"The entity is related in an unspecified way.",
			[]string{`"relation" claim type`},
		},
		{
			"label",
			"The entity is a label.",
			[]string{`"relation" claim type`},
		},
		{
			"property",
			"The entity is a property.",
			nil,
		},
		{
			"item",
			"The entity is an item.",
			nil,
		},
		{
			"file",
			"The entity is a file.",
			nil,
		},
		{
			"file URL",
			"URL of the file.",
			[]string{`"reference" claim type`},
		},
		{
			"preview URL",
			"URL of the preview.",
			[]string{`"reference" claim type`},
		},
		{
			"unit",
			"Unit associated with the amount.",
			nil,
		},
		{
			"claim type",
			"The property maps to a supported claim type.",
			nil,
		},
		{
			"description",
			"Description of the entity.",
			[]string{`"text" claim type`},
		},
		{
			"article",
			"Article about the entity.",
			[]string{`"text" claim type`},
		},
		{
			"has article",
			"The entity has an article.",
			nil,
		},
		{
			"name",
			"The name of the entity.",
			[]string{`"text" claim type`},
		},
		{
			"list",
			"A list has an unique ID, even a list with just one element. All elements of the list share this ID.",
			[]string{`"identifier" claim type`},
		},
		{
			"order",
			"Order of an element inside its list. Smaller numbers are closer to the beginning of the list.",
			[]string{`"amount" claim type`},
		},
		{
			// TODO: How to define a property (type of relation) between parent and child?
			"child",
			"List elements might have other lists as children. This is an ID of the child list.",
			[]string{`"identifier" claim type`},
		},
		{
			"page count",
			"Number of pages the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"size",
			"The size the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"length",
			"The length the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"width",
			"The width the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"height",
			"The height the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"depth",
			"The depth the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"weight",
			"The weight the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"diameter",
			"The diameter the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"circumference",
			"The circumference the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"duration",
			"The duration the entity has.",
			[]string{`"amount" claim type`},
		},
		{
			"media type",
			"Media (MIME) type of the file.",
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

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int, args ...interface{}) identifier.Identifier { //nolint:unparam
	a := []interface{}{}
	a = append(a, propertyMnemonic, claimMnemonic, i)
	a = append(a, args...)
	return GetID(nameSpaceCoreProperties, a...)
}

func GenerateCoreProperties(properties []struct {
	Name            string
	DescriptionHTML string
	Is              []string
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

		for _, isClaim := range property.Is {
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
