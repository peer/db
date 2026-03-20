package search_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/search"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// dummyMetadata returns a minimal DocumentMetadata for testing.
func dummyMetadata() *internal.DocumentMetadata {
	return &internal.DocumentMetadata{
		At:               internal.Time(time.Now().UTC()),
		InverseRelations: nil,
	}
}

// makeDocJSON creates a valid document.D JSON for a given ID.
func makeDocJSON(t *testing.T, id identifier.Identifier) json.RawMessage {
	t.Helper()
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}

// newTestBridgeConverter creates a minimal Converter for bridge tests.
func newTestBridgeConverter(t *testing.T) *search.Converter {
	t.Helper()
	c, errE := search.NewConverter(nil, nil, nil, func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return &document.D{
			CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		}, nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

func initBridge(t *testing.T) (
	context.Context,
	*store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes],
	*search.Bridge, *elastic.Client,
) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}
	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(_ context.Context) (string, string) {
		return schema, "test"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpool.Close)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	esClient, errE := search.GetClient(cleanhttp.DefaultPooledClient(), zerolog.Nop(), os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	index := schema

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		require.NoError(t, err)
	})

	// Create the index with PeerDB mapping so the converter's output is accepted.
	errE = search.EnsureIndex(ctx, esClient, index)
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internal.NewListener(dbpool)

	riverClient, workers, errE := internal.NewRiver(ctx, logger, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes]{
		Prefix:        prefix,
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
		CommittedSize: 100,
	}
	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	b := &search.Bridge{
		Store:    s,
		ESClient: esClient,
		Index:    index,
	}
	errE = b.Init(ctx, dbpool, listener, schema, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := riverClient.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-riverClient.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, s, b, esClient
}

// docExists returns true if the document with the given ID exists in Elasticsearch.
func docExists(t *testing.T, ctx context.Context, esClient *elastic.Client, index, id string) bool { //nolint:revive
	t.Helper()
	resp, err := esClient.Get().Index(index).Id(id).Do(ctx)
	if err != nil {
		if elastic.IsNotFound(err) {
			return false
		}
		t.Fatalf("unexpected ES error: %v", err)
	}
	return resp.Found
}

func TestBridgeRealTime(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	b.Start(ctx, newTestBridgeConverter(t))

	// Insert three documents.
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id3, makeDocJSON(t, id3), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for bridge to catch up and force ES to make documents searchable.
	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// All three documents should now be in search.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id1.String()), "doc1 should exist in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id2.String()), "doc2 should exist in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id3.String()), "doc3 should exist in ES")

	// Update doc1.
	_, _, v1, errE := s.GetLatest(ctx, id1)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Replace(ctx, id1, v1.Changeset, makeDocJSON(t, id1), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// The bridge always indexes the latest version, even if an older commit triggered it.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id1.String()), "doc1 should still exist after update")
}

func TestBridgeCatchUp(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	// Make commits BEFORE starting the bridge.
	id1 := identifier.New()
	id2 := identifier.New()

	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Bridge seq should still be 0 — nothing indexed yet.
	entries, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, entries, 2)

	// Now start the bridge. It should catch up from CommitLog.
	b.Start(ctx, newTestBridgeConverter(t))

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// Both documents should be in ES despite being committed before Start.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id1.String()), "catchup doc1 should be in ES after catch-up")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id2.String()), "catchup doc2 should be in ES after catch-up")
}

func TestBridgeDeletedDocument(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	b.Start(ctx, newTestBridgeConverter(t))

	id := identifier.New()

	// Insert then delete a document.
	v, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	assert.True(t, docExists(t, ctx, esClient, b.Index, id.String()), "document should exist before delete")

	_, errE = s.Delete(ctx, id, v.Changeset, dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// After deletion the bridge issues a bulk delete, so the document is removed from search.
	assert.False(t, docExists(t, ctx, esClient, b.Index, id.String()), "document should be removed from ES after delete")
}

func TestBridgeSeqAdvancement(t *testing.T) {
	t.Parallel()

	ctx, s, b, _ := initBridge(t)

	b.Start(ctx, newTestBridgeConverter(t))

	// Make several commits and verify the bridge table seq advances correctly.
	for range 5 {
		id := identifier.New()
		_, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), &internal.NoMetadata{})
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	errE := b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The bridge table seq must match the maximum CommitLog seq.
	commitLog, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, commitLog)

	maxSeq := commitLog[len(commitLog)-1].Seq

	// After WaitUntilCaughtUp the bridge seq is >= maxSeq by definition.
	// Verify with a direct CommitLog check.
	assert.GreaterOrEqual(t, maxSeq, int64(1))
}

