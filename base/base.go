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
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/search"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	// TODO: Determine reasonable size for the buffer.
	// TODO: Add some monitoring of the channel contention.
	bridgeBufferSize = 100
)

// B is a base for data and files.
//
//nolint:lll
type B struct {
	Schema string
	Index  string

	LanguagePriority map[string][]string

	// Data type for Store is on purpose not document.D so that we can serve it directly without doing first JSON unmarshal just to marshal it again immediately.
	documents   *store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes]
	coordinator *coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *documentCompleteMetadata]
	files       *storage.Storage
	bridge      *search.Bridge
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

	documents := &store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes]{
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

	bridge := &search.Bridge{
		Store:    documents,
		ESClient: esClient,
		Index:    b.Index,
	}
	errE = bridge.Init(ctx, dbpool, listener, b.Schema, riverClient, workers)
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
//
// Documents are documents with properties and vocabularies which are used
// to index documents for search.
//
// You have to call this or PopulateAndStart for each base after Init.
func (b *B) Start(ctx context.Context, documents []*document.D) errors.E {
	converter, errE := search.NewConverter(
		documents, documents, b.LanguagePriority,
		func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
			doc, _, _, _, errE := b.GetDocumentLatestDoc(ctx, id)
			if errE != nil {
				return nil, errE
			}
			return doc, nil
		},
	)
	if errE != nil {
		return errE
	}

	b.bridge.Start(ctx, converter)
	return nil
}
