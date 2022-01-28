package search

import (
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
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
		"label",
		"amount",
		"amount range",
		"enumeration",
		"relation",
		"file",
		"time",
		"time range",
		"duration",
		"duration range",
		"list",
	}

	builtinProperties = []struct {
		Name            string
		DescriptionHTML string
		Is              []string
	}{
		{
			"is",
			"unspecified type relation between two entities",
			nil,
		},
		{
			"property",
			"the entity is a property",
			nil,
		},
		{
			"item",
			"the entity is an item",
			nil,
		},
		{
			"unit",
			"unit associated with the amount",
			nil,
		},
		{
			"claim type",
			"the property maps to a supported claim type",
			nil,
		},
		{
			"description",
			"description",
			[]string{`"text" claim type`},
		},
		{
			"article",
			"article",
			[]string{`"text" claim type`},
		},
		{
			"Wikidata property id",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>`,
			[]string{`"identifier" claim type`},
		},
		{
			"Wikidata item id",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>`,
			[]string{`"identifier" claim type`},
		},
		{
			"Wikidata property page",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property page IRI`,
			[]string{`"reference" claim type`},
		},
		{
			"Wikidata item page",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item page IRI`,
			[]string{`"reference" claim type`},
		},
		{
			"English Wikipedia article title",
			`<a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> article title`,
			[]string{`"identifier" claim type`},
		},
		{
			"English Wikipedia article",
			`reference to <a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> article`,
			[]string{`"reference" claim type`},
		},
		{
			"Wikimedia Commons file",
			`reference to <a href="https://commons.wikimedia.org/wiki/Main_Page">Wikimedia Commons</a> file`,
			[]string{`"reference" claim type`},
		},
	}

	NameSpaceStandardProperties = uuid.MustParse("34cd10b4-5731-46b8-a6dd-45444680ca62")

	// TODO: Use sync.Map?
	StandardProperties = map[string]Document{}
)

func GetStandardPropertyReference(mnemonic string) DocumentReference {
	property, ok := StandardProperties[string(GetStandardPropertyID(mnemonic))]
	if !ok {
		panic(errors.Errorf(`standard property for mnemonic "%s" cannot be found`, mnemonic))
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

func GetStandardPropertyID(mnemonic string) Identifier {
	return GetID(NameSpaceStandardProperties, mnemonic)
}

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int) Identifier {
	return GetID(NameSpaceStandardProperties, propertyMnemonic, claimMnemonic, i)
}

func populateStandardProperties() {
	for _, builtinProperty := range builtinProperties {
		mnemonic := getMnemonic(builtinProperty.Name)
		id := string(GetStandardPropertyID(mnemonic))
		StandardProperties[id] = Document{
			CoreDocument: CoreDocument{
				ID: Identifier(id),
				Name: Name{
					"en": builtinProperty.Name,
				},
				Score: 0.0,
			},
			Mnemonic: Mnemonic(mnemonic),
			Active: &ClaimTypes{
				Text: TextClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetStandardPropertyID("DESCRIPTION"),
							Name: Name{
								"en": "description",
							},
							Score: 0.0,
						},
						HTML: TranslatableHTMLString{
							"en": builtinProperty.DescriptionHTML,
						},
					},
				},
				Relation: RelationClaims{
					{
						CoreClaim: CoreClaim{
							ID:         getPropertyClaimID(mnemonic, "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: DocumentReference{
							ID: GetStandardPropertyID("IS"),
							Name: Name{
								"en": "is",
							},
							Score: 0.0,
						},
						To: DocumentReference{
							ID: GetStandardPropertyID("PROPERTY"),
							Name: Name{
								"en": "property",
							},
							Score: 0.0,
						},
					},
				},
			},
		}

		activeClaimTypes := StandardProperties[id].Active
		for _, isClaim := range builtinProperty.Is {
			isClaimMnemonic := getMnemonic(isClaim)
			activeClaimTypes.Relation = append(activeClaimTypes.Relation, RelationClaim{
				CoreClaim: CoreClaim{
					ID:         getPropertyClaimID(mnemonic, isClaimMnemonic, 0),
					Confidence: 1.0,
				},
				Prop: DocumentReference{
					ID: GetStandardPropertyID("IS"),
					Name: Name{
						"en": "is",
					},
					Score: 0.0,
				},
				To: DocumentReference{
					ID: GetStandardPropertyID(isClaimMnemonic),
					Name: Name{
						"en": isClaim,
					},
					Score: 0.0,
				},
			})
		}

		for _, claimType := range claimTypes {
			name := fmt.Sprintf(`"%s" claim type`, claimType)
			mnemonic := getMnemonic(name)
			id := string(GetStandardPropertyID(mnemonic))
			description := fmt.Sprintf(`the property is useful with the "%s" claim type`, claimType)
			StandardProperties[id] = Document{
				CoreDocument: CoreDocument{
					ID: Identifier(id),
					Name: Name{
						"en": name,
					},
					Score: 0.0,
				},
				Mnemonic: Mnemonic(mnemonic),
				Active: &ClaimTypes{
					Text: TextClaims{
						{
							CoreClaim: CoreClaim{
								ID:         getPropertyClaimID(mnemonic, "DESCRIPTION", 0),
								Confidence: 1.0,
							},
							Prop: DocumentReference{
								ID: GetStandardPropertyID("DESCRIPTION"),
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
								ID:         getPropertyClaimID(mnemonic, "PROPERTY", 0),
								Confidence: 1.0,
							},
							Prop: DocumentReference{
								ID: GetStandardPropertyID("IS"),
								Name: Name{
									"en": "is",
								},
								Score: 0.0,
							},
							To: DocumentReference{
								ID: GetStandardPropertyID("PROPERTY"),
								Name: Name{
									"en": "property",
								},
								Score: 0.0,
							},
						},
						{
							CoreClaim: CoreClaim{
								ID:         getPropertyClaimID(mnemonic, "CLAIM_TYPE", 0),
								Confidence: 1.0,
							},
							Prop: DocumentReference{
								ID: GetStandardPropertyID("IS"),
								Name: Name{
									"en": "is",
								},
								Score: 0.0,
							},
							To: DocumentReference{
								ID: GetStandardPropertyID("CLAIM_TYPE"),
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
}

func init() {
	populateStandardProperties()
}
