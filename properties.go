package search

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

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
			"is",
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
			"also known as",
			"Entity is also known as.",
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
	CoreProperties = map[string]Document{}
)

func GetCorePropertyReference(mnemonic string) DocumentReference {
	property, ok := CoreProperties[string(GetCorePropertyID(mnemonic))]
	if !ok {
		panic(errors.Errorf(`core property for mnemonic "%s" cannot be found`, mnemonic))
	}
	return DocumentReference{
		ID:     property.ID,
		Name:   property.Name,
		Score:  property.Score,
		Scores: property.Scores,
	}
}

func getMnemonic(data string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(data), " ", "_"), `"`, "")
}

func GetID(namespace uuid.UUID, args ...interface{}) Identifier {
	res := namespace
	for _, arg := range args {
		res = uuid.NewSHA1(res, []byte(fmt.Sprint(arg)))
	}
	return Identifier(identifier.FromUUID(res))
}

func GetCorePropertyID(mnemonic string) Identifier {
	return GetID(nameSpaceCoreProperties, mnemonic)
}

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int, args ...interface{}) Identifier {
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
		id := string(GetCorePropertyID(mnemonic))
		CoreProperties[id] = Document{
			CoreDocument: CoreDocument{
				ID: Identifier(id),
				Name: Name{
					"en": property.Name,
				},
				Score: 0.0,
			},
			Mnemonic: Mnemonic(mnemonic),
			Claims: &ClaimTypes{
				Text: TextClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetCorePropertyID("DESCRIPTION"),
							Name: Name{
								"en": "description",
							},
							Score: 0.0,
						},
						HTML: TranslatableHTMLString{
							"en": property.DescriptionHTML,
						},
					},
				},
				Relation: RelationClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "IS", 0, "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetCorePropertyID("IS"),
							Name: Name{
								"en": "is",
							},
							Score: 0.0,
						},
						To: DocumentReference{
							ID: GetCorePropertyID("PROPERTY"),
							Name: Name{
								"en": "property",
							},
							Score: 0.0,
						},
					},
				},
			},
		}

		for _, isClaim := range property.Is {
			isClaimMnemonic := getMnemonic(isClaim)
			CoreProperties[id].Claims.Relation = append(CoreProperties[id].Claims.Relation, RelationClaim{
				CoreClaim: CoreClaim{
					ID:         getPropertyClaimID(mnemonic, "IS", 0, isClaimMnemonic, 0),
					Confidence: 1.0,
				},
				Prop: DocumentReference{
					ID: GetCorePropertyID("IS"),
					Name: Name{
						"en": "is",
					},
					Score: 0.0,
				},
				To: DocumentReference{
					ID: GetCorePropertyID(isClaimMnemonic),
					Name: Name{
						"en": isClaim,
					},
					Score: 0.0,
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
		id := string(GetCorePropertyID(mnemonic))
		description := fmt.Sprintf(`The property is useful with the "%s" claim type.`, claimType)
		CoreProperties[id] = Document{
			CoreDocument: CoreDocument{
				ID: Identifier(id),
				Name: Name{
					"en": name,
				},
				Score: 0.0,
			},
			Mnemonic: Mnemonic(mnemonic),
			Claims: &ClaimTypes{
				Text: TextClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetCorePropertyID("DESCRIPTION"),
							Name: Name{
								"en": "description",
							},
							Score: 0.0,
						},
						HTML: TranslatableHTMLString{
							"en": html.EscapeString(description),
						},
					},
				},
				Relation: RelationClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "IS", 0, "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetCorePropertyID("IS"),
							Name: Name{
								"en": "is",
							},
							Score: 0.0,
						},
						To: DocumentReference{
							ID: GetCorePropertyID("PROPERTY"),
							Name: Name{
								"en": "property",
							},
							Score: 0.0,
						},
					},
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "IS", 0, "CLAIM_TYPE", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetCorePropertyID("IS"),
							Name: Name{
								"en": "is",
							},
							Score: 0.0,
						},
						To: DocumentReference{
							ID: GetCorePropertyID("CLAIM_TYPE"),
							Name: Name{
								"en": "claim type",
							},
							Score: 0.0,
						},
					},
				},
			},
		}
	}
}

func SaveCoreProperties(ctx context.Context, log zerolog.Logger, esClient *elastic.Client, processor *elastic.BulkProcessor, index string) errors.E {
	for _, property := range CoreProperties {
		if ctx.Err() != nil {
			break
		}

		property := property
		log.Debug().Str("doc", string(property.ID)).Str("mnemonic", string(property.Mnemonic)).Msg("saving document")
		InsertOrReplaceDocument(processor, index, &property)
	}

	// Make sure all just added documents are available for search.
	err := processor.Flush()
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = esClient.Refresh(index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func init() {
	generateAllCoreProperties()
}