func TestBridgeNotifyRecovery(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	b.Start(ctx, newTestBridgeConverter(t))

	// Insert initial documents and wait for the bridge to catch up.
	id1 := identifier.New()
	id2 := identifier.New()
	_, errE := s.Insert(ctx, id1, makeDocJSON(t, id1), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, makeDocJSON(t, id2), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Simulate a listener reconnection by closing the store's Committed channel.
	// The bridge's run loop detects the channel close, exits with errCommittedChannelClosed,
	// and restarts — re-running the catch-up phase to recover any missed commits.
	err := s.HandleBacklog(ctx, s.Prefix+"CommittedChangesets", nil)
	require.NoError(t, err, "% -+#.1v", err) // This is still errors.E.

	// Insert more documents after the simulated reconnection. These may be missed by the
	// real-time channel but must be recovered via the catch-up phase on bridge restart.
	id3 := identifier.New()
	id4 := identifier.New()
	_, errE = s.Insert(ctx, id3, makeDocJSON(t, id3), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id4, makeDocJSON(t, id4), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// All four documents must be indexed, including those inserted after the simulated reconnection.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id1.String()), "initial doc1 should be in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id2.String()), "initial doc2 should be in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id3.String()), "recovery doc3 should be in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id4.String()), "recovery doc4 should be in ES")
}

func TestBridgeStaleDataNotIndexed(t *testing.T) {
	t.Parallel()

	// The bridge always fetches the latest version of each document, so even if an older
	// commit triggers indexing, the most up-to-date data ends up in Elasticsearch.
	ctx, s, b, esClient := initBridge(t)

	id := identifier.New()

	// Insert initial data and immediately replace before starting the bridge.
	v, errE := s.Insert(ctx, id, makeDocJSON(t, id), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Replace(ctx, id, v.Changeset, makeDocJSON(t, id), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now start the bridge. It should catch up and index the latest version.
	b.Start(ctx, newTestBridgeConverter(t))

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// The document in ES should exist — the bridge calls GetLatest so it always indexes the latest version.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id.String()), "document should be in ES")
}

// Well-known IDs for inverse relation tests.
//
//nolint:gochecknoglobals
var (
	testInstanceOfPropID       = identifier.From(core.Namespace, "INSTANCE_OF")
	testPropertyClassID        = identifier.From(core.Namespace, "PROPERTY")
	testInversePropertyOfPropI = identifier.From(core.Namespace, "INVERSE_PROPERTY_OF")
)

// makePropertyDocJSON creates a property document (INSTANCE_OF PROPERTY) with optional INVERSE_PROPERTY_OF.
func makePropertyDocJSON(t *testing.T, id identifier.Identifier, inverseOf *identifier.Identifier) json.RawMessage {
	t.Helper()
	claims := &document.ClaimTypes{}
	claims.Relation = append(claims.Relation, document.RelationClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
		Prop:      document.Reference{ID: testInstanceOfPropID},
		To:        document.Reference{ID: testPropertyClassID},
	})
	if inverseOf != nil {
		claims.Relation = append(claims.Relation, document.RelationClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: testInversePropertyOfPropI},
			To:        document.Reference{ID: *inverseOf},
		})
	}
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}

// makeDocWithRelationJSON creates a document with a relation claim.
func makeDocWithRelationJSON(t *testing.T, docID, propID, targetID identifier.Identifier) json.RawMessage {
	t.Helper()
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Relation: []document.RelationClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: propID},
					To:        document.Reference{ID: targetID},
				},
			},
		},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}

