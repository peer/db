package wikipedia

import (
	"context"
	"fmt"
	"html"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	NotSupportedError              = errors.Base("not supported")
	notSupportedDataValueTypeError = errors.BaseWrap(NotSupportedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(NotSupportedError, "not supported data type")

	nonMainWikipediaNamespaces = []string{
		"User:",
		"Wikipedia:",
		"File:",
		"MediaWiki:",
		"Template:",
		"Help:",
		"Category:",
		"Portal:",
		"Draft:",
		"TimedText:",
		"Module:",
	}
)

func GetDocumentID(id string) search.Identifier {
	return search.GetID(NameSpaceWikidata, id)
}

func deduplicate(data []string) []string {
	duplicates := make(map[string]bool, len(data))
	res := []string{}
	for _, d := range data {
		if !duplicates[d] {
			res = append(res, d)
			duplicates[d] = true
		}
	}
	return res
}

func getEnglishValues(values map[string]mediawiki.LanguageValue) []string {
	res := []string{}
	// Deterministic iteration over a map.
	languages := make([]string, 0, len(values))
	for language := range values {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if language == "en" || strings.HasPrefix(language, "en-") {
			res = append(res, values[language].Value)
		}
	}
	return deduplicate(res)
}

func getEnglishValuesSlice(values map[string][]mediawiki.LanguageValue) []string {
	res := []string{}
	// Deterministic iteration over a map.
	languages := make([]string, 0, len(values))
	for language := range values {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if language == "en" || strings.HasPrefix(language, "en-") {
			for _, v := range values[language] {
				res = append(res, v.Value)
			}
		}
	}
	return deduplicate(res)
}

// We have more precise claim types so this is not very precise (e.g., quantity is used
// both for amount and durations). It is good for the first pass but later on we augment
// claims about claim types using statistics on how are properties really used.
// TODO: Really collect statistics and augment claims about claim types.
// TODO: Determine automatically mnemonics.
func getPropertyClaimType(dataType mediawiki.DataType) string {
	switch dataType {
	case mediawiki.WikiBaseItem:
		return "RELATION_CLAIM_TYPE"
	case mediawiki.ExternalID:
		return "IDENTIFIER_CLAIM_TYPE"
	case mediawiki.String:
		return "STRING_CLAIM_TYPE"
	case mediawiki.Quantity:
		return "AMOUNT_CLAIM_TYPE"
	case mediawiki.Time:
		return "TIME_CLAIM_TYPE"
	case mediawiki.GlobeCoordinate:
		// Not supported.
		return ""
	case mediawiki.CommonsMedia:
		return "FILE_CLAIM_TYPE"
	case mediawiki.MonolingualText:
		return "TEXT_CLAIM_TYPE"
	case mediawiki.URL:
		return "REFERENCE_CLAIM_TYPE"
	case mediawiki.GeoShape:
		// Not supported.
		return ""
	case mediawiki.WikiBaseLexeme:
		// Not supported.
		return ""
	case mediawiki.WikiBaseSense:
		// Not supported.
		return ""
	case mediawiki.WikiBaseProperty:
		return "RELATION_CLAIM_TYPE"
	case mediawiki.Math:
		// Not supported.
		return ""
	case mediawiki.MusicalNotation:
		// Not supported.
		return ""
	case mediawiki.WikiBaseForm:
		// Not supported.
		return ""
	case mediawiki.TabularData:
		// Not supported.
		return ""
	}
	panic(errors.Errorf(`invalid data type: %d`, dataType))
}

func getConfidence(entityID, prop, statementID string, rank mediawiki.StatementRank) search.Confidence {
	switch rank {
	case mediawiki.Preferred:
		return 1.0
	case mediawiki.Normal:
		return 0.5
	case mediawiki.Deprecated:
		return -1.0
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// It does not return a valid reference: name is missing.
func getDocumentReference(id string) search.DocumentReference {
	return search.DocumentReference{
		ID:    GetDocumentID(id),
		Score: 0.0,
	}
}

func processSnak(ctx context.Context, client *retryablehttp.Client, prop string, idArgs []interface{}, confidence search.Confidence, snak mediawiki.Snak) (search.Claim, errors.E) {
	id := search.GetID(NameSpaceWikidata, idArgs...)

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return &search.UnknownValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getDocumentReference(prop),
		}, nil
	case mediawiki.NoValue:
		return &search.NoValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getDocumentReference(prop),
		}, nil
	}

	if snak.DataValue == nil {
		return nil, errors.Errorf(`nil data value`)
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch snak.DataType {
		case mediawiki.ExternalID:
			return &search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:       getDocumentReference(prop),
				Identifier: string(value),
			}, nil
		case mediawiki.String:
			return &search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:   getDocumentReference(prop),
				String: string(value),
			}, nil
		case mediawiki.CommonsMedia:
			fileInfo, err := getFileInfo(ctx, client, string(value))
			if err != nil {
				return nil, err
			}
			if fileInfo.MediaType == "" {
				return nil, errors.Errorf(`unknown media type for "%s"`, value)
			}
			args := append([]interface{}{}, idArgs...)
			args = append(args, "WIKIMEDIA_COMMONS_FILE", 0)
			claimID := search.GetID(NameSpaceWikidata, args...)
			return &search.FileClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
					Meta: &search.MetaClaims{
						RefClaimTypes: search.RefClaimTypes{
							Reference: search.ReferenceClaims{
								{
									CoreClaim: search.CoreClaim{
										ID:         claimID,
										Confidence: 1.0,
									},
									Prop: search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE"),
									IRI:  fileInfo.PageURL,
								},
							},
						},
					},
				},
				Prop:    getDocumentReference(prop),
				Type:    fileInfo.MediaType,
				URL:     fileInfo.URL,
				Preview: fileInfo.Preview,
			}, nil
		case mediawiki.URL:
			return &search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				IRI:  string(value),
			}, nil
		case mediawiki.GeoShape:
			return nil, errors.Errorf("%w: GeoShape", notSupportedDataTypeError)
		case mediawiki.Math:
			return nil, errors.Errorf("%w: Math", notSupportedDataTypeError)
		case mediawiki.MusicalNotation:
			return nil, errors.Errorf("%w: MusicalNotation", notSupportedDataTypeError)
		case mediawiki.TabularData:
			return nil, errors.Errorf("%w: TabularData", notSupportedDataTypeError)
		default:
			return nil, errors.Errorf("unexpected data type for StringValue: %d", snak.DataType)
		}
	case mediawiki.WikiBaseEntityIDValue:
		switch snak.DataType {
		case mediawiki.WikiBaseItem:
			if value.Type != mediawiki.ItemType {
				return nil, errors.Errorf("WikiBaseItem data type, but WikiBaseEntityIDValue has type %d, not ItemType", value.Type)
			}
			return &search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				To:   getDocumentReference(value.ID),
			}, nil
		case mediawiki.WikiBaseProperty:
			if value.Type != mediawiki.PropertyType {
				return nil, errors.Errorf("WikiBaseProperty data type, but WikiBaseEntityIDValue has type %d, not PropertyType", value.Type)
			}
			return &search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				To:   getDocumentReference(value.ID),
			}, nil
		case mediawiki.WikiBaseLexeme:
			return nil, errors.Errorf("%w: WikiBaseLexeme", notSupportedDataTypeError)
		case mediawiki.WikiBaseSense:
			return nil, errors.Errorf("%w: WikiBaseSense", notSupportedDataTypeError)
		case mediawiki.WikiBaseForm:
			return nil, errors.Errorf("%w: WikiBaseForm", notSupportedDataTypeError)
		default:
			return nil, errors.Errorf("unexpected data type for WikiBaseEntityIDValue: %d", snak.DataType)
		}
	case mediawiki.GlobeCoordinateValue:
		return nil, errors.Errorf("%w: GlobeCoordinateValue", notSupportedDataValueTypeError)
	case mediawiki.MonolingualTextValue:
		switch snak.DataType {
		case mediawiki.MonolingualText:
			if value.Language != "en" && !strings.HasPrefix(value.Language, "en-") {
				return nil, errors.Errorf("%w: limited only to English", NotSupportedError)
			}
			return &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				HTML: search.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for MonolingualTextValue: %d", snak.DataType)
		}
	case mediawiki.QuantityValue:
		switch snak.DataType {
		case mediawiki.Quantity:
			amount, exact := value.Amount.Float64()
			if !exact && math.IsInf(amount, 0) {
				return nil, errors.Errorf("amount cannot be represented by float64: %s", value.Amount.String())
			}
			var uncertaintyLower, uncertaintyUpper *float64
			if value.LowerBound != nil && value.UpperBound != nil {
				l, exact := value.LowerBound.Float64()
				if !exact && math.IsInf(l, 0) {
					return nil, errors.Errorf("lower bound cannot be represented by float64: %s", value.LowerBound.String())
				}
				uncertaintyLower = &l
				u, exact := value.UpperBound.Float64()
				if !exact && math.IsInf(u, 0) {
					return nil, errors.Errorf("upper bound cannot be represented by float64: %s", value.UpperBound.String())
				}
				uncertaintyUpper = &u
				if *uncertaintyLower > amount {
					return nil, errors.Errorf("lower bound %f cannot be larger than the amount %f", *uncertaintyLower, amount)
				}
				if *uncertaintyUpper < amount {
					return nil, errors.Errorf("upper bound %f cannot be smaller than the amount %f", *uncertaintyUpper, amount)
				}
			} else if value.LowerBound != nil || value.UpperBound != nil {
				return nil, errors.Errorf("both lower and upper bounds have to be provided, or none, not just one")
			}
			claim := search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:             getDocumentReference(prop),
				Amount:           amount,
				UncertaintyLower: uncertaintyLower,
				UncertaintyUpper: uncertaintyUpper,
			}

			if value.Unit == "1" {
				claim.Unit = search.AmountUnitNone
			} else {
				// For now we store the amount as-is and convert to the same unit later on
				// using the unit we store into meta claims.
				claim.Unit = search.AmountUnitCustom
				args := append([]interface{}{}, idArgs...)
				args = append(args, "UNIT", 0)
				claimID := search.GetID(NameSpaceWikidata, args...)
				var unitID string
				if strings.HasPrefix(value.Unit, "http://www.wikidata.org/entity/") {
					unitID = strings.TrimPrefix(value.Unit, "http://www.wikidata.org/entity/")
				} else if strings.HasPrefix(value.Unit, "https://www.wikidata.org/wiki/") {
					unitID = strings.TrimPrefix(value.Unit, "https://www.wikidata.org/wiki/")
				} else {
					return nil, errors.Errorf("unsupported unit URL: %s", value.Unit)
				}
				claim.CoreClaim.Meta = &search.MetaClaims{
					SimpleClaimTypes: search.SimpleClaimTypes{
						Relation: search.RelationClaims{
							{
								CoreClaim: search.CoreClaim{
									ID:         claimID,
									Confidence: 1.0,
								},
								Prop: search.GetStandardPropertyReference("UNIT"),
								To:   getDocumentReference(unitID),
							},
						},
					},
				}
			}

			return &claim, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType {
		case mediawiki.Time:
			// TODO: Convert timestamps in Julian calendar to ones in Gregorian calendar.
			return &search.TimeClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:      getDocumentReference(prop),
				Timestamp: search.Timestamp(value.Time),
				Precision: search.TimePrecision(value.Precision),
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", snak.DataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func addQualifiers(
	ctx context.Context, client *retryablehttp.Client, claim search.Claim,
	entityID, prop, statementID string,
	qualifiers map[string][]mediawiki.Snak, qualifiersOrder []string,
) errors.E {
	for _, p := range qualifiersOrder {
		for i, qualifier := range qualifiers[p] {
			qualifierClaim, err := processSnak(ctx, client, p, []interface{}{entityID, prop, statementID, "qualifier", p, i}, 0.5, qualifier)
			if errors.Is(err, NotSupportedError) {
				// We know what we do not support, ignore.
				continue
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has qualifiers that cannot be processed: property %s, qualifier %d: snak cannot be processed: %s\n", statementID, prop, entityID, p, i, err.Error())
				continue
			}
			err = claim.AddMeta(qualifierClaim)
			if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has qualifiers that cannot be processed: property %s, qualifier %d: cannot be added to meta claims\n", statementID, prop, entityID, p, i)
			}
		}
	}
	return nil
}

