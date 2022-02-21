package wikipedia

import (
	"context"
	"fmt"
	"reflect"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

var referenceNotFoundError = errors.Base("document reference to a nonexistent document")

type updateEmbeddedDocumentsVisitor struct {
	Context     context.Context
	Log         zerolog.Logger
	Cache       *Cache
	ESClient    *elastic.Client
	Changed     bool
	DocumentID  search.Identifier
	WikidataIDs []string
}

func (v *updateEmbeddedDocumentsVisitor) referenceNotFound(ref search.DocumentReference, claimID search.Identifier) errors.E {
	name := ref.Name["en"]
	if name == "" {
		name = ref.Name["XX"]
	}
	errE := errors.WithStack(referenceNotFoundError)
	details := errors.Details(errE)
	details["doc"] = string(v.DocumentID)
	if len(v.WikidataIDs) == 1 {
		details["entity"] = v.WikidataIDs[0]
	} else if len(v.WikidataIDs) > 1 {
		details["entity"] = v.WikidataIDs
	}
	details["claim"] = string(claimID)
	details["ref"] = string(ref.ID)
	if name != "" {
		details["name"] = name
	}
	return errE
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReference(ref search.DocumentReference, claimID search.Identifier) (*search.DocumentReference, errors.E) {
	id := ref.ID
	maybeRef, ok := v.Cache.Get(id)
	if ok {
		if maybeRef == nil {
			return nil, v.referenceNotFound(ref, claimID)
		}
		return maybeRef.(*search.DocumentReference), nil
	}

	esDoc, err := v.ESClient.Get().Index("docs").Id(string(id)).Do(v.Context)
	if elastic.IsNotFound(err) {
		v.Cache.Add(id, nil)
		return nil, v.referenceNotFound(ref, claimID)
	} else if err != nil {
		return nil, errors.WithStack(err)
	} else if !esDoc.Found {
		v.Cache.Add(id, nil)
		return nil, v.referenceNotFound(ref, claimID)
	}

	var document search.Document
	err = x.UnmarshalWithoutUnknownFields(esDoc.Source, &document)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	res := &search.DocumentReference{
		ID:     id,
		Name:   document.Name,
		Score:  document.Score,
		Scores: document.Scores,
	}
	v.Cache.Add(id, res)
	return res, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitIdentifier(claim *search.IdentifierClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitLabel(claim *search.LabelClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitEnumeration(claim *search.EnumerationClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	ref, err = v.getDocumentReference(claim.To, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.To, ref) {
		claim.To = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitIs(claim *search.IsClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.To, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.To, ref) {
		claim.To = *ref
		v.Changed = true
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
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitList(claim *search.ListClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	ref, err = v.getDocumentReference(claim.Element, claim.ID)
	if err != nil {
		if errors.Is(err, referenceNotFoundError) {
			v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
			v.Changed = true
			return search.Drop, nil
		}
		return search.Keep, err
	}

	if !reflect.DeepEqual(&claim.Element, ref) {
		claim.Element = *ref
		v.Changed = true
	}

	for i := range claim.Children {
		child := &claim.Children[i]
		ref, err := v.getDocumentReference(child.Prop, claim.ID)
		if err != nil {
			if errors.Is(err, referenceNotFoundError) {
				v.Log.Warn().Fields(errors.AllDetails(err)).Msg(err.Error())
				v.Changed = true
				// We might want just to remove a child, but because this codepath should not really by Wikidata
				// data (we do not convert any Wikidata statements to PeerDB list claims, and this is about
				// hierarchical lists) it is probably a reasonable simplification.
				return search.Drop, nil
			}
			return search.Keep, err
		}

		if !reflect.DeepEqual(&child.Prop, ref) {
			child.Prop = *ref
			v.Changed = true
		}
	}

	return search.Keep, nil
}

func UpdateEmbeddedDocuments(ctx context.Context, log zerolog.Logger, esClient *elastic.Client, cache *Cache, document *search.Document) (bool, errors.E) {
	wikidataIDClaims := []search.Claim{}
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_ITEM_ID"))...)
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_PROPERTY_ID"))...)

	wikidataIDs := []string{}
	for _, wikidataIDClaim := range wikidataIDClaims {
		idClaim, ok := wikidataIDClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected Wikidata ID claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(wikidataIDClaim.GetID())
			errors.Details(errE)["got"] = fmt.Sprintf("%T", wikidataIDClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			return false, errE
		}
		wikidataIDs = append(wikidataIDs, idClaim.Identifier)
	}

	ref := &search.DocumentReference{
		ID:     document.ID,
		Name:   document.Name,
		Score:  document.Score,
		Scores: document.Scores,
	}
	cache.Add(document.ID, ref)

	v := updateEmbeddedDocumentsVisitor{
		Context:     ctx,
		Log:         log,
		Cache:       cache,
		ESClient:    esClient,
		Changed:     false,
		DocumentID:  document.ID,
		WikidataIDs: wikidataIDs,
	}
	errE := document.Visit(&v)
	if errE != nil {
		return false, errE
	}

	return v.Changed, nil
}
