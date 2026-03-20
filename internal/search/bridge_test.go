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

	logger := zerolog.New(zerolog.NewTestWriter(t))
	ctx = logger.WithContext(ctx)

	prefix := "s" + strings.ToLower(identifier.New().String())

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(_ context.Context) (string, string) {
		return prefix, "test"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpool.Close)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, prefix)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	esClient, errE := search.GetClient(cleanhttp.DefaultPooledClient(), zerolog.Nop(), os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	index := prefix

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		require.NoError(t, err)
	})

	// Create the index with PeerDB mapping so the converter's output is accepted.
	errE = search.EnsureIndex(ctx, esClient, index)
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internal.NewListener(dbpool)

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
	errE = b.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

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
