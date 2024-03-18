package wikipedia

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
)

var (
	referenceNotFoundError  = errors.Base("document reference to a nonexistent document")
	referenceTemporaryError = errors.Base("document reference is a temporary reference")
)

type updateEmbeddedDocumentsVisitor struct {
	Context                      context.Context
	Log                          zerolog.Logger
	Index                        string
	Cache                        *es.Cache
	SkippedWikidataEntities      *sync.Map
	SkippedWikimediaCommonsFiles *sync.Map
	ESClient                     *elastic.Client
	Changed                      int
	DocumentID                   identifier.Identifier
	EntityIDs                    []string
}

func (v *updateEmbeddedDocumentsVisitor) makeError(err error, ref search.DocumentReference, claimID identifier.Identifier) errors.E {
	errE := errors.WithStack(err)
	details := errors.Details(errE)
	details["doc"] = v.DocumentID.String()
	if len(v.EntityIDs) == 1 {
		details["entity"] = v.EntityIDs[0]
	} else if len(v.EntityIDs) > 1 {
		details["entity"] = v.EntityIDs
	}
	details["claim"] = claimID.String()
	if ref.ID != nil {
		details["ref"] = ref.ID.String()
	} else {
		prop, id := v.getOriginalID(ref.Temporary)
		if prop != "" && id != "" {
			details["name"] = id
		}
	}
	return errE
}

func (v *updateEmbeddedDocumentsVisitor) logWarning(fileDoc *search.Document, claimID identifier.Identifier, msg string) {
	l := v.Log.Warn().Str("doc", v.DocumentID.String())
	if len(v.EntityIDs) == 1 {
		l = l.Str("entity", v.EntityIDs[0])
	} else if len(v.EntityIDs) > 1 {
		l = l.Strs("entity", v.EntityIDs)
	}
	l = l.Str("claim", claimID.String())
	l = l.Str("ref", fileDoc.ID.String())
	l.Msg(msg)
}

func (v *updateEmbeddedDocumentsVisitor) handleError(err errors.E, ref search.DocumentReference) (search.VisitResult, errors.E) {
	if errors.Is(err, referenceNotFoundError) {
		if ref.ID != nil {
			if _, ok := v.SkippedWikidataEntities.Load(ref.ID.String()); ok {
				v.Log.Debug().Err(err).Fields(errors.AllDetails(err)).Send()
			} else {
				v.Log.Warn().Err(err).Fields(errors.AllDetails(err)).Send()
			}
		} else {
			prop, id := v.getOriginalID(ref.Temporary)
			if prop == "WIKIMEDIA_COMMONS_FILE_NAME" {
				if _, ok := v.SkippedWikimediaCommonsFiles.Load(id); ok {
					v.Log.Debug().Err(err).Fields(errors.AllDetails(err)).Send()
				} else {
					v.Log.Warn().Err(err).Fields(errors.AllDetails(err)).Send()
				}
			} else {
				name, ok := errors.AllDetails(err)["name"].(string)
				if ok && (strings.HasPrefix(name, "Template:") || strings.HasPrefix(name, "Module:") || strings.HasPrefix(name, "Category:")) {
					v.Log.Debug().Err(err).Fields(errors.AllDetails(err)).Send()
				} else {
					v.Log.Warn().Err(err).Fields(errors.AllDetails(err)).Send()
				}
			}
		}
		v.Changed++
		return search.Drop, nil
	}
	return search.Keep, err
}

