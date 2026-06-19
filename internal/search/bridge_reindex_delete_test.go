package search_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

// makeInversePropertyDoc builds a property document declaring id as the inverse property of inverseOf.
func makeInversePropertyDoc(id, inverseOf identifier.Identifier) *document.D {
	claims := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
				To:        document.Reference{ID: internalCore.PropertyClassID},
			},
		},
	}
	if inverseOf != (identifier.Identifier{}) {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: internalCore.InversePropertyOfPropID},
			To:        document.Reference{ID: inverseOf},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// TestBridgeReindexDoesNotResurrectDeletedDocument guards the external-versioning protection against a reindex
// resurrecting a concurrently deleted document. B embeds from C and is enqueued for reindex (A references it).
// The reindex of B reads B live, then blocks in a post-hook while we delete B and wait for the delete to remove
// it from ES. When the reindex is released it writes its stale, pre-delete copy of B; ElasticSearch must reject
// that write because its external version (the reindex job's older snapshot seq) is below the delete's version,
// so B stays deleted.
func TestBridgeReindexDoesNotResurrectDeletedDocument(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	s, b, esClient := env.store, env.bridge, env.esClient

	classID := identifier.New()
	relProp := identifier.New()
	destProp := identifier.New()
	sourceProp := identifier.New()
	propInv := identifier.New()
	propInvTarget := identifier.New()
	docA := identifier.New()
	docB := identifier.New()
	docC := identifier.New()

	classDoc := makeEmbedClassDoc(classID, relProp, destProp, sourceProp)
	propInvDoc := makeInversePropertyDoc(propInv, propInvTarget)
	propInvTargetDoc := makeInversePropertyDoc(propInvTarget, identifier.Identifier{})

	// Gate that blocks the first getDocument(docC) call after arming (the reindex of B resolving C), so we can
	// delete B while the reindex holds a live, pre-delete copy of it.
	var mu sync.Mutex
	armed := false
	reached := make(chan struct{})
	release := make(chan struct{})

	getDocument := func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
		data, _, _, _, errE := s.GetLatest(ctx, id)
		if errors.Is(errE, store.ErrValueNotFound) { // ErrValueDeleted wraps ErrValueNotFound.
			return &document.D{
				CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
			}, nil
		} else if errE != nil {
			return nil, errE
		}
		var doc document.D
		errE = x.UnmarshalWithoutUnknownFields(data, &doc)
		if errE != nil {
			return nil, errE
		}
		return &doc, nil
	}

	// A post-hook runs on every produceLevels of a live document, including each reindex of B. When armed it
	// blocks the first time it sees B (the reindex holding a live, pre-delete copy), then disarms so the
	// concurrent delete's own reads of B's parent version pass through.
	b.DocumentPostHooks = []func(
		ctx context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
	) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E){
		func(
			_ context.Context, doc *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
		) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
			if errE != nil || doc == nil || doc.ID != docB {
				return doc, metadata, version, parentChangesets, errE
			}
			mu.Lock()
			block := armed
			if block {
				armed = false
			}
			mu.Unlock()
			if block {
				close(reached)
				<-release
			}
			return doc, metadata, version, parentChangesets, nil
		},
	}

	conv, errE := internalSearch.NewConverter([]*document.D{propInvDoc, propInvTargetDoc}, nil, []*document.D{classDoc}, nil, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)
	startBridge(ctx, t, env, conv)

	// The inverse-relation computation resolves the property's inverse via getDocument, so the property docs
	// must also be in the store.
	_, errE = s.Insert(ctx, propInv, makePropertyDocJSON(t, propInv, &propInvTarget), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propInvTarget, makePropertyDocJSON(t, propInvTarget, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert C, then B (which references and embeds C), then A (which references B).
	_, errE = s.Insert(ctx, docC, makeEmbedDocJSON(t, docC, classID, relProp, nil), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docB, makeEmbedDocJSON(t, docB, classID, relProp, &docC), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Indices.Refresh().Index(b.IndexPrefix).Do(ctx)
	testutils.RequireNoESError(t, err)
	require.True(t, testutils.DocExists(ctx, t, esClient, b.IndexPrefix, docB.String()), "B should exist before delete")

	// Arm the gate and insert A referencing B via the inverse property, which gives B an inverse relation and
	// enqueues B for reindex. Do not wait for catch-up: the reindex of B will block on the gate.
	mu.Lock()
	armed = true
	mu.Unlock()
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propInv, docB), dummyMetadata(), dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	select {
	case <-reached:
	case <-time.After(30 * time.Second):
		t.Fatal("reindex of B did not run its post-hook; B was not enqueued for reindex")
	}

	// The reindex of B now holds a live, pre-delete copy of B. Delete B; the main indexing loop removes it from
	// ES independently of the blocked reindex worker.
	_, metaB, vB, _, errE := s.GetLatest(ctx, docB)
	require.NoError(t, errE, "% -+#.1v", errE)
	newMetaB := dummyMetadata()
	newMetaB.CarryOver(metaB)
	_, errE = s.Delete(ctx, docB, vB.Changeset, newMetaB, dummyCommitMetadata())
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Indices.Refresh().Index(b.IndexPrefix).Do(ctx)
		testutils.RequireNoESError(t, err)
		assert.False(c, testutils.DocExists(ctx, t, esClient, b.IndexPrefix, docB.String()), "B should be removed from ES by the delete")
	}, 30*time.Second, 100*time.Millisecond)

	// Release the blocked reindex; it now writes its stale, pre-delete copy of B.
	close(release)

	errE = b.WaitUntilCaughtUp(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// B must stay deleted: the external versioning makes ElasticSearch reject the stale reindex write.
	_, err = esClient.Indices.Refresh().Index(b.IndexPrefix).Do(ctx)
	testutils.RequireNoESError(t, err)
	assert.False(t, testutils.DocExists(ctx, t, esClient, b.IndexPrefix, docB.String()), "B must stay deleted (reindex must not resurrect it)")
}
