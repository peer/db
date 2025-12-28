// Package storage provides file storage functionality for PeerDB.
package storage

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

type beginMetadata struct {
	At        types.Time `json:"at"`
	Size      int64      `json:"size"`
	MediaType string     `json:"mediaType"`
	Filename  string     `json:"filename,omitempty"`
}

type endMetadata struct {
	At        types.Time `json:"at"`
	Discarded bool       `json:"discarded,omitempty"`
	Chunks    int64      `json:"chunks,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

type chunkMetadata struct {
	At     types.Time `json:"at"`
	Start  int64      `json:"start"`
	Length int64      `json:"length"`
}

type chunk struct {
	Chunk    int64
	Data     []byte
	Metadata chunkMetadata
}

// FileMetadata contains metadata about a stored file.
type FileMetadata struct {
	At        types.Time `json:"at"`
	Size      int64      `json:"size"`
	MediaType string     `json:"mediaType"`
	Filename  string     `json:"filename,omitempty"`
	Etag      string     `json:"etag"`
}

// Storage provides file storage operations.
type Storage struct {
	// Prefix to use when initializing PostgreSQL objects used by this storage.
	Prefix string

	// A channel to which changesets are send when they are committed.
	// The changesets and view objects sent do not have an associated Store.
	//
	// The order in which they are sent is not necessary the order in which
	// they were committed. You should not rely on the order.
	Committed chan<- store.CommittedChangeset[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]

	store       *store.Store[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]
	coordinator *coordinator.Coordinator[[]byte, *beginMetadata, *endMetadata, *chunkMetadata]
}

// Init initializes the Storage with the given database connection pool.
func (s *Storage) Init(ctx context.Context, dbpool *pgxpool.Pool) errors.E {
	if s.store != nil {
		return errors.New("already initialized")
	}

	storageStore := &store.Store[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]{
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

	storageCoordinator := &coordinator.Coordinator[[]byte, *beginMetadata, *endMetadata, *chunkMetadata]{
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

// Store returns the underlying store.Store instance.
func (s *Storage) Store() *store.Store[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None] {
	return s.store
}

func (s *Storage) endCallback(ctx context.Context, session identifier.Identifier, endMetadata *endMetadata) (*endMetadata, errors.E) {
	if endMetadata.Discarded {
		return nil, nil //nolint:nilnil
	}

	beginMetadata, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	chunksList, errE := s.ListChunks(ctx, session)
	if errE != nil {
		return nil, errE
	}

	chunks := make([]chunk, 0, len(chunksList))
	for _, c := range chunksList {
		data, metadata, errE := s.coordinator.GetData(ctx, session, c)
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return nil, errE
		}
		chunks = append(chunks, chunk{
			Chunk:    c,
			Data:     data,
			Metadata: *metadata,
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
			errE = errors.Errorf("%w: gap between chunks", ErrEndNotPossible)
			errors.Details(errE)["end"] = size
			errors.Details(errE)["start"] = c.Metadata.Start
			errors.Details(errE)["chunk"] = c.Chunk
			return nil, errE
		}
		end := c.Metadata.Start + c.Metadata.Length
		if end > beginMetadata.Size {
			// This should have already been checked in UploadChunk so it is not an ErrEndNotPossible.
			errE = errors.New("chunk larger than file")
			errors.Details(errE)["start"] = c.Metadata.Start
			errors.Details(errE)["end"] = end
			errors.Details(errE)["size"] = beginMetadata.Size
			errors.Details(errE)["chunk"] = c.Chunk
			return nil, errE
		}
		if end <= size {
			// We already have this data.
			continue
		}

		copy(buffer[c.Metadata.Start:end], c.Data)
		size = end
	}

	if size < beginMetadata.Size {
		errE = errors.Errorf("%w: chunks smaller than file", ErrEndNotPossible)
		errors.Details(errE)["chunks"] = size
		errors.Details(errE)["size"] = beginMetadata.Size
		return nil, errE
	}

	metadata := &FileMetadata{
		At:        endMetadata.At,
		Size:      beginMetadata.Size,
		MediaType: beginMetadata.MediaType,
		Filename:  beginMetadata.Filename,
		Etag:      computeEtag(buffer),
	}

	_, errE = s.store.Insert(ctx, session, buffer, metadata, &types.NoMetadata{})
	if errE != nil {
		return nil, errE
	}

	endMetadata.Chunks = int64(len(chunksList))
	endMetadata.Time = time.Since(time.Time(endMetadata.At)).Milliseconds()
	return endMetadata, nil
}

// BeginUpload starts a new file upload session.
func (s *Storage) BeginUpload(ctx context.Context, size int64, mediaType, filename string) (identifier.Identifier, errors.E) {
	metadata := &beginMetadata{
		At:        types.Time(time.Now().UTC()),
		Size:      size,
		MediaType: mediaType,
		Filename:  filename,
	}
	return s.coordinator.Begin(ctx, metadata)
}

// UploadChunk uploads a chunk of data for an ongoing upload session.
func (s *Storage) UploadChunk(ctx context.Context, session identifier.Identifier, chunk []byte, start int64) errors.E {
	if len(chunk) == 0 {
		return errors.Errorf("%w: zero length chunk", ErrInvalidChunk)
	}

	beginMetadata, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return errE
	}
	end := start + int64(len(chunk))
	if end > beginMetadata.Size {
		errE = errors.Errorf("%w: chunk larger than file", ErrInvalidChunk)
		errors.Details(errE)["start"] = start
		errors.Details(errE)["end"] = end
		errors.Details(errE)["size"] = beginMetadata.Size
		return errE
	}

	metadata := &chunkMetadata{
		At:     types.Time(time.Now().UTC()),
		Start:  start,
		Length: int64(len(chunk)),
	}
	_, errE = s.coordinator.Append(ctx, session, chunk, metadata, nil)
	return errE
}

// ListChunks returns a list of chunk IDs for an upload session.
func (s *Storage) ListChunks(ctx context.Context, session identifier.Identifier) ([]int64, errors.E) {
	// TODO: Support more than 5000 chunks.
	return s.coordinator.List(ctx, session, nil)
}

// GetChunk retrieves the start position and length of a chunk.
func (s *Storage) GetChunk(ctx context.Context, session identifier.Identifier, chunk int64) (int64, int64, errors.E) {
	metadata, errE := s.coordinator.GetMetadata(ctx, session, chunk)
	if errE != nil {
		return 0, 0, errE
	}
	return metadata.Start, metadata.Length, nil
}

// EndUpload finalizes an upload session and assembles the file.
func (s *Storage) EndUpload(ctx context.Context, session identifier.Identifier) errors.E {
	metadata := &endMetadata{
		At:        types.Time(time.Now().UTC()),
		Discarded: false,
		Chunks:    0,
		Time:      0,
	}
	_, errE := s.coordinator.End(ctx, session, metadata)
	return errE
}

// DiscardUpload discards an upload session without saving the file.
func (s *Storage) DiscardUpload(ctx context.Context, session identifier.Identifier) errors.E {
	metadata := &endMetadata{
		At:        types.Time(time.Now().UTC()),
		Discarded: true,
		Chunks:    0,
		Time:      0,
	}
	_, errE := s.coordinator.End(ctx, session, metadata)
	return errE
}
