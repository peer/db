package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/search"
)

const (
	progressPrintRate = 30 * time.Second
	lruCacheSize      = 1000000
)

type counter int64

func (c *counter) Increment() {
	atomic.AddInt64((*int64)(c), 1)
}

func (c *counter) Count() int64 {
	return atomic.LoadInt64((*int64)(c))
}

type Cache struct {
	*lru.Cache
	missCount uint64
}

func NewCache(size int) (*Cache, error) {
	cache, err := lru.New(lruCacheSize)
	if err != nil {
		return nil, err
	}
	return &Cache{
		Cache:     cache,
		missCount: 0,
	}, nil
}

func (c *Cache) Get(key interface{}) (interface{}, bool) {
	value, ok := c.Cache.Get(key)
	if !ok {
		atomic.AddUint64(&c.missCount, 1)
	}
	return value, ok
}

func (c *Cache) MissCount() uint64 {
	return atomic.SwapUint64(&c.missCount, 0)
}

func updateEmbeddedDocuments(ctx context.Context, config *Config, esClient *elastic.Client, processor *elastic.BulkProcessor) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	var c counter

	total, err := esClient.Count("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	cache, err := NewCache(lruCacheSize)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	ticker := x.NewTicker(ctx, &c, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s, cache miss: %d, docs: %d, indexed: %d, failed: %d\n", p.Percent(), p.Remaining().Truncate(time.Second), cache.MissCount(), c.Count(), stats.Succeeded, stats.Failed)
		}
	}()

	hits := make(chan *elastic.SearchHit, documentProcessingThreads)
	g.Go(func() error {
		defer close(hits)

		scroll := esClient.Scroll("docs").Size(documentProcessingThreads * 10).SearchSource(elastic.NewSearchSource().SeqNoAndPrimaryTerm(true))
		for {
			results, err := scroll.Do(ctx)
			if errors.Is(err, io.EOF) {
				return nil
			} else if err != nil {
				return errors.WithStack(err)
			}

			for _, hit := range results.Hits.Hits {
				select {
				case hits <- hit:
				case <-ctx.Done():
					return errors.WithStack(ctx.Err())
				}
			}
		}
	})

	for i := 0; i < documentProcessingThreads; i++ {
		g.Go(func() error {
			for {
				select {
				case hit, ok := <-hits:
					if !ok {
						return nil
					}
					err := processDocument(ctx, esClient, processor, cache, hit)
					if err != nil {
						return err
					}
					c.Increment()
				case <-ctx.Done():
					return errors.WithStack(ctx.Err())
				}
			}
		})
	}

	return errors.WithStack(g.Wait())
}

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
	fmt.Fprintf(os.Stderr, "document %s (%s) has a claim %s with a document reference to %s (%s), but document does not exist\n", v.DocumentID, v.WikidataID, claimID, ref.ID, name)
}

func (v *updateEmbeddedDocumentsVisitor) getDocumentReference(ref search.DocumentReference, claimID search.Identifier) (*search.DocumentReference, errors.E) {
	id := ref.ID
	maybeRef, ok := v.Cache.Get(id)
	if ok {
		if maybeRef == nil {
			v.warnDocumentReference(ref, claimID)
			return nil, nil
		}
		return maybeRef.(*search.DocumentReference), nil
	}

	esDoc, err := v.ESClient.Get().Index("docs").Id(string(id)).Do(v.Context)
	if elastic.IsNotFound(err) {
		v.Cache.Add(id, nil)
		v.warnDocumentReference(ref, claimID)
		return nil, nil
	} else if err != nil {
		return nil, errors.WithStack(err)
	} else if !esDoc.Found {
		v.Cache.Add(id, nil)
		v.warnDocumentReference(ref, claimID)
		return nil, nil
	}

	var document search.Document
	err = json.Unmarshal(esDoc.Source, &document)
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	ref, err = v.getDocumentReference(claim.To, claim.ID)
	if err != nil {
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitDuration(claim *search.DurationClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	return search.Keep, nil
}

func (v *updateEmbeddedDocumentsVisitor) VisitDurationRange(claim *search.DurationRangeClaim) (search.VisitResult, errors.E) {
	err := claim.VisitMeta(v)
	if err != nil {
		return search.Keep, err
	}

	ref, err := v.getDocumentReference(claim.Prop, claim.ID)
	if err != nil {
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
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
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Prop, ref) {
		claim.Prop = *ref
		v.Changed = true
	}

	ref, err = v.getDocumentReference(claim.Element, claim.ID)
	if err != nil {
		return search.Keep, err
	}

	if ref == nil {
		v.Changed = true
		return search.Drop, nil
	}

	if !reflect.DeepEqual(&claim.Element, ref) {
		claim.Element = *ref
		v.Changed = true
	}

	for i := range claim.Children {
		child := &claim.Children[i]
		ref, err := v.getDocumentReference(child.Prop, claim.ID)
		if err != nil {
			return search.Keep, err
		}

		if ref == nil {
			v.Changed = true
			return search.Drop, nil
		}

		if !reflect.DeepEqual(&child.Prop, ref) {
			child.Prop = *ref
			v.Changed = true
		}
	}

	return search.Keep, nil
}

func processDocument(ctx context.Context, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *Cache, hit *elastic.SearchHit) errors.E {
	var document search.Document
	err := json.Unmarshal(hit.Source, &document)
	if err != nil {
		return errors.WithStack(err)
	}

	// ID is not stored in the document, so we set it here ourselves.
	document.ID = search.Identifier(hit.Id)

	wikidataIDClaims := []search.Claim{}
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_ITEM_ID"))...)
	wikidataIDClaims = append(wikidataIDClaims, document.Get(search.GetStandardPropertyID("WIKIDATA_PROPERTY_ID"))...)

	wikidataIDs := []string{}
	for _, wikidataIDClaim := range wikidataIDClaims {
		idClaim, ok := wikidataIDClaim.(*search.IdentifierClaim)
		if !ok {
			return errors.Errorf("Wikidata ID claim %s which is not an ID claim, but %T", wikidataIDClaim.GetID(), wikidataIDClaim)
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
		return errE
	}

	if v.Changed {
		req := elastic.NewBulkIndexRequest().Index("docs").Id(hit.Id).IfSeqNo(*hit.SeqNo).IfPrimaryTerm(*hit.PrimaryTerm).Doc(&document)
		processor.Add(req)
	}

	return nil
}
