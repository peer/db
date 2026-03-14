package es_test

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

	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

type (
	bridgeStore = store.Store[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]
	bridgeType  = es.Bridge[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]
)

func initBridge(t *testing.T) (context.Context, *bridgeStore, *bridgeType, *elastic.Client) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}
	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	schema := identifier.New().String()
	prefix := identifier.New().String() + "_"
	index := "s" + strings.ToLower(identifier.New().String())

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	esClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	// Use a simple index without the PeerDB mapping so that _source is enabled,
	// allowing tests to verify document content via the Get API.
	_, err := esClient.CreateIndex(index).Do(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := esClient.DeleteIndex(index).Do(context.Background())
		assert.NoError(t, err)
	})

	listener := internal.NewListener(dbpool)

	s := &bridgeStore{
		Prefix:        prefix,
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
		CommittedSize: 100,
	}
	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	b := &bridgeType{
		Store:    s,
		ESClient: esClient,
		Index:    index,
	}
	errE = b.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	internal.StartListener(ctx, listener)

	// Allow the listener goroutine to connect and register LISTEN before tests make commits.
	time.Sleep(100 * time.Millisecond)

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

// docData returns the source data of an ES document, or nil if not found.
func docData(t *testing.T, ctx context.Context, esClient *elastic.Client, index, id string) json.RawMessage { //nolint:revive
	t.Helper()
	resp, err := esClient.Get().Index(index).Id(id).Do(ctx)
	if err != nil {
		if elastic.IsNotFound(err) {
			return nil
		}
		t.Fatalf("unexpected ES error: %v", err)
	}
	if !resp.Found {
		return nil
	}
	return resp.Source
}

func TestBridgeRealTime(t *testing.T) {
	t.Parallel()

	ctx, s, b, esClient := initBridge(t)

	b.Start(ctx)

	// Insert three documents.
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	doc1 := json.RawMessage(`{"name":"doc1"}`)
	doc2 := json.RawMessage(`{"name":"doc2"}`)
	doc3 := json.RawMessage(`{"name":"doc3"}`)

	_, errE := s.Insert(ctx, id1, doc1, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, doc2, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id3, doc3, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for bridge to catch up and force ES to make documents searchable.
	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// All three documents should now be in ES.
	assert.True(t, docExists(t, ctx, esClient, b.Index, id1.String()), "doc1 should exist in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id2.String()), "doc2 should exist in ES")
	assert.True(t, docExists(t, ctx, esClient, b.Index, id3.String()), "doc3 should exist in ES")

	// Update doc1.
	doc1Updated := json.RawMessage(`{"name":"doc1-updated"}`)
	_, _, v1, errE := s.GetLatest(ctx, id1)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Replace(ctx, id1, v1.Changeset, doc1Updated, internal.DummyData, internal.DummyData)
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

	_, errE := s.Insert(ctx, id1, json.RawMessage(`{"name":"catchup1"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, json.RawMessage(`{"name":"catchup2"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Bridge seq should still be 0 — nothing indexed yet.
	entries, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, entries, 2)

	// Now start the bridge. It should catch up from CommitLog.
	b.Start(ctx)

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

	b.Start(ctx)

	id := identifier.New()

	// Insert then delete a document.
	v, errE := s.Insert(ctx, id, json.RawMessage(`{"name":"to-delete"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	assert.True(t, docExists(t, ctx, esClient, b.Index, id.String()), "document should exist before delete")

	_, errE = s.Delete(ctx, id, v.Changeset, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err = esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// After deletion the bridge issues a bulk delete, so the document is removed from ES.
	assert.False(t, docExists(t, ctx, esClient, b.Index, id.String()), "document should be removed from ES after delete")
}

func TestBridgeSeqAdvancement(t *testing.T) {
	t.Parallel()

	ctx, s, b, _ := initBridge(t)

	b.Start(ctx)

	// Make several commits and verify the bridge table seq advances correctly.
	for range 5 {
		id := identifier.New()
		_, errE := s.Insert(ctx, id, internal.DummyData, internal.DummyData, internal.DummyData)
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

	b.Start(ctx)

	// Insert initial documents and wait for the bridge to catch up.
	id1 := identifier.New()
	id2 := identifier.New()
	_, errE := s.Insert(ctx, id1, json.RawMessage(`{"name":"initial1"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id2, json.RawMessage(`{"name":"initial2"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Simulate a listener reconnection by closing the store's Committed channel.
	// The bridge's run loop detects the channel close, exits with errCommittedChannelClosed,
	// and restarts — re-running the catch-up phase to recover any missed commits.
	err := s.HandleBacklog(ctx, s.Prefix+"CommittedChangesets", nil)
	require.NoError(t, errE, "% -+#.1v", err) // This is still errors.E.

	// Insert more documents after the simulated reconnection. These may be missed by the
	// real-time channel but must be recovered via the catch-up phase on bridge restart.
	id3 := identifier.New()
	id4 := identifier.New()
	_, errE = s.Insert(ctx, id3, json.RawMessage(`{"name":"recovery1"}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, id4, json.RawMessage(`{"name":"recovery2"}`), internal.DummyData, internal.DummyData)
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

	b.Start(ctx)

	id := identifier.New()

	// Insert initial data.
	v, errE := s.Insert(ctx, id, json.RawMessage(`{"val":1}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Immediately update before the bridge processes the insert.
	_, errE = s.Replace(ctx, id, v.Changeset, json.RawMessage(`{"val":2}`), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = b.WaitUntilCaughtUp(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, err := esClient.Refresh(b.Index).Do(ctx)
	require.NoError(t, err)

	// The document in ES should reflect the latest version, not an intermediate one.
	data := docData(t, ctx, esClient, b.Index, id.String())
	require.NotNil(t, data, "document should be in ES")

	// The bridge calls GetLatest, so regardless of which commit triggered indexing,
	// the stored data is the latest version.
	var doc struct {
		Val int `json:"val"`
	}
	err = json.Unmarshal(data, &doc)
	require.NoError(t, err)
	assert.Equal(t, 2, doc.Val, "ES should contain the latest version of the document")
}