// addReference uses the first snak of a reference to construct a claim and all other snaks are added as meta claims of that first claim.
func addReference(ctx context.Context, client *retryablehttp.Client, claim search.Claim, entityID, prop, statementID string, i int, reference mediawiki.Reference) errors.E {
	var referenceClaim search.Claim

	for _, p := range reference.SnaksOrder {
		for j, snak := range reference.Snaks[p] {
			c, err := processSnak(ctx, client, p, []interface{}{entityID, prop, statementID, "reference", i, p, j}, 0.5, snak)
			if errors.Is(err, NotSupportedError) {
				// We know what we do not support, ignore.
				continue
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has a reference %d that cannot be processed: snak %s/%d cannot be processed: %s\n", statementID, prop, entityID, i, p, j, err.Error())
				continue
			}
			if referenceClaim == nil {
				referenceClaim = c
			} else {
				err = referenceClaim.AddMeta(c)
				if err != nil {
					fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has a reference %d that cannot be processed: snak %s/%d cannot be processed: cannot be added to meta claims\n", statementID, prop, entityID, i, p, j)
				}
			}
		}
	}

	if referenceClaim == nil {
		return nil
	}

	err := claim.AddMeta(referenceClaim)
	if err != nil {
		fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has a reference %d that cannot be processed: cannot be added to meta claims\n", statementID, prop, entityID, i)
	}

	return nil
}

