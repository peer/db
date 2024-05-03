package storage

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/store"
)

type fileMetadata struct {
	At        time.Time `json:"at"`
	Size      int64     `json:"size"`
	MediaType string    `json:"mediaType"`
	Filename  string    `json:"filename"`
}

type endMetadata struct {
	At     time.Time `json:"at"`
	Chunks int64     `json:"chunks"`
	// Processing time in milliseconds.
	Time int64 `json:"time"`
}

type chunkMetadata struct {
	At     time.Time `json:"at"`
	Start  int64     `json:"start"`
	Length int64     `json:"length"`
}

type chunk struct {
	Chunk    int64
	Data     []byte
	Metadata chunkMetadata
}

type Storage struct {
	// Prefix to use when initializing PostgreSQL objects used by this storage.
	Prefix string

	// A channel to which changesets are send when they are committed.
	// The changesets and view objects sent do not have an associated Store.
	//
	// The order in which they are sent is not necessary the order in which
	// they were committed. You should not rely on the order.
	Committed chan<- store.CommittedChangeset[[]byte, json.RawMessage, store.None]

	store       *store.Store[[]byte, json.RawMessage, store.None]
	coordinator *coordinator.Coordinator[[]byte, json.RawMessage]
}

func (s *Storage) Init(ctx context.Context, dbpool *pgxpool.Pool) errors.E {
	if s.store != nil {
		return errors.New("already initialized")
	}

	storageStore := &store.Store[[]byte, json.RawMessage, store.None]{
		Prefix:       s.Prefix,
		Committed:    s.Committed,
		DataType:     "bytea",
		MetadataType: "jsonb",
		PatchType:    "",
	}
	errE := storageStore.Init(ctx, dbpool)
	if errE != nil {
		return errE
	}

	storageCoordinator := &coordinator.Coordinator[[]byte, json.RawMessage]{
		Prefix:       s.Prefix,
		DataType:     "bytea",
		MetadataType: "jsonb",
		EndCallback:  s.endCallback,
		Appended:     nil,
		Ended:        nil,
	}
	errE = storageCoordinator.Init(ctx, dbpool)
	if errE != nil {
		return errE
	}

	s.store = storageStore
	s.coordinator = storageCoordinator

	return nil
}

func (s *Storage) Store() *store.Store[[]byte, json.RawMessage, store.None] {
	return s.store
}

