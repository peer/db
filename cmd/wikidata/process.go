package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/identifier"
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedError              = errors.Base("not supported")
	notSupportedDataValueTypeError = errors.BaseWrap(notSupportedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(notSupportedError, "not supported data type")
)

func getDocumentID(id string) search.Identifier {
	return search.Identifier(identifier.FromUUID(uuid.NewSHA1(NameSpaceWikidata, []byte(id))))
}

func getStandardPropertyClaimID(id, mnemonic string, i int) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					NameSpaceWikidata,
					[]byte(id),
				),
				[]byte(mnemonic),
			),
			[]byte(strconv.Itoa(i)),
		),
	))
}

func getPropertyClaimIDFromHash(id, prop, statementID, hash string) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					uuid.NewSHA1(
						NameSpaceWikidata,
						[]byte(id),
					),
					[]byte(prop),
				),
				[]byte(statementID),
			),
			[]byte(hash),
		),
	))
}

func getPropertyClaimID(id, prop, statementID, namespace string, i int) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					uuid.NewSHA1(
						uuid.NewSHA1(
							NameSpaceWikidata,
							[]byte(id),
						),
						[]byte(prop),
					),
					[]byte(statementID),
				),
				[]byte(namespace),
			),
			[]byte(strconv.Itoa(i)),
		),
	))
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
func getDocumentReference(prop string) search.DocumentReference {
	return search.DocumentReference{
		ID:    getDocumentID(prop),
		Score: 0.0,
	}
}

func processSnak(entityID, prop, statementID, namespace string, confidence search.Confidence, snak mediawiki.Snak) (interface{}, errors.E) {
	var id search.Identifier
	if snak.Hash != "" {
		id = getPropertyClaimIDFromHash(entityID, prop, statementID, snak.Hash)
	} else {
		id = getPropertyClaimID(entityID, prop, statementID, namespace, 0)
	}

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
			return nil, errors.New("TODO string+CommonsMedia")
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
			return nil, errors.New("TODO Quantity")
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType {
		case mediawiki.Time:
			return nil, errors.New("TODO Time")
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", snak.DataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func processEntity(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
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
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_ID", 0),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
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
							ID:         getStandardPropertyClaimID(entity.ID, "PROPERTY", 0),
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
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_ITEM_ID", 0),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_ITEM_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_ITEM_PAGE", 0),
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
							ID:         getStandardPropertyClaimID(entity.ID, "ITEM", 0),
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

	if entity.DataType != nil {
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			document.Active.SimpleClaimTypes.Relation = append(document.Active.SimpleClaimTypes.Relation, search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID: getStandardPropertyClaimID(entity.ID, claimTypeMnemonic, 0),
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
					ID:         getStandardPropertyClaimID(entity.ID, "DESCRIPTION", i),
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
			claim, err := processSnak(entity.ID, prop, statement.ID, "", confidence, statement.MainSnak)
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

	return saveDocument(config, document)
}

func saveDocument(config *Config, property search.Document) errors.E {
	path := filepath.Join(config.OutputDir, fmt.Sprintf("%s.json", property.ID))
	file, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(property)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