func ConvertEntity(ctx context.Context, client *retryablehttp.Client, entity mediawiki.Entity) (*search.Document, errors.E) {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English content for now.
	if len(englishLabels) == 0 {
		if entity.Type == mediawiki.Property {
			// But properties should all have English label, so we warn here.
			fmt.Fprintf(os.Stderr, "property %s is missing a label in English\n", entity.ID)
		}
		return nil, errors.Errorf("%w: limited only to English", NotSupportedError)
	}

	id := GetDocumentID(entity.ID)

	// We simply use the first label we have.
	name := englishLabels[0]
	englishLabels = englishLabels[1:]

	// TODO: Set mnemonic if a property and the name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere.
	document := search.Document{
		CoreDocument: search.CoreDocument{
			ID: id,
			Name: search.Name{
				"en": name,
			},
			Score: 0.0,
		},
	}

	if entity.Type == mediawiki.Property {
		document.Active = &search.DocumentClaimTypes{
			RefClaimTypes: search.RefClaimTypes{
				Identifier: search.IdentifierClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_PROPERTY_ID", 0),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
							Confidence: 1.0,
						},
						Prop: search.GetStandardPropertyReference("WIKIDATA_PROPERTY_PAGE"),
						IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/Property:%s", entity.ID),
					},
				},
			},
			SimpleClaimTypes: search.SimpleClaimTypes{
				Relation: search.RelationClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "IS", "PROPERTY", 0),
							Confidence: 1.0,
						},
						Prop: search.GetStandardPropertyReference("IS"),
						To:   search.GetStandardPropertyReference("PROPERTY"),
					},
				},
			},
		}
	} else if entity.Type == mediawiki.Item {
		document.Active = &search.DocumentClaimTypes{
			RefClaimTypes: search.RefClaimTypes{
				Identifier: search.IdentifierClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_ITEM_ID", 0),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_ITEM_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_ITEM_PAGE", 0),
							Confidence: 1.0,
						},
						Prop: search.GetStandardPropertyReference("WIKIDATA_ITEM_PAGE"),
						IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/%s", entity.ID),
					},
				},
			},
			SimpleClaimTypes: search.SimpleClaimTypes{
				Relation: search.RelationClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceWikidata, entity.ID, "IS", "ITEM", 0),
							Confidence: 1.0,
						},
						Prop: search.GetStandardPropertyReference("IS"),
						To:   search.GetStandardPropertyReference("ITEM"),
					},
				},
			},
		}
	} else {
		return nil, errors.Errorf(`entity %s has invalid type: %d`, entity.ID, entity.Type)
	}

	siteLink, ok := entity.SiteLinks["enwiki"]
	if ok {
		url := siteLink.URL
		if url == "" {
			url = fmt.Sprintf("https://en.wikipedia.org/wiki/%s", siteLink.Title)
		}
		for _, namespace := range nonMainWikipediaNamespaces {
			if strings.HasPrefix(siteLink.Title, namespace) {
				// Only items have sitelinks. We want only items related to main
				// Wikipedia articles (main namespace).
				return nil, errors.Errorf("%w: limited only to items related to main Wikipedia articles: %s", NotSupportedError, siteLink.Title)
			}
		}
		document.Active.RefClaimTypes.Identifier = append(document.Active.RefClaimTypes.Identifier, search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikidata, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE_TITLE", 0),
				Confidence: 1.0,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE_TITLE"),
			Identifier: siteLink.Title,
		})
		document.Active.RefClaimTypes.Reference = append(document.Active.RefClaimTypes.Reference, search.ReferenceClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikidata, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE", 0),
				Confidence: 1.0,
			},
			Prop: search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE"),
			IRI:  url,
		})
	}

	if entity.DataType != nil {
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			document.Active.SimpleClaimTypes.Relation = append(document.Active.SimpleClaimTypes.Relation, search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID: search.GetID(NameSpaceWikidata, entity.ID, "IS", claimTypeMnemonic, 0),
					// We have low confidence in this claim. Later on we augment it using statistics
					// on how are properties really used.
					Confidence: 0.0,
				},
				Prop: search.GetStandardPropertyReference("IS"),
				To:   search.GetStandardPropertyReference(claimTypeMnemonic),
			})
		}
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	if len(englishAliases) > 0 {
		englishLabels = append(englishLabels, englishAliases...)
		englishLabels = deduplicate(englishLabels)
	}

	if len(englishLabels) > 0 {
		document.CoreDocument.OtherNames = search.OtherNames{
			"en": englishLabels,
		}
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	if len(englishDescriptions) > 0 {
		simple := &document.Active.SimpleClaimTypes
		for i, description := range englishDescriptions {
			simple.Text = append(simple.Text, search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceWikidata, entity.ID, "DESCRIPTION", i),
					Confidence: 1.0,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{
					"en": html.EscapeString(description),
				},
			})
		}
	}

	// Deterministic iteration over a map.
	props := make([]string, 0, len(entity.Claims))
	for prop := range entity.Claims {
		props = append(props, prop)
	}
	sort.Strings(props)

	for _, prop := range props {
		for i, statement := range entity.Claims[prop] {
			if statement.ID == "" {
				// All statements should have an ID, so we warn here.
				fmt.Fprintf(os.Stderr, "statement %d of property %s for entity %s is missing an ID\n", i, prop, entity.ID)
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claim, err := processSnak(ctx, client, prop, []interface{}{entity.ID, prop, statement.ID, "mainsnak"}, confidence, statement.MainSnak)
			if errors.Is(err, NotSupportedError) {
				// We know what we do not support, ignore.
				continue
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has mainsnak that cannot be processed: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
			err = addQualifiers(ctx, client, claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder)
			if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has qualifiers that cannot be processed: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
			for i, reference := range statement.References {
				err = addReference(ctx, client, claim, entity.ID, prop, statement.ID, i, reference)
				if err != nil {
					fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has a reference %d that cannot be processed: %s\n", statement.ID, prop, entity.ID, i, err.Error())
					continue
				}
			}
			err = document.Add(claim)
			if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s cannot be added: %s\n", statement.ID, prop, entity.ID, err.Error())
			}
		}
	}

	return &document, nil
}
