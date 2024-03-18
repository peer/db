package wikipedia

import (
	"context"
	"fmt"
	"html"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
)

const (
	WikidataReference                 = "Wikidata-"
	WikimediaCommonsEntityReference   = "CommonsEntity-"
	WikimediaCommonsFileReference     = "CommonsFile-"
	WikipediaCategoryReference        = "WikipediaCategory-"
	WikipediaTemplateReference        = "WikipediaTemplate-"
	WikimediaCommonsCategoryReference = "CommonsCategory-"
	WikimediaCommonsTemplateReference = "CommonsTemplate-"
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedDataValueTypeError = errors.BaseWrap(SilentSkippedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(SilentSkippedError, "not supported data type")
	NotFoundError                  = errors.Base("not found")

	// Besides main namespace we allow also templates, modules, and categories.
	nonMainNamespaces = []string{
		"User:",
		"Wikipedia:",
		"File:",
		"MediaWiki:",
		"Help:",
		"Portal:",
		"Draft:",
		"TimedText:",
	}

	dataTypeToClaimTypeMap = map[mediawiki.DataType]string{
		mediawiki.WikiBaseItem: "RELATION_CLAIM_TYPE",
		mediawiki.ExternalID:   "IDENTIFIER_CLAIM_TYPE",
		mediawiki.String:       "STRING_CLAIM_TYPE",
		mediawiki.Quantity:     "AMOUNT_CLAIM_TYPE",
		mediawiki.Time:         "TIME_CLAIM_TYPE",
		// Not supported.
		mediawiki.GlobeCoordinate: "",
		mediawiki.CommonsMedia:    "FILE_CLAIM_TYPE",
		mediawiki.MonolingualText: "TEXT_CLAIM_TYPE",
		mediawiki.URL:             "REFERENCE_CLAIM_TYPE",
		// Not supported.
		mediawiki.GeoShape: "",
		// Not supported.
		mediawiki.WikiBaseLexeme: "",
		// Not supported.
		mediawiki.WikiBaseSense:    "",
		mediawiki.WikiBaseProperty: "RELATION_CLAIM_TYPE",
		// Not supported.
		mediawiki.Math: "",
		// Not supported.
		mediawiki.MusicalNotation: "",
		// Not supported.
		mediawiki.WikiBaseForm: "",
		// Not supported.
		mediawiki.TabularData: "",
	}

	claimTypeToDataTypesMap = map[string][]mediawiki.DataType{}
)

func init() {
	for dataType, claimType := range dataTypeToClaimTypeMap {
		// We skip if not supported.
		if claimType == "" {
			continue
		}
		claimTypeToDataTypesMap[claimType] = append(claimTypeToDataTypesMap[claimType], dataType)
	}
}

func GetWikidataDocumentID(id string) identifier.Identifier {
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

// We have more precise claim types so this is not very precise. It is good for the
// first pass but later on we augment claims about claim types using statistics on
// how are properties really used.
// TODO: Really collect statistics and augment claims about claim types.
// TODO: Determine automatically mnemonics.
func getPropertyClaimType(dataType mediawiki.DataType) string {
	claimType, ok := dataTypeToClaimTypeMap[dataType]
	if !ok {
		panic(errors.Errorf(`invalid data type: %d`, dataType))
	}
	return claimType
}

func getConfidence(entityID, prop, statementID string, rank mediawiki.StatementRank) search.Confidence {
	switch rank {
	case mediawiki.Preferred:
		return es.HighConfidence
	case mediawiki.Normal:
		return es.MediumConfidence
	case mediawiki.Deprecated:
		return es.NoConfidence
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// getDocumentReference does not return a valid reference, but it encodes original
// ID into the _temp field to be resolved later. It panics for unsupported IDs.
func getDocumentReference(id, source string) search.DocumentReference {
	if strings.HasPrefix(id, "M") {
		return search.DocumentReference{
			ID:        nil,
			Temporary: WikimediaCommonsEntityReference + id,
		}
	} else if strings.HasPrefix(id, "P") || strings.HasPrefix(id, "Q") {
		return search.DocumentReference{
			ID:        nil,
			Temporary: WikidataReference + id,
		}
	} else if strings.HasPrefix(id, "Category:") {
		switch source {
		case "ENGLISH_WIKIPEDIA":
			return search.DocumentReference{
				ID:        nil,
				Temporary: WikipediaCategoryReference + id,
			}
		case "WIKIMEDIA_COMMONS":
			return search.DocumentReference{
				ID:        nil,
				Temporary: WikimediaCommonsCategoryReference + id,
			}
		}
	} else if strings.HasPrefix(id, "Template:") || strings.HasPrefix(id, "Module:") {
		switch source {
		case "ENGLISH_WIKIPEDIA":
			return search.DocumentReference{
				ID:        nil,
				Temporary: WikipediaTemplateReference + id,
			}
		case "WIKIMEDIA_COMMONS":
			return search.DocumentReference{
				ID:        nil,
				Temporary: WikimediaCommonsTemplateReference + id,
			}
		}
	} else if strings.HasPrefix(id, "File:") {
		return search.DocumentReference{
			ID:        nil,
			Temporary: WikimediaCommonsFileReference + id,
		}
	}

	panic(errors.Errorf("unsupported ID for source \"%s\": %s", source, id))
}

func getDocumentFromESByProp(ctx context.Context, index string, esClient *elastic.Client, property, id string) (*search.Document, *elastic.SearchHit, errors.E) {
	searchResult, err := esClient.Search(index).Query(elastic.NewNestedQuery("claims.id",
		elastic.NewBoolQuery().Must(
			elastic.NewTermQuery("claims.id.prop._id", search.GetCorePropertyID(property).String()),
			elastic.NewTermQuery("claims.id.id", id),
		),
	)).SeqNoAndPrimaryTerm(true).Do(ctx)
	if err != nil {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(err)
	}

	// There might be multiple hits because IDs are not unique (we remove zeroes and do a case insensitive matching).
	for _, hit := range searchResult.Hits.Hits {
		var document search.Document
		err = x.UnmarshalWithoutUnknownFields(hit.Source, &document)
		if err != nil {
			// Caller should add details to the error.
			return nil, nil, errors.WithStack(err)
		}

		// ID is not stored in the document, so we set it here ourselves.
		var errE errors.E
		document.ID, errE = identifier.FromString(hit.Id)
		if errE != nil {
			return nil, nil, errE
		}

		found := false
		for _, claim := range document.Get(search.GetCorePropertyID(property)) {
			if c, ok := claim.(*search.IdentifierClaim); ok && c.Identifier == id {
				found = true
				break
			}
		}

		// If this hit is not precisely for this name, we continue with the next one.
		if !found {
			continue
		}

		return &document, hit, nil
	}

	// Caller should add details to the error.
	return nil, nil, errors.WithStack(NotFoundError)
}

func getDocumentFromESByID(ctx context.Context, index string, esClient *elastic.Client, id identifier.Identifier) (*search.Document, *elastic.GetResult, errors.E) {
	esDoc, err := esClient.Get().Index(index).Id(id.String()).Do(ctx)
	if elastic.IsNotFound(err) {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(NotFoundError)
	} else if err != nil {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(err)
	} else if !esDoc.Found {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(NotFoundError)
	}

	var document search.Document
	err = x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if err != nil {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(err)
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = id

	return &document, esDoc, nil
}

func GetWikidataItem(ctx context.Context, index string, esClient *elastic.Client, id string) (*search.Document, *elastic.SearchHit, errors.E) {
	document, hit, err := getDocumentFromESByProp(ctx, index, esClient, "WIKIDATA_ITEM_ID", id)
	if err != nil {
		errors.Details(err)["entity"] = id
		return nil, nil, err
	}

	return document, hit, nil
}

func clampConfidence(c search.Score) search.Score {
	if c < 0 {
		// max(c, es.HighNegationConfidence).
		if c < es.HighNegationConfidence {
			return es.HighNegationConfidence
		}
		return c
	}
	// min(c, es.HighConfidence).
	if c < es.HighConfidence {
		return c
	}
	return es.HighConfidence
}

func resolveDataTypeFromPropertyDocument(document *search.Document, prop string, valueType *mediawiki.WikiBaseEntityType) (mediawiki.DataType, errors.E) {
	for _, claim := range document.Get(search.GetCorePropertyID("IS")) {
		if c, ok := claim.(*search.RelationClaim); ok {
			for claimType, dataTypes := range claimTypeToDataTypesMap {
				if c.To.ID != nil && *c.To.ID == search.GetCorePropertyID(claimType) {
					if len(dataTypes) == 1 {
						return dataTypes[0], nil
					} else if claimType == "RELATION_CLAIM_TYPE" && valueType != nil {
						switch *valueType {
						case mediawiki.PropertyType:
							return mediawiki.WikiBaseProperty, nil
						case mediawiki.ItemType:
							return mediawiki.WikiBaseItem, nil
						case mediawiki.LexemeType, mediawiki.FormType, mediawiki.SenseType:
							fallthrough
						default:
							err := errors.Errorf("%w: not supported value type", notSupportedDataTypeError)
							errors.Details(err)["prop"] = prop
							return 0, err
						}
					} else {
						err := errors.New("multiple data types, but not RELATION_CLAIM_TYPE")
						errors.Details(err)["prop"] = prop
						return 0, err
					}
				}
			}
		} else {
			err := errors.New("IS claim which is not relation claim")
			errors.Details(err)["prop"] = prop
			return 0, err
		}
	}
	err := errors.Errorf("%w: no suitable IS claim found", notSupportedDataTypeError)
	errors.Details(err)["prop"] = prop
	return 0, err
}

func getDataTypeForProperty(
	ctx context.Context, index string, esClient *elastic.Client, cache *es.Cache,
	prop string, valueType *mediawiki.WikiBaseEntityType,
) (mediawiki.DataType, errors.E) {
	id := GetWikidataDocumentID(prop)

	maybeDocument, ok := cache.Get(id)
	if ok {
		if maybeDocument == nil {
			err := errors.WithStack(NotFoundError)
			errors.Details(err)["prop"] = prop
			return 0, err
		}
		return resolveDataTypeFromPropertyDocument(maybeDocument.(*search.Document), prop, valueType)
	}

	document, _, err := getDocumentFromESByID(ctx, index, esClient, id)
	if errors.Is(err, NotFoundError) {
		cache.Add(id, nil)
		errors.Details(err)["prop"] = prop
		return 0, err
	} else if err != nil {
		errors.Details(err)["prop"] = prop
		return 0, err
	}

	cache.Add(document.ID, document)

	return resolveDataTypeFromPropertyDocument(document, prop, valueType)
}

func getWikiBaseEntityType(value interface{}) *mediawiki.WikiBaseEntityType {
	wikiBaseEntityValue, ok := value.(mediawiki.WikiBaseEntityIDValue)
	if !ok {
		return nil
	}
	return &wikiBaseEntityValue.Type
}

func processSnak( //nolint:ireturn,nolintlint
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, cache *es.Cache,
	namespace uuid.UUID, prop string, idArgs []interface{}, confidence search.Confidence, snak mediawiki.Snak,
) ([]search.Claim, errors.E) {
	id := search.GetID(namespace, idArgs...)

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return []search.Claim{
			&search.UnknownValueClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop, ""),
			},
		}, nil
	case mediawiki.NoValue:
		return []search.Claim{
			&search.NoValueClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop, ""),
			},
		}, nil
	}

	if snak.DataValue == nil {
		return nil, errors.New("nil data value")
	}

	var dataType mediawiki.DataType
	if snak.DataType == nil {
		// Wikimedia Commons might not have the datatype field set, so we have to fetch it ourselves.
		// See: https://phabricator.wikimedia.org/T311977
		var err errors.E
		dataType, err = getDataTypeForProperty(ctx, index, esClient, cache, prop, getWikiBaseEntityType(snak.DataValue.Value))
		if err != nil {
			return nil, errors.WithMessagef(err, "unable to resolve data type for property with value %T", snak.DataValue.Value)
		}
	} else {
		dataType = *snak.DataType
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.ExternalID:
			return []search.Claim{
				&search.IdentifierClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:       getDocumentReference(prop, ""),
					Identifier: string(value),
				},
			}, nil
		case mediawiki.String:
			return []search.Claim{
				&search.StringClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:   getDocumentReference(prop, ""),
					String: string(value),
				},
			}, nil
		case mediawiki.CommonsMedia:
			// First we make sure we do not have underscores.
			title := strings.ReplaceAll(string(value), "_", " ")
			// The first letter has to be upper case.
			title = FirstUpperCase(title)
			title = "File:" + title

			args := append([]interface{}{}, idArgs...)
			args = append(args, "IS", 0, title, 0)
			claimID := search.GetID(namespace, args...)

			return []search.Claim{
				// An invalid claim we post-process later.
				&search.FileClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
						Meta: &search.ClaimTypes{
							Relation: search.RelationClaims{
								{
									CoreClaim: search.CoreClaim{
										ID:         claimID,
										Confidence: es.HighConfidence,
									},
									Prop: search.GetCorePropertyReference("IS"),
									To:   getDocumentReference(title, ""),
								},
							},
						},
					},
					Prop: getDocumentReference(prop, ""),
					Type: "invalid/invalid",
					URL:  "https://xx.invalid",
				},
			}, nil
		case mediawiki.URL:
			return []search.Claim{
				&search.ReferenceClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					IRI:  string(value),
				},
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
			return nil, errors.Errorf("unexpected data type for StringValue: %d", dataType)
		}
	case mediawiki.WikiBaseEntityIDValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.WikiBaseItem:
			if value.Type != mediawiki.ItemType {
				return nil, errors.Errorf("WikiBaseItem data type, but WikiBaseEntityIDValue has type %d, not ItemType", value.Type)
			}
			return []search.Claim{
				&search.RelationClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					To:   getDocumentReference(value.ID, ""),
				},
			}, nil
		case mediawiki.WikiBaseProperty:
			if value.Type != mediawiki.PropertyType {
				return nil, errors.Errorf("WikiBaseProperty data type, but WikiBaseEntityIDValue has type %d, not PropertyType", value.Type)
			}
			return []search.Claim{
				&search.RelationClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					To:   getDocumentReference(value.ID, ""),
				},
			}, nil
		case mediawiki.WikiBaseLexeme:
			return nil, errors.Errorf("%w: WikiBaseLexeme", notSupportedDataTypeError)
		case mediawiki.WikiBaseSense:
			return nil, errors.Errorf("%w: WikiBaseSense", notSupportedDataTypeError)
		case mediawiki.WikiBaseForm:
			return nil, errors.Errorf("%w: WikiBaseForm", notSupportedDataTypeError)
		default:
			return nil, errors.Errorf("unexpected data type for WikiBaseEntityIDValue: %d", dataType)
		}
	case mediawiki.GlobeCoordinateValue:
		return nil, errors.Errorf("%w: GlobeCoordinateValue", notSupportedDataValueTypeError)
	case mediawiki.MonolingualTextValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.MonolingualText:
			if value.Language != "en" && !strings.HasPrefix(value.Language, "en-") {
				return nil, errors.WithStack(errors.BaseWrap(SilentSkippedError, "limited only to English"))
			}
			return []search.Claim{
				&search.TextClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					HTML: search.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
				},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for MonolingualTextValue: %d", dataType)
		}
	case mediawiki.QuantityValue:
		switch dataType { //nolint:exhaustive
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

			var unit search.AmountUnit
			var metaClaims *search.ClaimTypes
			if value.Unit == "1" {
				unit = search.AmountUnitNone
			} else {
				// For now we store the amount as-is and convert to the same unit later on
				// using the unit we store into meta claims.
				// TODO: Implement unit post-processing.
				unit = search.AmountUnitCustom
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
				metaClaims = &search.ClaimTypes{
					Relation: search.RelationClaims{
						{
							CoreClaim: search.CoreClaim{
								ID:         claimID,
								Confidence: es.HighConfidence,
							},
							Prop: search.GetCorePropertyReference("UNIT"),
							To:   getDocumentReference(unitID, ""),
						},
					},
				}
			}

			claims := []search.Claim{
				&search.AmountClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
						Meta:       metaClaims,
					},
					Prop:   getDocumentReference(prop, ""),
					Amount: amount,
					Unit:   unit,
				},
			}
			if uncertaintyLower != nil && uncertaintyUpper != nil {
				// We lower the confidence of the original claim.
				claims[0].(*search.AmountClaim).Confidence *= 0.9
				claims = append(
					claims,
					&search.AmountRangeClaim{
						CoreClaim: search.CoreClaim{
							ID: id,
							// We raise the confidence of the range claim.
							Confidence: clampConfidence(confidence * 1.1), //nolint:gomnd
							Meta:       metaClaims,
						},
						Prop:  getDocumentReference(prop, ""),
						Lower: *uncertaintyLower,
						Upper: *uncertaintyUpper,
						Unit:  unit,
					},
				)
			}

			return claims, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", dataType)
		}
	case mediawiki.TimeValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.Time:
			return []search.Claim{
				// TODO: Convert timestamps in Julian calendar to ones in Gregorian calendar.
				&search.TimeClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:      getDocumentReference(prop, ""),
					Timestamp: search.Timestamp(value.Time),
					Precision: search.TimePrecision(value.Precision),
				},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", dataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func addQualifiers(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, cache *es.Cache, namespace uuid.UUID,
	claim search.Claim, entityID, prop, statementID string, qualifiers map[string][]mediawiki.Snak, qualifiersOrder []string,
) errors.E {
	for _, p := range qualifiersOrder {
		for i, qualifier := range qualifiers[p] {
			qualifierClaims, err := processSnak(
				ctx, index, log, esClient, cache, namespace, p, []interface{}{entityID, prop, statementID, "qualifier", p, i}, es.MediumConfidence, qualifier,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			for j, qualifierClaim := range qualifierClaims {
				err = claim.AddMeta(qualifierClaim)
				if err != nil {
					log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i).Int(j)).
						Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
				}
			}
		}
	}
	return nil
}

