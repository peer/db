package main

import (
	"context"
	"fmt"
	"html"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedError              = errors.Base("not supported")
	notSupportedDataValueTypeError = errors.BaseWrap(notSupportedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(notSupportedError, "not supported data type")

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

func getDocumentID(id string) search.Identifier {
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
		ID:    getDocumentID(id),
		Score: 0.0,
	}
}

func processSnak(ctx context.Context, entityID, prop, statementID string, confidence search.Confidence, snak mediawiki.Snak) (interface{}, errors.E) {
	if snak.Hash != "" {
		return nil, errors.Errorf("statement %s of property %s for entity %s has a snak without a hash", statementID, prop, entityID)
	}
	id := search.GetID(NameSpaceWikidata, entityID, prop, statementID, snak.Hash)

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return search.UnknownValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getDocumentReference(prop),
		}, nil
	case mediawiki.NoValue:
		return search.NoValueClaim{
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
			return search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:       getDocumentReference(prop),
				Identifier: string(value),
			}, nil
		case mediawiki.String:
			return search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:   getDocumentReference(prop),
				String: string(value),
			}, nil
		case mediawiki.CommonsMedia:
			fileInfo, err := getFileInfo(ctx, string(value))
			if err != nil {
				return nil, err
			}
			if fileInfo.MediaType == "" {
				return nil, errors.Errorf(`unknown media type for "%s"`, value)
			}
			claimID := search.GetID(NameSpaceWikidata, id, prop, statementID, snak.Hash, "WIKIMEDIA_COMMONS_FILE", 0)
			return search.FileClaim{
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
			return search.ReferenceClaim{
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
			return search.RelationClaim{
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
			return search.RelationClaim{
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
				return nil, errors.Errorf("%w: limited only to English", notSupportedError)
			}
			return search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:  getDocumentReference(prop),
				Plain: search.TranslatablePlainString{value.Language: value.Text},
				HTML:  search.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
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
				claimID := search.GetID(NameSpaceWikidata, id, prop, statementID, snak.Hash, "UNIT", 0)
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

			return claim, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType {
		case mediawiki.Time:
			// TODO: Convert timestamps in Julian calendar to ones in Gregorian calendar.
			return search.TimeClaim{
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

func processEntity(ctx context.Context, config *Config, processor *elastic.BulkProcessor, entity mediawiki.Entity) errors.E {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English content for now.
	if len(englishLabels) == 0 {
		if entity.Type == mediawiki.Property {
			// But properties should all have English label, so we warn here.
			fmt.Fprintf(os.Stderr, "property %s is missing a label in English\n", entity.ID)
		}
		return nil
	}

	id := getDocumentID(entity.ID)

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
		return errors.Errorf(`entity %s has invalid type: %d`, entity.ID, entity.Type)
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
				return nil
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
				Plain: search.TranslatablePlainString{
					"en": description,
				},
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
		statements := entity.Claims[prop]
		for i, statement := range statements {
			if statement.ID == "" {
				// All statements should have an ID, so we warn here.
				fmt.Fprintf(os.Stderr, "statement %d of property %s for entity %s is missing an ID\n", i, prop, entity.ID)
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claim, err := processSnak(ctx, entity.ID, prop, statement.ID, confidence, statement.MainSnak)
			if errors.Is(err, notSupportedError) {
				// We know what we do not support, ignore.
				continue
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has mainsnak that cannot be processed: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
			// err = addQualifiers(claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder)
			// if err != nil {
			// 	return err
			// }
			// err = addReferences(claim, entity.ID, prop, statement.ID, statement.References)
			// if err != nil {
			// 	return err
			// }
			err = document.Add(claim)
			if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s cannot be added: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
		}
	}

	saveDocument(config, processor, document)

	return nil
}

func saveDocument(config *Config, processor *elastic.BulkProcessor, doc search.Document) {
	req := elastic.NewBulkUpdateRequest().Index("docs").Id(string(doc.ID)).Doc(doc).DocAsUpsert(true)
	processor.Add(req)
}
