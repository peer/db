// Package base provides the main entry point for storing and managing data and files in PeerDB.
//
// It is a high-level component which wraps multiple lower-level components and offers
// an unified API for storing and managing data and files in PeerDB.
//
// It supports two types of data:
//
//   - PeerDB documents.
//   - Files.
package base

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	// TODO: Determine reasonable size for the buffer.
	// TODO: Add some monitoring of the channel contention.
	bridgeBufferSize = 100
)

// DocumentMetadata contains metadata about a document including its timestamp.
type DocumentMetadata struct {
	At types.Time `json:"at"`
}

// B is a base for data and files.
//
//nolint:lll
type B struct {
	Schema string
	Index  string

	// Data type for Store is on purpose not document.D so that we can serve it directly without doing first JSON unmarshal just to marshal it again immediately.
	documents   *store.Store[json.RawMessage, *DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]
	coordinator *coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *documentCompleteMetadata]
	files       *storage.Storage
	bridge      *es.Bridge[json.RawMessage, *DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]
}

// Init initializes the base.
func (b *B) Init(
	ctx context.Context,
	dbpool *pgxpool.Pool, listener *internal.Listener,
	esClient *elastic.Client,
	riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if b.documents != nil {
		return errors.New("already initialized")
	}

	documents := &store.Store[json.RawMessage, *DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]{
		Prefix:        "docs",
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
		CommittedSize: bridgeBufferSize,
	}
	errE := documents.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	c := &coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *documentCompleteMetadata]{
		Prefix:            "docs",
		DataType:          "jsonb",
		MetadataType:      "jsonb",
		CompleteSession:   b.completeDocumentSession,
		CompleteSessionTx: b.completeDocumentSessionTx,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = c.Init(ctx, dbpool, nil, b.Schema, riverClient, workers)
	if errE != nil {
		return errE
	}

	files := &storage.Storage{
		Schema: b.Schema,
		Prefix: "files",
	}
	// We do not use the underlying store's Committed channel here so we pass nil as listener.
	errE = files.Init(ctx, dbpool, nil, riverClient, workers)
	if errE != nil {
		return errE
	}

	bridge := &es.Bridge[json.RawMessage, *DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes]{
		Store:    documents,
		ESClient: esClient,
		Index:    b.Index,
	}
	errE = bridge.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	b.documents = documents
	b.coordinator = c
	b.files = files
	b.bridge = bridge

	return nil
}

// Start starts the base.
func (b *B) Start(ctx context.Context) errors.E {
	b.bridge.Start(ctx)
	return nil
}