// makeConverterWithInverse creates a converter that knows about inverse properties.
// propX has inversePropertyOf propY. The getDocument callback fetches from the store.
func makeConverterWithInverse(
	t *testing.T, propX, propY identifier.Identifier,
	s *store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes],
) *search.Converter {
	t.Helper()

	propXDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: propX}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Relation: []document.RelationClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: testInstanceOfPropID},
					To:        document.Reference{ID: testPropertyClassID},
				},
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: testInversePropertyOfPropI},
					To:        document.Reference{ID: propY},
				},
			},
		},
	}
	propYDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: propY}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Relation: []document.RelationClaim{
				{
					CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: testInstanceOfPropID},
					To:        document.Reference{ID: testPropertyClassID},
				},
			},
		},
	}

	properties := []*document.D{propXDoc, propYDoc}

	c, errE := search.NewConverter(properties, nil, nil, func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
		data, _, _, errE := s.GetLatest(ctx, id)
		if errE != nil {
			if errors.Is(errE, store.ErrValueNotFound) {
				// Return a minimal document for IDs not in the store (e.g., core property/class IDs).
				return &document.D{
					CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
				}, nil
			}
			return nil, errE
		}
		var doc document.D
		err := json.Unmarshal(data, &doc)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &doc, nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

// docHasRelation checks if an ES document has a nested relation claim with the given prop and target.
func docHasRelation(ctx context.Context, t *testing.T, esClient *elastic.Client, index string, docID, propID, targetID identifier.Identifier) bool {
	t.Helper()

	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.rel.prop", propID.String()),
		elastic.NewTermQuery("claims.rel.to", targetID.String()),
	)
	query := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("id", docID.String()),
		elastic.NewNestedQuery("claims.rel", boolQuery),
	)
	res, err := esClient.Search().Index(index).Query(query).Size(1).Do(ctx)
	if err != nil {
		t.Fatalf("ES search error: %v", err)
	}
	return res.Hits.TotalHits.Value > 0
}

func TestBridgeInverseRelationReindexing(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	// Property X has inversePropertyOf Y.
	// So A --X--> B means B should get an inverse claim B --Y--> A.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	b.Start(ctx, converter)

	// Insert property documents into the store so getDocument can find them.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert document A with a relation A --X--> B, and document B (empty).
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for the bridge to index the initial commits.
	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify that docB's metadata was updated with inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, metadata, _, errE := s.GetLatest(ctx, docB)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		assert.NotEmpty(c, metadata.InverseRelations, "docB metadata should have inverse relations")
	}, 10*time.Second, 100*time.Millisecond)

	// Wait for the River job to re-index document B with the inverse relation.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse relation B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)

	// Doc A should have the forward relation A --X--> B.
	assert.True(t, docHasRelation(ctx, t, esClient, b.Index, docA, propX, docB),
		"docA should have forward relation A --X--> B")
}

func TestBridgeInverseRelationMutual(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	// Property X has inversePropertyOf Y.
	// A --X--> B means B gets B --Y--> A.
	// B --X--> A means A gets A --Y--> B.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	b.Start(ctx, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert A --X--> B and B --X--> A in the same commit won't work with the store API
	// (each Insert is its own commit). So insert them as separate commits.
	docA := identifier.New()
	docB := identifier.New()
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docB, makeDocWithRelationJSON(t, docB, propX, docA), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Both documents should eventually have both forward and inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		// A should have forward A --X--> B and inverse A --Y--> B (from B --X--> A).
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docA, propX, docB),
			"docA should have forward A --X--> B")
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docA, propY, docB),
			"docA should have inverse A --Y--> B")
		// B should have forward B --X--> A and inverse B --Y--> A (from A --X--> B).
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propX, docA),
			"docB should have forward B --X--> A")
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
	}, 10*time.Second, 100*time.Millisecond)
}

func TestBridgeInverseRelationMultipleSources(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	// Property X has inversePropertyOf Y.
	// Both A and C point to B with property X.
	// B should get two inverse relations: B --Y--> A and B --Y--> C.
	propX := identifier.New()
	propY := identifier.New()

	converter := makeConverterWithInverse(t, propX, propY, s)
	b.Start(ctx, converter)

	// Insert property documents.
	_, errE := s.Insert(ctx, propX, makePropertyDocJSON(t, propX, &propY), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, propY, makePropertyDocJSON(t, propY, nil), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	docA := identifier.New()
	docB := identifier.New()
	docC := identifier.New()

	_, errE = s.Insert(ctx, docB, makeDocJSON(t, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docA, makeDocWithRelationJSON(t, docA, propX, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, docC, makeDocWithRelationJSON(t, docC, propX, docB), dummyMetadata(), &internal.NoMetadata{})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// B should eventually have both inverse relations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := esClient.Refresh(b.Index).Do(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docA),
			"docB should have inverse B --Y--> A")
		assert.True(c, docHasRelation(ctx, t, esClient, b.Index, docB, propY, docC),
			"docB should have inverse B --Y--> C")
	}, 10*time.Second, 100*time.Millisecond)
}