// addReference operates in two modes. In the first mode, when there is only one snak type per reference, it just converts those snaks to claims.
// In the second mode, when there are multiple snak types, it wraps them into a temporary WIKIDATA_REFERENCE claim which will be processed later.
// TODO: Implement post-processing of temporary WIKIDATA_REFERENCE claims.
func addReference(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, cache *es.Cache, namespace uuid.UUID,
	claim search.Claim, entityID, prop, statementID string, i int, reference mediawiki.Reference,
) errors.E {
	// Edge case.
	if len(reference.SnaksOrder) == 0 {
		return nil
	}

	var referenceClaim search.Claim

	if len(reference.SnaksOrder) == 1 {
		referenceClaim = claim
	} else {
		referenceClaim = &search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entityID, prop, statementID, "reference", i, "WIKIDATA_REFERENCE", 0),
				Confidence: es.NoConfidence,
			},
			Prop: search.GetCorePropertyReference("WIKIDATA_REFERENCE"),
			HTML: search.TranslatableHTMLString{
				"XX": html.EscapeString("A temporary group of multiple Wikidata reference statements for later processing."),
			},
		}
	}

	for _, property := range reference.SnaksOrder {
		for j, snak := range reference.Snaks[property] {
			cs, err := processSnak(
				ctx, index, log, esClient, cache, namespace, property, []interface{}{entityID, prop, statementID, "reference", i, property, j}, es.MediumConfidence, snak,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			for k, c := range cs {
				err = referenceClaim.AddMeta(c)
				if err != nil {
					log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j).Int(k)).
						Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
				}
			}
		}
	}

	if len(reference.SnaksOrder) > 1 {
		err := claim.AddMeta(referenceClaim)
		if err != nil {
			log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i)).
				Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
		}
	}

	return nil
}

