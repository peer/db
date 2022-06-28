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
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

const (
	HighConfidence   = 1.0
	MediumConfidence = 0.5
	LowConfidence    = 0.0
	NoConfidence     = -1.0
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedDataValueTypeError = errors.BaseWrap(SilentSkippedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(SilentSkippedError, "not supported data type")
	NotFoundError                  = errors.Base("not found")

	// Besides main Wikipedia namespace we allow also templates, modules, and categories.
	nonMainWikipediaNamespaces = []string{
		"User:",
		"Wikipedia:",
		"File:",
		"MediaWiki:",
		"Help:",
		"Portal:",
		"Draft:",
		"TimedText:",
	}
)

func GetWikidataDocumentID(id string) search.Identifier {
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
		return HighConfidence
	case mediawiki.Normal:
		return MediumConfidence
	case mediawiki.Deprecated:
		return NoConfidence
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// It does not return a valid reference: name is set to the ID itself for the language "XX".
// This works correctly only for Wikidata references. If it is used with Wikimedia Commons reference,
// it will generate a reference without ID.
func getDocumentReference(id string) search.DocumentReference {
	if strings.HasPrefix(id, "M") {
		return search.DocumentReference{
			Name: map[string]string{
				"XX": id,
			},
			Score: NoConfidence,
		}
	}

	return search.DocumentReference{
		ID: GetWikidataDocumentID(id),
		Name: map[string]string{
			"XX": id,
		},
		Score: NoConfidence,
	}
}

func getDocumentFromES(ctx context.Context, esClient *elastic.Client, property, id string) (*search.Document, *elastic.SearchHit, errors.E) {
	searchResult, err := esClient.Search("docs").Query(elastic.NewNestedQuery("active.id",
		elastic.NewBoolQuery().Must(
			elastic.NewTermQuery("active.id.prop._id", search.GetStandardPropertyID(property)),
			elastic.NewTermQuery("active.id.id", id),
		),
	)).SeqNoPrimaryTerm(true).Do(ctx)
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
		document.ID = search.Identifier(hit.Id)

		found := false
		for _, claim := range document.Get(search.GetStandardPropertyID(property)) {
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

// TODO: Should we use cache for cases where item has not been found?
//       Currently we use the function in the context where every item document is fetched
//       only once, one after the other, so caching will not help.

// We do follow a redirect, because currently we use the function in
// the context where we want the target document (to add its article).
func GetWikidataItem(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, token string, apiLimit int, id string,
) (*search.Document, *elastic.SearchHit, string, errors.E) {
	document, hit, err := getDocumentFromES(ctx, esClient, "WIKIDATA_ITEM_ID", id)
	if errors.Is(err, NotFoundError) {
		// Passthrough.
	} else if err != nil {
		errors.Details(err)["entity"] = id
		return nil, nil, "", err
	} else {
		return document, hit, "", nil
	}

	// We could not find the item. Maybe there is a redirect?
	ii, err := GetImageInfo(ctx, httpClient, "www.wikidata.org", token, apiLimit, id)
	if err != nil {
		// Not found error here probably means that the item has been deleted recently.
		errE := errors.WithMessage(err, "checking for redirect")
		errors.Details(errE)["entity"] = id
		return nil, nil, "", errE
	} else if ii.Redirect == "" {
		// No redirect.
		errE := errors.WithStack(NotFoundError)
		errors.Details(errE)["entity"] = id
		return nil, nil, "", errE
	}

	document, hit, err = getDocumentFromES(ctx, esClient, "WIKIDATA_ITEM_ID", ii.Redirect)
	if err != nil {
		errE := errors.WithMessage(err, "after redirect")
		errors.Details(errE)["entity"] = id
		errors.Details(errE)["redirect"] = ii.Redirect
		return nil, nil, "", errE
	}

	// There is nothing to do about it. This is an artifact of items being merged.
	log.Debug().Str("entity", id).Str("redirect", ii.Redirect).Msg("item redirects")

	return document, hit, ii.Redirect, nil
}

func processSnak( //nolint:ireturn,nolintlint
	ctx context.Context, log zerolog.Logger, namespace uuid.UUID, prop string, idArgs []interface{}, confidence search.Confidence, snak mediawiki.Snak,
) (search.Claim, errors.E) {
	id := search.GetID(namespace, idArgs...)

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
		return nil, errors.New("nil data value")
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch snak.DataType { //nolint:exhaustive
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
			// First we make sure we do not have spaces.
			filename := strings.ReplaceAll(string(value), " ", "_")
			// The first letter has to be upper case.
			filename = FirstUpperCase(filename)

			// First we make sure we do not have underscores.
			title := strings.ReplaceAll(filename, "_", " ")
			// The first letter has to be upper case.
			title = FirstUpperCase(title)
			title = "File:" + title

			args := append([]interface{}{}, idArgs...)
			args = append(args, "IS", 0)
			claimID := search.GetID(namespace, args...)

			// An invalid reference we post-process later.
			return &search.FileClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
					Meta: &search.ClaimTypes{
						Is: search.IsClaims{
							{
								CoreClaim: search.CoreClaim{
									ID:         claimID,
									Confidence: NoConfidence,
								},
								To: search.DocumentReference{
									ID: search.GetID(NameSpaceWikimediaCommonsFile, title),
									Name: map[string]string{
										"XX": filename,
									},
									Score: NoConfidence,
								},
							},
						},
					},
				},
				Prop: getDocumentReference(prop),
				Type: "invalid/invalid",
				URL:  "https://xx.invalid",
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
		switch snak.DataType { //nolint:exhaustive
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
		switch snak.DataType { //nolint:exhaustive
		case mediawiki.MonolingualText:
			if value.Language != "en" && !strings.HasPrefix(value.Language, "en-") {
				return nil, errors.WithStack(errors.BaseWrap(SilentSkippedError, "limited only to English"))
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
		switch snak.DataType { //nolint:exhaustive
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
				// TODO: Implement unit post-processing.
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
				claim.CoreClaim.Meta = &search.ClaimTypes{
					Relation: search.RelationClaims{
						{
							CoreClaim: search.CoreClaim{
								ID:         claimID,
								Confidence: HighConfidence,
							},
							Prop: search.GetStandardPropertyReference("UNIT"),
							To:   getDocumentReference(unitID),
						},
					},
				}
			}

			return &claim, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType { //nolint:exhaustive
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
	ctx context.Context, log zerolog.Logger, namespace uuid.UUID, claim search.Claim, entityID, prop, statementID string,
	qualifiers map[string][]mediawiki.Snak, qualifiersOrder []string,
) errors.E {
	for _, p := range qualifiersOrder {
		for i, qualifier := range qualifiers[p] {
			qualifierClaim, err := processSnak(
				ctx, log, namespace, p, []interface{}{entityID, prop, statementID, "qualifier", p, i}, MediumConfidence, qualifier,
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
			err = claim.AddMeta(qualifierClaim)
			if err != nil {
				log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
			}
		}
	}
	return nil
}

// addReference operates in two modes. In the first mode, when there is only one snak type per reference, it just converts those snaks to claims.
// In the second mode, when there are multiple snak types, it wraps them into a temporary WIKIDATA_REFERENCE claim which will be processed later.
// TODO: Implement post-processing of temporary WIKIDATA_REFERENCE claims.
func addReference(
	ctx context.Context, log zerolog.Logger, namespace uuid.UUID, claim search.Claim, entityID, prop, statementID string, i int, reference mediawiki.Reference,
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
				Confidence: NoConfidence,
			},
			Prop: search.GetStandardPropertyReference("WIKIDATA_REFERENCE"),
			HTML: search.TranslatableHTMLString{
				"XX": html.EscapeString("A temporary group of multiple Wikidata reference statements for later processing."),
			},
		}
	}

	for _, property := range reference.SnaksOrder {
		for j, snak := range reference.Snaks[property] {
			c, err := processSnak(
				ctx, log, namespace, property, []interface{}{entityID, prop, statementID, "reference", i, property, j}, MediumConfidence, snak,
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
			err = referenceClaim.AddMeta(c)
			if err != nil {
				log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j)).
					Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
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
	ctx context.Context, log zerolog.Logger, namespace uuid.UUID, entity mediawiki.Entity,
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

	var id search.Identifier
	var name string
	if entity.Type == mediawiki.MediaInfo {
		id = search.GetID(NameSpaceWikimediaCommonsFile, entity.Title)

		// We make a name from the title by removing prefix and file extension.
		name = strings.TrimPrefix(entity.Title, "File:")
		name = strings.TrimSuffix(name, path.Ext(name))
	} else {
		id = GetWikidataDocumentID(entity.ID)

		// We simply use the first label we have.
		name = englishLabels[0]
		englishLabels = englishLabels[1:]

		// Remove prefix if it is a template, module, or category entity.
		name = strings.TrimPrefix(name, "Template:")
		name = strings.TrimPrefix(name, "Module:")
		name = strings.TrimPrefix(name, "Category:")
	}

	// TODO: Set mnemonic if a property and the name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere. To every claim from input entity?
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
		document.Active = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_ID", 0),
						Confidence: HighConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
						Confidence: HighConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIDATA_PROPERTY_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/Property:%s", entity.ID),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "PROPERTY", 0),
						Confidence: HighConfidence,
					},
					To: search.GetStandardPropertyReference("PROPERTY"),
				},
			},
		}
	} else if entity.Type == mediawiki.Item {
		document.Active = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_ITEM_ID", 0),
						Confidence: HighConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIDATA_ITEM_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIDATA_ITEM_PAGE", 0),
						Confidence: HighConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIDATA_ITEM_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/%s", entity.ID),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "ITEM", 0),
						Confidence: HighConfidence,
					},
					To: search.GetStandardPropertyReference("ITEM"),
				},
			},
		}
	} else if entity.Type == mediawiki.MediaInfo {
		filename := strings.TrimPrefix(entity.Title, "File:")
		filename = strings.ReplaceAll(filename, " ", "_")
		filename = FirstUpperCase(filename)

		prefix := GetMediawikiFilePrefix(filename)

		document.Active = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIMEDIA_COMMONS_ENTITY_ID", 0),
						Confidence: HighConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_ENTITY_ID"),
					Identifier: entity.ID,
				},
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIMEDIA_COMMONS_FILE_NAME", 0),
						Confidence: HighConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE_NAME"),
					Identifier: filename,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "WIKIMEDIA_COMMONS_FILE", 0),
						Confidence: HighConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE"),
					IRI:  fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", filename),
				},
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "FILE_URL", 0),
						Confidence: HighConfidence,
					},
					Prop: search.GetStandardPropertyReference("FILE_URL"),
					IRI:  fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/%s/%s", prefix, filename),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, entity.ID, "FILE", 0),
						Confidence: HighConfidence,
					},
					To: search.GetStandardPropertyReference("FILE"),
				},
			},
		}
	} else {
		return nil, errors.Errorf(`entity has invalid type: %d`, entity.Type)
	}

	// Exists for Wikidata entities.
	siteLink, ok := entity.SiteLinks["enwiki"]
	if ok {
		url := siteLink.URL
		if url == "" {
			// First we make sure we do not have spaces.
			urlTitle := strings.ReplaceAll(siteLink.Title, " ", "_")
			// The first letter has to be upper case.
			urlTitle = FirstUpperCase(urlTitle)
			url = fmt.Sprintf("https://en.wikipedia.org/wiki/%s", urlTitle)
		}
		for _, namespace := range nonMainWikipediaNamespaces {
			if strings.HasPrefix(siteLink.Title, namespace) {
				// Only items have sitelinks. We want only items related to main Wikipedia articles (main namespace),
				// templates, modules, and categories.
				errE := errors.WithStack(errors.BaseWrap(SilentSkippedError, "`limited only to items related to main Wikipedia articles, templates, and categories"))
				errors.Details(errE)["title"] = siteLink.Title
				return nil, errE
			}
		}
		document.Active.Identifier = append(document.Active.Identifier, search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE_TITLE", 0),
				Confidence: HighConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE_TITLE"),
			Identifier: siteLink.Title,
		})
		document.Active.Reference = append(document.Active.Reference, search.ReferenceClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE", 0),
				Confidence: HighConfidence,
			},
			Prop: search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE"),
			IRI:  url,
		})
	}

	if entity.DataType != nil {
		// Which claim type should be used with this property?
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			document.Active.Is = append(document.Active.Is, search.IsClaim{
				CoreClaim: search.CoreClaim{
					ID: search.GetID(namespace, entity.ID, claimTypeMnemonic, 0),
					// We have low confidence in this claim. Later on we augment it using statistics
					// on how are properties really used.
					// TODO: Decide what should really be confidence here or implement "later on" part described above.
					Confidence: LowConfidence,
				},
				To: search.GetStandardPropertyReference(claimTypeMnemonic),
			})
		}
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	englishLabels = append(englishLabels, englishAliases...)
	englishLabels = deduplicate(englishLabels)
	for i, label := range englishLabels {
		document.Active.Text = append(document.Active.Text, search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entity.ID, "ALSO_KNOWN_AS", i),
				Confidence: HighConfidence,
			},
			Prop: search.GetStandardPropertyReference("ALSO_KNOWN_AS"),
			HTML: search.TranslatableHTMLString{
				"en": html.EscapeString(label),
			},
		})
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	for i, description := range englishDescriptions {
		document.Active.Text = append(document.Active.Text, search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, entity.ID, "DESCRIPTION", i),
				Confidence: MediumConfidence,
			},
			Prop: search.GetStandardPropertyReference("DESCRIPTION"),
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
			claim, err := processSnak(
				ctx, log, namespace, prop, []interface{}{entity.ID, prop, statement.ID, "mainsnak"}, confidence, statement.MainSnak,
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
			err = addQualifiers(
				ctx, log, namespace, claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder,
			)
			if err != nil {
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("qualifiers")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			for i, reference := range statement.References {
				err = addReference(ctx, log, namespace, claim, entity.ID, prop, statement.ID, i, reference)
				if err != nil {
					log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("reference").Int(i)).
						Err(err).Fields(errors.AllDetails(err)).Send()
					continue
				}
			}
			err = document.Add(claim)
			if err != nil {
				log.Error().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID)).
					Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
			}
		}
	}

	return &document, nil
}