func (v *updateEmbeddedDocumentsVisitor) getOriginalID(temporary string) (string, string) {
	if strings.HasPrefix(temporary, WikipediaCategoryReference) {
		return "ENGLISH_WIKIPEDIA_PAGE_TITLE", strings.TrimPrefix(temporary, WikipediaCategoryReference)
	} else if strings.HasPrefix(temporary, WikipediaTemplateReference) {
		return "ENGLISH_WIKIPEDIA_PAGE_TITLE", strings.TrimPrefix(temporary, WikipediaTemplateReference)
	} else if strings.HasPrefix(temporary, WikimediaCommonsCategoryReference) {
		return "WIKIMEDIA_COMMONS_PAGE_TITLE", strings.TrimPrefix(temporary, WikimediaCommonsCategoryReference)
	} else if strings.HasPrefix(temporary, WikimediaCommonsTemplateReference) {
		return "WIKIMEDIA_COMMONS_PAGE_TITLE", strings.TrimPrefix(temporary, WikimediaCommonsTemplateReference)
	} else if strings.HasPrefix(temporary, WikimediaCommonsFileReference) {
		filename := strings.TrimPrefix(strings.TrimPrefix(temporary, WikimediaCommonsFileReference), "File:")
		filename = strings.ReplaceAll(filename, " ", "_")
		filename = FirstUpperCase(filename)
		return "WIKIMEDIA_COMMONS_FILE_NAME", filename
	} else if strings.HasPrefix(temporary, WikimediaCommonsEntityReference) {
		return "WIKIMEDIA_COMMONS_ENTITY_ID", strings.TrimPrefix(temporary, WikimediaCommonsEntityReference)
	}

	return "", ""
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReference(ref search.DocumentReference, claimID identifier.Identifier) (*search.DocumentReference, errors.E) {
	if ref.ID != nil {
		return v.getDocumentReferenceByID(ref, claimID)
	}

	prop, id := v.getOriginalID(ref.Temporary)
	if prop != "" && id != "" {
		return v.getDocumentReferenceByProp(ref, claimID, prop, id)
	}

	errE := errors.Errorf("invalid reference")
	body, err := x.MarshalWithoutEscapeHTML(ref)
	if err == nil {
		errors.Details(errE)["body"] = string(body)
	}
	return nil, errE
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentByProp(property, title string) (*search.Document, errors.E) {
	// Here we check cache with string type, so values cannot conflict with caching done
	// by getDocumentByID, which uses Identifier type.
	maybeDocument, ok := v.Cache.Get(title)
	if ok {
		if maybeDocument == nil {
			return nil, errors.WithStack(NotFoundError)
		}
		return maybeDocument.(*search.Document), nil
	}

	document, _, err := getDocumentFromESByProp(v.Context, v.Index, v.ESClient, property, title)
	if errors.Is(err, NotFoundError) {
		v.Cache.Add(title, nil)
		return nil, err
	} else if err != nil {
		return nil, err
	}

	v.Cache.Add(title, document)
	v.Cache.Add(document.ID, document)

	return document, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentByID(id identifier.Identifier) (*search.Document, errors.E) {
	// Here we check cache with Identifier type, so values cannot conflict with caching done
	// by getDocumentByTitle, which uses string type.
	maybeDocument, ok := v.Cache.Get(id)
	if ok {
		if maybeDocument == nil {
			return nil, errors.WithStack(NotFoundError)
		}
		return maybeDocument.(*search.Document), nil
	}

	document, _, err := getDocumentFromESByID(v.Context, v.Index, v.ESClient, id)
	if errors.Is(err, NotFoundError) {
		v.Cache.Add(id, nil)
		return nil, err
	} else if err != nil {
		return nil, err
	}

	v.Cache.Add(document.ID, document)

	return document, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReferenceByProp(
	ref search.DocumentReference, claimID identifier.Identifier, property, title string,
) (*search.DocumentReference, errors.E) {
	document, err := v.getDocumentByProp(property, title)
	if errors.Is(err, NotFoundError) {
		return nil, v.makeError(referenceNotFoundError, ref, claimID)
	} else if err != nil {
		return nil, v.makeError(err, ref, claimID)
	}

	res := document.Reference()

	return &res, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReferenceByID(ref search.DocumentReference, claimID identifier.Identifier) (*search.DocumentReference, errors.E) {
	if ref.ID == nil {
		return nil, v.makeError(referenceTemporaryError, ref, claimID)
	}

	document, err := v.getDocumentByID(*ref.ID)
	if errors.Is(err, NotFoundError) {
		return nil, v.makeError(referenceNotFoundError, ref, claimID)
	} else if err != nil {
		return nil, v.makeError(err, ref, claimID)
	}

	res := document.Reference()

	return &res, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitIdentifier(claim *search.IdentifierClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitReference(claim *search.ReferenceClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitText(claim *search.TextClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitString(claim *search.StringClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitAmount(claim *search.AmountClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitAmountRange(claim *search.AmountRangeClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitRelation(claim *search.RelationClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	ref, err = v.getDocumentReference(claim.To, claim.ID)
	if err != nil {
		return v.handleError(err, claim.To)
	}

	if !reflect.DeepEqual(&claim.To, ref) {
		claim.To = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitNoValue(claim *search.NoValueClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitUnknownValue(claim *search.UnknownValueClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitTime(claim *search.TimeClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitTimeRange(claim *search.TimeRangeClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitFile(claim *search.FileClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return v.handleError(err, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	var fileDocument *search.Document
	for _, cc := range claim.GetMeta(search.GetCorePropertyID("IS")) {
		if c, ok := cc.(*search.RelationClaim); ok {
			// c.To.ID should be non-nil ID because we called claim.VisitMeta(v) above.
			fileDocument, err = v.getDocumentByID(*c.To.ID)
			if errors.Is(err, NotFoundError) {
				return v.handleError(v.makeError(referenceNotFoundError, c.To, c.ID), c.To)
			} else if err != nil {
				return v.handleError(v.makeError(err, c.To, c.ID), c.To)
			}

			break
		}
	}

	if fileDocument != nil {
		var mediaType string
		for _, cc := range fileDocument.Get(search.GetCorePropertyID("MEDIA_TYPE")) {
			if c, ok := cc.(*search.StringClaim); ok {
				mediaType = c.String
				break
			}
		}
		if mediaType == "" {
			v.logWarning(fileDocument, claim.ID, "referenced Wikimedia commons file document is missing a MEDIA_TYPE string claim")
			v.Changed++
			return search.Drop, nil
		}

		if !reflect.DeepEqual(claim.Type, mediaType) {
			claim.Type = mediaType
			v.Changed++
		}

		var fileURL string
		for _, cc := range fileDocument.Get(search.GetCorePropertyID("FILE_URL")) {
			if c, ok := cc.(*search.ReferenceClaim); ok {
				fileURL = c.IRI
				break
			}
		}
		if fileURL == "" {
			v.logWarning(fileDocument, claim.ID, "referenced Wikimedia commons file document is missing a FILE_URL reference claim")
			v.Changed++
			return search.Drop, nil
		}

		if !reflect.DeepEqual(claim.URL, fileURL) {
			claim.URL = fileURL
			v.Changed++
		}

		// TODO: First extract individual lists, then sort each least by order, and then concatenate lists.
		var previews []string
		for _, cc := range fileDocument.Get(search.GetCorePropertyID("PREVIEW_URL")) {
			if c, ok := cc.(*search.ReferenceClaim); ok {
				previews = append(previews, c.IRI)
			}
		}

		if !reflect.DeepEqual(claim.Preview, previews) {
			claim.Preview = previews
			v.Changed++
		}
	}

	return search.Keep, nil
}

func UpdateEmbeddedDocuments(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client, cache *es.Cache,
	skippedWikidataEntities *sync.Map, skippedWikimediaCommonsFiles *sync.Map, document *search.Document,
) (bool, errors.E) {
	// We try to obtain unhashed document IDs to use in logging.
	entityIDClaims := []search.Claim{}
	entityIDClaims = append(entityIDClaims, document.Get(search.GetCorePropertyID("WIKIDATA_ITEM_ID"))...)
	entityIDClaims = append(entityIDClaims, document.Get(search.GetCorePropertyID("WIKIDATA_PROPERTY_ID"))...)
	entityIDClaims = append(entityIDClaims, document.Get(search.GetCorePropertyID("WIKIMEDIA_COMMONS_FILE_NAME"))...)
	entityIDClaims = append(entityIDClaims, document.Get(search.GetCorePropertyID("ENGLISH_WIKIPEDIA_FILE_NAME"))...)

	entityIDs := []string{}
	for _, entityIDClaim := range entityIDClaims {
		idClaim, ok := entityIDClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected ID claim type")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = entityIDClaim.GetID().String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", entityIDClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			return false, errE
		}
		entityIDs = append(entityIDs, idClaim.Identifier)
	}

	cache.Add(document.ID, document)

	v := updateEmbeddedDocumentsVisitor{
		Context:                      ctx,
		Log:                          log,
		Index:                        index,
		Cache:                        cache,
		SkippedWikidataEntities:      skippedWikidataEntities,
		SkippedWikimediaCommonsFiles: skippedWikimediaCommonsFiles,
		ESClient:                     esClient,
		Changed:                      0,
		DocumentID:                   document.ID,
		EntityIDs:                    entityIDs,
	}
	errE := document.Visit(&v)
	if errE != nil {
		return false, errE
	}

	return v.Changed > 0, nil
}