// ConvertEntity converts both Wikidata entities and Wikimedia Commons entities.
// Entities can reference only Wikimedia Commons files and not Wikipedia files.
func ConvertEntity(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, cache *es.Cache,
	namespace uuid.UUID, entity mediawiki.Entity,
) (*search.Document, errors.E) {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English Wikidata entities for now.
	// We do not require from Wikimedia Commons to have labels, so we skip the check for them.
	if len(englishLabels) == 0 && entity.Type != mediawiki.MediaInfo {
		if entity.Type == mediawiki.Property {
			// But properties should all have English label, so we warn here.
			log.Warn().Str("entity", entity.ID).Msg("property is missing a label in English")
		}
		return nil, errors.WithStack(errors.BaseWrap(SilentSkippedError, "limited only to English"))
	}

	var id identifier.Identifier
	var name string
	var filename string
	if entity.Type == mediawiki.MediaInfo {
		filename = strings.TrimPrefix(entity.Title, "File:")
		filename = strings.ReplaceAll(filename, " ", "_")
		filename = FirstUpperCase(filename)

		id = search.GetID(NameSpaceWikimediaCommonsFile, filename)

		// We make a name from the title by removing prefix and file extension.
		name = strings.TrimPrefix(entity.Title, "File:")
		name = strings.TrimSuffix(name, path.Ext(name))
	} else {
		id = GetWikidataDocumentID(entity.ID)

		// We simply use the first label we have. We do not remove it from englishLabels
		// so that deduplication works correctly, but we check later on that we are not
		// adding any NAME claim equal to name.
		name = englishLabels[0]

		// Remove prefix if it is a template, module, or category entity.
		name = strings.TrimPrefix(name, "Template:")
		name = strings.TrimPrefix(name, "Module:")
		name = strings.TrimPrefix(name, "Category:")
	}

	// TODO: Set mnemonic if it is a property and the name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere. To every claim from input entity?
	document := search.Document{
		CoreDocument: search.CoreDocument{
			ID:    id,
			Score: 0.5,
		},
	}

	err := document.Add(&search.TextClaim{
		CoreClaim: search.CoreClaim{
			ID: search.GetID(namespace, entity.ID, "NAME", 0),
			// The first added English label is added as a high confidence claim.
			Confidence: es.HighConfidence,
		},
		Prop: search.GetCorePropertyReference("NAME"),
		HTML: search.TranslatableHTMLString{
			"en": html.EscapeString(name),
		},
	})
	if err != nil {
		log.Error().Str("entity", entity.ID).Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
	}

	if entity.Type == mediawiki.Property {
		document.Claims = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_ID", 0),
						Confidence: es.HighConfidence,
					},
					Prop:       search.GetCorePropertyReference("WIKIDATA_PROPERTY_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
						Confidence: es.HighConfidence,
					},
					Prop: search.GetCorePropertyReference("WIKIDATA_PROPERTY_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/Property:%s", entity.ID),
				},
			},
			Relation: search.RelationClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "IS", 0, "PROPERTY", 0),
						Confidence: es.HighConfidence,
					},
					Prop: search.GetCorePropertyReference("IS"),
					To:   search.GetCorePropertyReference("PROPERTY"),
				},
			},
		}
	} else if entity.Type == mediawiki.Item {
		document.Claims = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_ITEM_ID", 0),
						Confidence: es.HighConfidence,
					},
					Prop:       search.GetCorePropertyReference("WIKIDATA_ITEM_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_ITEM_PAGE", 0),
						Confidence: es.HighConfidence,
					},
					Prop: search.GetCorePropertyReference("WIKIDATA_ITEM_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/%s", entity.ID),
				},
			},
			Relation: search.RelationClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "IS", 0, "ITEM", 0),
						Confidence: es.HighConfidence,
					},
					Prop: search.GetCorePropertyReference("IS"),
					To:   search.GetCorePropertyReference("ITEM"),
				},
			},
		}
	} else if entity.Type == mediawiki.MediaInfo {
		// It is expected that this document will be merged with another document with standard
		// file claims, so the claims here are just a set of additional claims to be added and
		// are missing standard file claims.
		document.Claims = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIMEDIA_COMMONS_ENTITY_ID", 0),
						Confidence: es.HighConfidence,
					},
					Prop:       search.GetCorePropertyReference("WIKIMEDIA_COMMONS_ENTITY_ID"),
					Identifier: entity.ID,
				},
			},
		}
	} else {
		return nil, errors.Errorf(`entity has invalid type: %d`, entity.Type)
	}

	// Exists for Wikidata entities.
	for _, site := range []struct {
		Wiki           string
		MnemonicPrefix string
		Domain         string
	}{
		{
			"enwiki",
			"ENGLISH_WIKIPEDIA",
			"en.wikipedia.org",
		},
		{
			"commonswiki",
			"WIKIMEDIA_COMMONS",
			"commons.wikimedia.org",
		},
	} {
		siteLink, ok := entity.SiteLinks[site.Wiki]
		if ok {
			url := siteLink.URL
			if url == "" {
				// First we make sure we do not have spaces.
				urlTitle := strings.ReplaceAll(siteLink.Title, " ", "_")
				// The first letter has to be upper case.
				urlTitle = FirstUpperCase(urlTitle)
				url = fmt.Sprintf("https://%s/wiki/%s", site.Domain, urlTitle)
			}
			for _, namespace := range nonMainNamespaces {
				if strings.HasPrefix(siteLink.Title, namespace) {
					// Only items have sitelinks. We want only items related to main articles (main namespace),
					// templates, modules, and categories.
					errE := errors.WithStack(errors.BaseWrap(SilentSkippedError, "`limited only to items related to main articles, templates, and categories"))
					errors.Details(errE)["wiki"] = site.Wiki
					errors.Details(errE)["title"] = siteLink.Title
					return nil, errE
				}
			}
			document.Claims.Identifier = append(document.Claims.Identifier, search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(namespace, entity.ID, site.MnemonicPrefix+"_PAGE_TITLE", 0),
					Confidence: es.HighConfidence,
				},
				Prop:       search.GetCorePropertyReference(site.MnemonicPrefix + "_PAGE_TITLE"),
				Identifier: siteLink.Title,
			})
			document.Claims.Reference = append(document.Claims.Reference, search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(namespace, entity.ID, site.MnemonicPrefix+"_PAGE", 0),
					Confidence: es.HighConfidence,
				},
				Prop: search.GetCorePropertyReference(site.MnemonicPrefix + "_PAGE"),
				IRI:  url,
			})
			// Here we add English Wikipedia page title to labels to be included as another NAME claim on the document.
			if site.Wiki == "enwiki" {
				englishLabels = append(englishLabels, siteLink.Title)
			}
		}
	}

	if entity.DataType != nil {
		// Which claim type should be used with this property?
		// We use this in resolveDataTypeFromPropertyDocument, too.
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			document.Claims.Relation = append(document.Claims.Relation, search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID: search.GetID(namespace, entity.ID, "IS", 0, claimTypeMnemonic, 0),
					// We have low confidence in this claim. Later on we augment it using statistics
					// on how are properties really used.
					// TODO: Decide what should really be confidence here or implement "later on" part described above.
					Confidence: es.LowConfidence,
				},
				Prop: search.GetCorePropertyReference("IS"),
				To:   search.GetCorePropertyReference(claimTypeMnemonic),
			})
		}
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	englishLabels = append(englishLabels, englishAliases...)
	for i, label := range englishLabels {
		// Remove prefix if it is a template, module, or category entity.
		label = strings.TrimPrefix(label, "Template:")
		label = strings.TrimPrefix(label, "Module:")
		label = strings.TrimPrefix(label, "Category:")
		englishLabels[i] = label
	}
	englishLabels = deduplicate(englishLabels)
	for i, label := range englishLabels {
		// NAME claim with name value has already been added, so we skip it.
		if label == name {
			continue
		}

		document.Claims.Text = append(document.Claims.Text, search.TextClaim{
			CoreClaim: search.CoreClaim{
				// We add +1 to i to make sure we do not repeat claim ID (we use the same form for name value NAME claim).
				ID: search.GetID(namespace, entity.ID, "NAME", i+1),
				// Other English labels and aliases are added with the medium confidence.
				Confidence: es.MediumConfidence,
			},
			Prop: search.GetCorePropertyReference("NAME"),
			HTML: search.TranslatableHTMLString{
				"en": html.EscapeString(label),
			},
		})
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	for i, description := range englishDescriptions {
		document.Claims.Text = append(document.Claims.Text, search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entity.ID, "DESCRIPTION", i),
				Confidence: es.MediumConfidence,
			},
			Prop: search.GetCorePropertyReference("DESCRIPTION"),
			HTML: search.TranslatableHTMLString{
				"en": html.EscapeString(description),
			},
		})
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
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Int(i)).Msg("missing a statement ID")
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claims, err := processSnak(
				ctx, index, log, esClient, cache, namespace, prop, []interface{}{entity.ID, prop, statement.ID, "mainsnak"}, confidence, statement.MainSnak,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			for j, claim := range claims {
				err = addQualifiers(
					ctx, index, log, esClient, cache, namespace, claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder,
				)
				if err != nil {
					log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j).Str("qualifiers")).
						Err(err).Fields(errors.AllDetails(err)).Send()
					continue
				}
				for i, reference := range statement.References {
					err = addReference(ctx, index, log, esClient, cache, namespace, claim, entity.ID, prop, statement.ID, i, reference)
					if err != nil {
						log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j).Str("reference").Int(i)).
							Err(err).Fields(errors.AllDetails(err)).Send()
						continue
					}
				}
				err = document.Add(claim)
				if err != nil {
					log.Error().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j)).
						Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
				}
			}
		}
	}

	return &document, nil
}