func (s *Storage) endCallback(ctx context.Context, session identifier.Identifier, endMetadataJSON json.RawMessage) (json.RawMessage, errors.E) {
	beginMetadataJSON, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return endMetadataJSON, errE
	}
	var beginMetadata fileMetadata
	errE = x.UnmarshalWithoutUnknownFields(beginMetadataJSON, &beginMetadata)
	if errE != nil {
		return endMetadataJSON, errE
	}
	var endMetadata endMetadata //nolint:govet
	errE = x.UnmarshalWithoutUnknownFields(endMetadataJSON, &endMetadata)
	if errE != nil {
		return endMetadataJSON, errE
	}

	chunksList, errE := s.ListChunks(ctx, session)
	if errE != nil {
		return endMetadataJSON, errE
	}

	chunks := make([]chunk, 0, len(chunksList))
	for _, c := range chunksList {
		data, metadataJSON, errE := s.coordinator.GetData(ctx, session, c) //nolint:govet
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return endMetadataJSON, errE
		}
		var metadata chunkMetadata
		errE = x.UnmarshalWithoutUnknownFields(metadataJSON, &metadata)
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return endMetadataJSON, errE
		}
		chunks = append(chunks, chunk{
			Chunk:    c,
			Data:     data,
			Metadata: metadata,
		})
	}
	// chunksList is sorted from newest to the oldest chunk and we use a stable sort here, so the
	// result is that if there are multiple chunks at the same start, newer will be will be used first.
	slices.SortStableFunc(chunks, func(a, b chunk) int {
		return cmp.Compare(a.Metadata.Start, b.Metadata.Start)
	})

	// TODO: Do not do this in memory.
	//       This opens a simple attack where attacker begins upload claiming large size and then ends it, requiring us to allocate memory here.
	size := int64(0)
	buffer := make([]byte, beginMetadata.Size)

	for _, c := range chunks {
		if c.Metadata.Start > size {
			errE = errors.New("gap between chunks")
			errors.Details(errE)["end"] = size
			errors.Details(errE)["start"] = c.Metadata.Start
			errors.Details(errE)["chunk"] = c.Chunk
			return endMetadataJSON, errE
		}
		end := c.Metadata.Start + c.Metadata.Length
		if end > beginMetadata.Size {
			errE = errors.New("chunk larger than file")
			errors.Details(errE)["start"] = c.Metadata.Start
			errors.Details(errE)["end"] = c.Metadata.Start + c.Metadata.Length
			errors.Details(errE)["size"] = beginMetadata.Size
			errors.Details(errE)["chunk"] = c.Chunk
			return endMetadataJSON, errE
		}
		if end <= size {
			// We already have this data.
			continue
		}

		copy(buffer[c.Metadata.Start:end], c.Data)
		size = end
	}

	if size < beginMetadata.Size {
		errE = errors.New("chunks smaller than file")
		errors.Details(errE)["chunks"] = size
		errors.Details(errE)["size"] = beginMetadata.Size
		return endMetadataJSON, errE
	}

	beginMetadata.At = time.Now().UTC()
	beginMetadataJSON, errE = x.MarshalWithoutEscapeHTML(beginMetadata)
	if errE != nil {
		return endMetadataJSON, errE
	}

	_, errE = s.store.Insert(ctx, session, buffer, beginMetadataJSON)
	if errE != nil {
		return endMetadataJSON, errE
	}

	endMetadata.Chunks = int64(len(chunksList))
	endMetadata.Time = time.Since(endMetadata.At).Milliseconds()
	return x.MarshalWithoutEscapeHTML(endMetadata)
}

func (s *Storage) BeginUpload(ctx context.Context, size int64, mediaType, filename string) (identifier.Identifier, errors.E) {
	metadata := fileMetadata{
		At:        time.Now().UTC(),
		Size:      size,
		MediaType: mediaType,
		Filename:  filename,
	}
	metadataJSON, errE := x.MarshalWithoutEscapeHTML(metadata)
	if errE != nil {
		return identifier.Identifier{}, errE
	}
	return s.coordinator.Begin(ctx, metadataJSON)
}

func (s *Storage) UploadChunk(ctx context.Context, session identifier.Identifier, chunk []byte, start int64) errors.E {
	metadata := chunkMetadata{
		At:     time.Now().UTC(),
		Start:  start,
		Length: int64(len(chunk)),
	}
	metadataJSON, errE := x.MarshalWithoutEscapeHTML(metadata)
	if errE != nil {
		return errE
	}
	_, errE = s.coordinator.Push(ctx, session, chunk, metadataJSON)
	return errE
}

func (s *Storage) ListChunks(ctx context.Context, session identifier.Identifier) ([]int64, errors.E) {
	// TODO: Support more than 5000 chunks.
	return s.coordinator.List(ctx, session, nil)
}

func (s *Storage) GetChunk(ctx context.Context, session identifier.Identifier, chunk int64) (int64, int64, errors.E) {
	metadataJSON, errE := s.coordinator.GetMetadata(ctx, session, chunk)
	if errE != nil {
		return 0, 0, errE
	}
	var metadata chunkMetadata
	errE = x.UnmarshalWithoutUnknownFields(metadataJSON, &metadata)
	if errE != nil {
		return 0, 0, errE
	}
	return metadata.Start, metadata.Length, nil
}

func (s *Storage) EndUpload(ctx context.Context, session identifier.Identifier) errors.E {
	metadata := endMetadata{
		At:     time.Now().UTC(),
		Chunks: 0,
		Time:   0,
	}
	metadataJSON, errE := x.MarshalWithoutEscapeHTML(metadata)
	if errE != nil {
		return errE
	}
	return s.coordinator.End(ctx, session, metadataJSON)
}
