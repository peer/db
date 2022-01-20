package search

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

var (
	// TODO: Determine automatically.
	claimTypes = []string{
		// Meta claim types.
		"is",
		"identifier",
		"reference",

		// Simple claim types.
		"text",
		"string",
		"label",
		"amount",
		"amount range",
		"enumeration",

		// Time claim types.
		"time",
		"time range",
		"duration",
		"duration range",

		// Item claim types.
		"file",
		"list",
		"item",
	}

	builtinProperties = []struct {
		Name             string
		DescriptionPlain string
		DescriptionHTML  string
		Is               []string
	}{
		{
			"claim type",
			"the property maps to a supported claim type",
			"the property maps to a supported claim type",
			nil,
		},
		{
			"description",
			"description",
			"description",
			[]string{`"text" claim type`},
		},
		{
			"Wikidata property id",
			"Wikidata property identifier",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>`,
			[]string{`"identifier" claim type`},
		},
		{
			"Wikidata item id",
			"Wikidata item identifier",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item <a href="https://www.wikidata.org/wiki/Wikidata:Identifiers">identifier</a>`,
			[]string{`"identifier" claim type`},
		},
		{
			"Wikidata property page",
			"Wikidata property page",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> property page IRI`,
			[]string{`"reference" claim type`},
		},
		{
			"Wikidata item page",
			"Wikidata item page",
			`<a href="https://www.wikidata.org/wiki/Wikidata:Main_Page">Wikidata</a> item page IRI`,
			[]string{`"reference" claim type`},
		},
		{
			"English Wikipedia article",
			"reference to English Wikipedia article",
			`reference to <a href="https://en.wikipedia.org/wiki/Main_Page">English Wikipedia</a> article`,
			[]string{`"reference" claim type`},
		},
	}

	NameSpaceStandardProperties = uuid.MustParse("34cd10b4-5731-46b8-a6dd-45444680ca62")

	// TODO: Use sync.Map.
	KnownProperties = map[string]Property{}
)

func GetStandardPropertyReference(mnemonic string) PropertyReference {
	property, ok := KnownProperties[getPropertyID(mnemonic)]
	if !ok {
		panic(errors.Errorf(`standard property for mnemonic "%s" cannot be found`, mnemonic))
	}
	return PropertyReference{
		ID:     property.ID,
		Name:   property.Name,
		Score:  property.Score,
		Scores: property.Scores,
	}
}

func getMnemonic(data string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(data), " ", "_"), `"`, "")
}

func getPropertyID(mnemonic string) string {
	return identifier.FromUUID(uuid.NewSHA1(NameSpaceStandardProperties, []byte(mnemonic)))
}

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int) string {
	return identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					NameSpaceStandardProperties,
					[]byte(propertyMnemonic),
				),
				[]byte(claimMnemonic),
			),
			[]byte(strconv.Itoa(i)),
		),
	)
}

func populateStandardProperties() {
	for _, builtinProperty := range builtinProperties {
		mnemonic := getMnemonic(builtinProperty.Name)
		id := getPropertyID(mnemonic)
		KnownProperties[id] = Property{
			CoreDocument: CoreDocument{
				ID: Identifier(id),
				Name: Name{
					"en": builtinProperty.Name,
				},
				Score: 0.0,
			},
			Mnemonic: Mnemonic(mnemonic),
			Active: &PropertyClaimTypes{
				SimpleClaimTypes: SimpleClaimTypes{
					Text: TextClaims{
						{
							CoreClaim: CoreClaim{
								ID:         Identifier(getPropertyClaimID(mnemonic, "DESCRIPTION", 0)),
								Confidence: 1.0,
							},
							Prop: PropertyReference{
								ID: Identifier(getPropertyID("DESCRIPTION")),
								Name: Name{
									"en": "description",
								},
								Score: 0.0,
							},
							Plain: TranslatablePlainString{
								"en": builtinProperty.DescriptionPlain,
							},
							HTML: TranslatableHTMLString{
								"en": builtinProperty.DescriptionHTML,
							},
						},
					},
				},
			},
		}

		meta := &KnownProperties[id].Active.MetaClaimTypes
		for _, isClaim := range builtinProperty.Is {
			isClaimMnemonic := getMnemonic(isClaim)
			meta.Is = append(meta.Is, IsClaim{
				CoreClaim: CoreClaim{
					ID:         Identifier(getPropertyClaimID(mnemonic, isClaimMnemonic, 0)),
					Confidence: 1.0,
				},
				Prop: PropertyReference{
					ID: Identifier(getPropertyID(isClaimMnemonic)),
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
			id := getPropertyID(mnemonic)
			description := fmt.Sprintf(`the property is useful with the "%s" claim type`, claimType)
			KnownProperties[id] = Property{
				CoreDocument: CoreDocument{
					ID: Identifier(id),
					Name: Name{
						"en": name,
					},
					Score: 0.0,
				},
				Mnemonic: Mnemonic(mnemonic),
				Active: &PropertyClaimTypes{
					MetaClaimTypes: MetaClaimTypes{
						Is: IsClaims{
							{
								CoreClaim: CoreClaim{
									ID:         Identifier(getPropertyClaimID(mnemonic, "CLAIM_TYPE", 0)),
									Confidence: 1.0,
								},
								Prop: PropertyReference{
									ID: Identifier(getPropertyID("CLAIM_TYPE")),
									Name: Name{
										"en": "claim type",
									},
									Score: 0.0,
								},
							},
						},
					},
					SimpleClaimTypes: SimpleClaimTypes{
						Text: TextClaims{
							{
								CoreClaim: CoreClaim{
									ID:         Identifier(getPropertyClaimID(mnemonic, "DESCRIPTION", 0)),
									Confidence: 1.0,
								},
								Prop: PropertyReference{
									ID: Identifier(getPropertyID("DESCRIPTION")),
									Name: Name{
										"en": "description",
									},
									Score: 0.0,
								},
								Plain: TranslatablePlainString{
									"en": description,
								},
								HTML: TranslatableHTMLString{
									"en": html.EscapeString(description),
								},
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
