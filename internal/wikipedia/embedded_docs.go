package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

var (
	errReferenceNotFound  = errors.Base("document reference to a nonexistent document")
	errReferenceTemporary = errors.Base("document reference is a temporary reference")
)

type updateEmbeddedDocumentsVisitor struct {
	Context                      context.Context //nolint:containedctx
	Log                          zerolog.Logger
	Index                        string
	Store                        *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]
	Cache                        *es.Cache
	SkippedWikidataEntities      *sync.Map
	SkippedWikimediaCommonsFiles *sync.Map
	ESClient                     *elastic.Client
	Changed                      int
	DocumentID                   identifier.Identifier
	EntityIDs                    []string
}

func (v *updateEmbeddedDocumentsVisitor) makeError(err error, ref document.Reference, claimID identifier.Identifier) errors.E {
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

func (v *updateEmbeddedDocumentsVisitor) logWarning(fileDoc *document.D, claimID identifier.Identifier, msg string) {
	l := v.Log.Warn().Str("doc", v.DocumentID.String()) //nolint:zerologlint
	if len(v.EntityIDs) == 1 {
		l = l.Str("entity", v.EntityIDs[0])
	} else if len(v.EntityIDs) > 1 {
		l = l.Strs("entity", v.EntityIDs)
	}
	l = l.Str("claim", claimID.String())
	l = l.Str("ref", fileDoc.ID.String())
	l.Msg(msg)
}

func (v *updateEmbeddedDocumentsVisitor) handleError(errE errors.E, ref document.Reference) (document.VisitResult, errors.E) {
	if errors.Is(errE, errReferenceNotFound) { //nolint:nestif
		if ref.ID != nil {
			if _, ok := v.SkippedWikidataEntities.Load(ref.ID.String()); ok {
				v.Log.Debug().Err(errE).Send()
			} else {
				v.Log.Warn().Err(errE).Send()
			}
		} else {
			prop, id := v.getOriginalID(ref.Temporary)
			if prop == "WIKIMEDIA_COMMONS_FILE_NAME" {
				if _, ok := v.SkippedWikimediaCommonsFiles.Load(id); ok {
					v.Log.Debug().Err(errE).Send()
				} else {
					v.Log.Warn().Err(errE).Send()
				}
			} else {
				name, ok := errors.AllDetails(errE)["name"].(string)
				if ok && (strings.HasPrefix(name, "Template:") || strings.HasPrefix(name, "Module:") || strings.HasPrefix(name, "Category:")) {
					v.Log.Debug().Err(errE).Send()
				} else {
					v.Log.Warn().Err(errE).Send()
				}
			}
		}
		v.Changed++
		return document.Drop, nil
	}
	return document.Keep, errE
}

func (v *updateEmbeddedDocumentsVisitor) getOriginalID(temporary []string) (string, string) {
	if len(temporary) != 2 { //nolint:gomnd
		return "", ""
	}

	switch temporary[0] {
	case WikipediaCategoryReference:
		return "ENGLISH_WIKIPEDIA_PAGE_TITLE", temporary[1]
	case WikipediaTemplateReference:
		return "ENGLISH_WIKIPEDIA_PAGE_TITLE", temporary[1]
	case WikimediaCommonsCategoryReference:
		return "WIKIMEDIA_COMMONS_PAGE_TITLE", temporary[1]
	case WikimediaCommonsTemplateReference:
		return "WIKIMEDIA_COMMONS_PAGE_TITLE", temporary[1]
	case WikimediaCommonsFileReference:
		filename := strings.TrimPrefix(temporary[1], "File:")
		filename = strings.ReplaceAll(filename, " ", "_")
		filename = FirstUpperCase(filename)
		return "WIKIMEDIA_COMMONS_FILE_NAME", filename
	case WikimediaCommonsEntityReference:
		return "WIKIMEDIA_COMMONS_ENTITY_ID", temporary[1]
	}

	return "", ""
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReference(ref document.Reference, claimID identifier.Identifier) (*document.Reference, errors.E) {
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

func (v *updateEmbeddedDocumentsVisitor) getDocumentByProp(property, title string) (*document.D, errors.E) {
	// Here we check cache with string type, so values cannot conflict with caching done
	// by getDocumentByID, which uses Identifier type.
	maybeDocument, ok := v.Cache.Get(title)
	if ok {
		if maybeDocument == nil {
			return nil, errors.WithStack(ErrNotFound)
		}
		return maybeDocument.(*document.D), nil //nolint:forcetypeassert
	}

	document, _, err := getDocumentFromByProp(v.Context, v.Store, v.Index, v.ESClient, property, title)
	if errors.Is(err, ErrNotFound) {
		v.Cache.Add(title, nil)
		return nil, err
	} else if err != nil {
		return nil, err
	}

	v.Cache.Add(title, document)
	v.Cache.Add(document.ID, document)

	return document, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentByID(id identifier.Identifier) (*document.D, errors.E) {
	// Here we check cache with Identifier type, so values cannot conflict with caching done
	// by getDocumentByTitle, which uses string type.
	maybeDocument, ok := v.Cache.Get(id)
	if ok {
		if maybeDocument == nil {
			return nil, errors.WithStack(ErrNotFound)
		}
		return maybeDocument.(*document.D), nil //nolint:forcetypeassert
	}

	document, _, err := getDocumentFromByID(v.Context, v.Store, id)
	if errors.Is(err, ErrNotFound) {
		v.Cache.Add(id, nil)
		return nil, err
	} else if err != nil {
		return nil, err
	}

	v.Cache.Add(document.ID, document)

	return document, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReferenceByProp(
	ref document.Reference, claimID identifier.Identifier, property, title string,
) (*document.Reference, errors.E) {
	document, err := v.getDocumentByProp(property, title)
	if errors.Is(err, ErrNotFound) {
		return nil, v.makeError(errReferenceNotFound, ref, claimID)
	} else if err != nil {
		return nil, v.makeError(err, ref, claimID)
	}

	res := document.Reference()

	return &res, nil
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReferenceByID(ref document.Reference, claimID identifier.Identifier) (*document.Reference, errors.E) {
	if ref.ID == nil {
		return nil, v.makeError(errReferenceTemporary, ref, claimID)
	}

	document, err := v.getDocumentByID(*ref.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, v.makeError(errReferenceNotFound, ref, claimID)
	} else if err != nil {
		return nil, v.makeError(err, ref, claimID)
	}

	res := document.Reference()

	return &res, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitText(claim *document.TextClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitAmountRange(claim *document.AmountRangeClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitRelation(claim *document.RelationClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	ref, errE = v.getDocumentReference(claim.To, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.To)
	}

	if !reflect.DeepEqual(&claim.To, ref) {
		claim.To = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitNoValue(claim *document.NoValueClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitUnknownValue(claim *document.UnknownValueClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitTimeRange(claim *document.TimeRangeClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	return document.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitFile(claim *document.FileClaim) (document.VisitResult, errors.E) {
	errE := claim.Visit(v)
	if errE != nil {
		return document.Keep, errE
	}

	ref, errE := v.getDocumentReference(claim.Prop, claim.ID)
	if errE != nil {
		return v.handleError(errE, claim.Prop)
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed++
	}

	var fileDocument *document.D
	for _, cc := range claim.Get(document.GetCorePropertyID("IS")) {
		if c, ok := cc.(*document.RelationClaim); ok {
			// c.To.ID should be non-nil ID because we called claim.Visit(v) above.
			fileDocument, errE = v.getDocumentByID(*c.To.ID)
			if errors.Is(errE, ErrNotFound) {
				return v.handleError(v.makeError(errReferenceNotFound, c.To, c.ID), c.To)
			} else if errE != nil {
				return v.handleError(v.makeError(errE, c.To, c.ID), c.To)
			}

			break
		}
	}

	if fileDocument != nil {
		var mediaType string
		for _, cc := range fileDocument.Get(document.GetCorePropertyID("MEDIA_TYPE")) {
			if c, ok := cc.(*document.StringClaim); ok {
				mediaType = c.String
				break
			}
		}
		if mediaType == "" {
			v.logWarning(fileDocument, claim.ID, "referenced Wikimedia commons file document is missing a MEDIA_TYPE string claim")
			v.Changed++
			return document.Drop, nil
		}

		if !reflect.DeepEqual(claim.MediaType, mediaType) {
			claim.MediaType = mediaType
			v.Changed++
		}

		var fileURL string
		for _, cc := range fileDocument.Get(document.GetCorePropertyID("FILE_URL")) {
			if c, ok := cc.(*document.ReferenceClaim); ok {
				fileURL = c.IRI
				break
			}
		}
		if fileURL == "" {
			v.logWarning(fileDocument, claim.ID, "referenced Wikimedia commons file document is missing a FILE_URL reference claim")
			v.Changed++
			return document.Drop, nil
		}

		if !reflect.DeepEqual(claim.URL, fileURL) {
			claim.URL = fileURL
			v.Changed++
		}

		// TODO: First extract individual lists, then sort each least by order, and then concatenate lists.
		var previews []string
		for _, cc := range fileDocument.Get(document.GetCorePropertyID("PREVIEW_URL")) {
			if c, ok := cc.(*document.ReferenceClaim); ok {
				previews = append(previews, c.IRI)
			}
		}

		if !reflect.DeepEqual(claim.Preview, previews) {
			claim.Preview = previews
			v.Changed++
		}
	}

	return document.Keep, nil
}

func UpdateEmbeddedDocuments(
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	index string, esClient *elastic.Client, cache *es.Cache, skippedWikidataEntities *sync.Map, skippedWikimediaCommonsFiles *sync.Map, doc *document.D,
) (bool, errors.E) {
	// We try to obtain unhashed document IDs to use in logging.
	entityIDClaims := []document.Claim{}
	entityIDClaims = append(entityIDClaims, doc.Get(document.GetCorePropertyID("WIKIDATA_ITEM_ID"))...)
	entityIDClaims = append(entityIDClaims, doc.Get(document.GetCorePropertyID("WIKIDATA_PROPERTY_ID"))...)
	entityIDClaims = append(entityIDClaims, doc.Get(document.GetCorePropertyID("WIKIMEDIA_COMMONS_FILE_NAME"))...)
	entityIDClaims = append(entityIDClaims, doc.Get(document.GetCorePropertyID("ENGLISH_WIKIPEDIA_FILE_NAME"))...)

	entityIDs := []string{}
	for _, entityIDClaim := range entityIDClaims {
		idClaim, ok := entityIDClaim.(*document.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected ID claim type")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = entityIDClaim.GetID().String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", entityIDClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", new(document.IdentifierClaim))
			return false, errE
		}
		entityIDs = append(entityIDs, idClaim.Value)
	}

	cache.Add(doc.ID, doc)

	v := updateEmbeddedDocumentsVisitor{
		Context:                      ctx,
		Log:                          logger,
		Index:                        index,
		Store:                        store,
		Cache:                        cache,
		SkippedWikidataEntities:      skippedWikidataEntities,
		SkippedWikimediaCommonsFiles: skippedWikimediaCommonsFiles,
		ESClient:                     esClient,
		Changed:                      0,
		DocumentID:                   doc.ID,
		EntityIDs:                    entityIDs,
	}
	errE := doc.Visit(&v)
	if errE != nil {
		return false, errE
	}

	return v.Changed > 0, nil
}
