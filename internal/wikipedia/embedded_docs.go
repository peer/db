package wikipedia

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

var notFoundDocumentError = errors.Base("not found document")

type updateEmbeddedDocumentsVisitor struct {
	Context    context.Context
	Cache      *Cache
	ESClient   *elastic.Client
	Changed    bool
	DocumentID search.Identifier
	WikidataID string
}

func (v *updateEmbeddedDocumentsVisitor) warnDocumentReference(ref search.DocumentReference, claimID search.Identifier) {
	name := ref.Name["en"]
	if name == "" {
		name = ref.Name["XX"]
	}
	fmt.Fprintf(
		os.Stderr,
		"document %s (%s) has a claim %s with a document reference to %s (%s), but document does not exist\n",
		v.DocumentID, v.WikidataID, claimID, ref.ID, name,
	)
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReference(ref search.DocumentReference, claimID search.Identifier) (*search.DocumentReference, errors.E) {
	id := ref.ID
	maybeRef, ok := v.Cache.Get(id)
	if ok {
		if maybeRef == nil {
			v.warnDocumentReference(ref, claimID)
			return nil, errors.Errorf(`%w %s`, notFoundDocumentError, claimID)
		}
		return maybeRef.(*search.DocumentReference), nil
	}

	esDoc, err := v.ESClient.Get().Index("docs").Id(string(id)).Do(v.Context)
	if elastic.IsNotFound(err) {
		v.Cache.Add(id, nil)
		v.warnDocumentReference(ref, claimID)
		return nil, errors.Errorf(`%w %s`, notFoundDocumentError, claimID)
	} else if err != nil {
		return nil, errors.WithStack(err)
	} else if !esDoc.Found {
		v.Cache.Add(id, nil)
		v.warnDocumentReference(ref, claimID)
		return nil, errors.Errorf(`%w %s`, notFoundDocumentError, claimID)
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
		if errors.Is(err, notFoundDocumentError) {
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
			if errors.Is(err, notFoundDocumentError) {
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

func UpdateEmbeddedDocuments(ctx context.Context, esClient *elastic.Client, cache *Cache, document *search.Document) (bool, errors.E) {
	wikidataIDClaims := []search.Claim{}
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_ITEM_ID"))...)
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_PROPERTY_ID"))...)

	wikidataIDs := []string{}
	for _, wikidataIDClaim := range wikidataIDClaims {
		idClaim, ok := wikidataIDClaim.(*search.IdentifierClaim)
		if !ok {
			return false, errors.Errorf("document %s has a Wikidata ID claim %s which is not an ID claim, but %T", document.ID, wikidataIDClaim.GetID(), wikidataIDClaim)
		}
		wikidataIDs = append(wikidataIDs, idClaim.Identifier)
	}

	wikidataID := strings.Join(wikidataIDs, ",")

	ref := &search.DocumentReference{
		ID:     document.ID,
		Name:   document.Name,
		Score:  document.Score,
		Scores: document.Scores,
	}
	cache.Add(document.ID, ref)

	v := updateEmbeddedDocumentsVisitor{
		Context:    ctx,
		Cache:      cache,
		ESClient:   esClient,
		Changed:    false,
		DocumentID: document.ID,
		WikidataID: wikidataID,
	}
	errE := document.Visit(&v)
	if errE != nil {
		return false, errE
	}

	return v.Changed, nil
}
