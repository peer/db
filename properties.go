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
		"amount",
		"amount range",
		"enumeration",
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
			"Wikidata reference",
			"A temporary group of multiple Wikidata reference statements as meta claims for later processing.",
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
			"media type",
			"Media (MIME) type of the file.",
			[]string{`"string" claim type`},
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
			[]string{`"enumeration" claim type`},
		},
		{
			"Uses English Wikipedia template",
			`Entity uses a <a href="https://en.wikipedia.org/wiki/Help:Templates">English Wikipedia template</a> in the source of its article or description.`,
			[]string{`"relation" claim type`},
		},
		{
			"Uses Wikimedia Commons template",
			`Entity uses a <a href="https://commons.wikimedia.org/wiki/Help:Templates">Wikimedia Commons template</a> in the source of its article or description.`,
			[]string{`"relation" claim type`},
		},
		{
			"In English Wikipedia category",
			`Entity is in <a href="https://en.wikipedia.org/wiki/Help:Category">English Wikipedia category</a>.`,
			[]string{`"relation" claim type`},
		},
		{
			"In Wikimedia Commons category",
			`Entity is in <a href="https://commons.wikimedia.org/wiki/Commons:Categories">Wikimedia Commons category</a>.`,
			[]string{`"relation" claim type`},
		},
	}

	nameSpaceStandardProperties = uuid.MustParse("34cd10b4-5731-46b8-a6dd-45444680ca62")

	// TODO: Use sync.Map?

	// StandardProperties is a map from a standard property ID to a document describing it.
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
	return GetID(nameSpaceStandardProperties, mnemonic)
}

func getPropertyClaimID(propertyMnemonic, claimMnemonic string, i int, args ...interface{}) Identifier {
	a := []interface{}{}
	a = append(a, propertyMnemonic, claimMnemonic, i)
	a = append(a, args...)
	return GetID(nameSpaceStandardProperties, a...)
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
							ID:         getPropertyClaimID(mnemonic, "IS", 0, "PROPERTY", 0),
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
					ID:         getPropertyClaimID(mnemonic, "IS", 0, isClaimMnemonic, 0),
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
			description := fmt.Sprintf(`The property is useful with the "%s" claim type.`, claimType)
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
								ID:         getPropertyClaimID(mnemonic, "IS", 0, "PROPERTY", 0),
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
								ID:         getPropertyClaimID(mnemonic, "IS", 0, "CLAIM_TYPE", 0),
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
