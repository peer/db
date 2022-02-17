package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	progressPrintRate   = 30 * time.Second
	lruCacheSize        = 1000000
	scrollingMultiplier = 10
)

var notFoundDocumentError = errors.Base("not found document")

type PrepareCommand struct{}

func (c *PrepareCommand) Run(globals *Globals) errors.E {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	esClient, errE := search.EnsureIndex(ctx, cleanhttp.DefaultPooledClient())
	if errE != nil {
		return errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Indexing error: %s\n", err.Error())
			} else if response.Errors {
				for _, failed := range response.Failed() {
					fmt.Fprintf(os.Stderr, "Indexing error %d (%s): %s [type=%s]\n", failed.Status, http.StatusText(failed.Status), failed.Error.Reason, failed.Error.Type)
				}
				fmt.Fprintf(os.Stderr, "Indexing error\n")
			}
		},
	).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer processor.Close()

	errE = c.saveStandardProperties(ctx, globals, esClient)
	if errE != nil {
		return errE
	}

	return c.updateEmbeddedDocuments(ctx, globals, esClient, processor)
}

func (c *PrepareCommand) saveStandardProperties(ctx context.Context, globals *Globals, esClient *elastic.Client) errors.E {
	for id, property := range search.StandardProperties {
		// We do not use a bulk processor because we want these documents to be available immediately.
		// We can pass a reference here because it is a blocking call and call completes before the next loop.
		_, err := esClient.Index().Index("docs").Id(id).BodyJson(&property).Do(ctx) //nolint:gosec
		if err != nil {
			return errors.WithStack(err)
		}
	}
	// Make sure all added documents are available for search.
	_, err := esClient.Refresh("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *PrepareCommand) updateEmbeddedDocuments(ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor) errors.E {
	// TODO: Make configurable.
	documentProcessingThreads := runtime.GOMAXPROCS(0)

	var count x.Counter

	total, err := esClient.Count("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	cache, err := wikipedia.NewCache(lruCacheSize)
	if err != nil {
		return errors.WithStack(err)
	}

	g, ctx := errgroup.WithContext(ctx)

	ticker := x.NewTicker(ctx, &count, total, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			fmt.Fprintf(
				os.Stderr,
				"Progress: %0.2f%%, ETA: %s, cache miss: %d, docs: %d, indexed: %d, failed: %d\n",
				p.Percent(), p.Remaining().Truncate(time.Second), cache.MissCount(), count.Count(), stats.Succeeded, stats.Failed,
			)
		}
	}()

	hits := make(chan *elastic.SearchHit, documentProcessingThreads)
	g.Go(func() error {
		defer close(hits)

		scroll := esClient.Scroll("docs").Size(documentProcessingThreads * scrollingMultiplier).SearchSource(elastic.NewSearchSource().SeqNoAndPrimaryTerm(true))
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
					err := c.processDocument(ctx, esClient, processor, cache, hit)
					if err != nil {
						return err
					}
					count.Increment()
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
	Cache      *wikipedia.Cache
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

func (c *PrepareCommand) processDocument(
	ctx context.Context, esClient *elastic.Client, processor *elastic.BulkProcessor, cache *wikipedia.Cache, hit *elastic.SearchHit,
) errors.E {
	var document search.Document
	err := x.UnmarshalWithoutUnknownFields(hit.Source, &document)
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
