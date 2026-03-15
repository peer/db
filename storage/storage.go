// Package storage provides file storage functionality for PeerDB.
//
// This is a low-level component.
package storage

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internal "gitlab.com/peerdb/peerdb/internal/store"
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
}

type completeData struct {
	Buffer       []byte
	FileMetadata *FileMetadata
	EndMetadata  *endMetadata
	Chunks       int64
}

type completeMetadata struct {
	Chunks int64 `json:"chunks,omitempty"`

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

type chunkPos struct {
	start  int64
	length int64
	chunk  int64
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

	store       *store.Store[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]
	coordinator *coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *completeMetadata]
}

// Init initializes the Storage with the given database connection pool.
//
// A non-nil listener is required when the Committed channel is set.
func (s *Storage) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internal.Listener, schema string, riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if s.store != nil {
		return errors.New("already initialized")
	}

	storageStore := &store.Store[[]byte, *FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]{
		Prefix:       s.Prefix,
		DataType:     "bytea",
		MetadataType: "jsonb",
		PatchType:    "",
	}
	errE := storageStore.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	storageCoordinator := &coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *completeMetadata]{
		Prefix:            s.Prefix,
		DataType:          "bytea",
		MetadataType:      "jsonb",
		CompleteSession:   s.completeStorageSession,
		CompleteSessionTx: s.completeStorageSessionTx,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = storageCoordinator.Init(ctx, dbpool, nil, schema, riverClient, workers)
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

func (s *Storage) completeStorageSession(ctx context.Context, session identifier.Identifier) (*completeData, errors.E) {
	beginMetadata, endMetadata, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	if endMetadata.Discarded {
		return &completeData{
			Buffer:       nil,
			FileMetadata: nil,
			EndMetadata:  endMetadata,
			Chunks:       0,
		}, nil
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

	// This should match implementation in validateChunks.
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

	return &completeData{
		Buffer:       buffer,
		FileMetadata: metadata,
		EndMetadata:  endMetadata,
		Chunks:       int64(len(chunksList)),
	}, nil
}

func (s *Storage) completeStorageSessionTx(ctx context.Context, _ pgx.Tx, session identifier.Identifier, data *completeData) (*completeMetadata, errors.E) {
	if data.EndMetadata.Discarded {
		return &completeMetadata{
			Chunks: 0,
			Time:   time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	_, errE := s.store.Insert(ctx, session, data.Buffer, data.FileMetadata, &types.NoMetadata{})
	if errE != nil {
		return nil, errE
	}

	return &completeMetadata{
		Chunks: data.Chunks,
		Time:   time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
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

	beginMetadata, _, _, errE := s.coordinator.Get(ctx, session)
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
	// Validate chunks before ending the session so that errors are returned synchronously
	// and the session remains active for user to attempt to fix any error.
	errE := s.validateChunks(ctx, session)
	if errE != nil {
		return errE
	}

	metadata := &endMetadata{
		At:        types.Time(time.Now().UTC()),
		Discarded: false,
	}
	return s.coordinator.End(ctx, session, metadata)
}

// validateChunks checks that the uploaded chunks cover the full file without gaps.
func (s *Storage) validateChunks(ctx context.Context, session identifier.Identifier) errors.E {
	beginMetadata, _, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return errE
	}

	chunksList, errE := s.ListChunks(ctx, session)
	if errE != nil {
		return errE
	}

	chunks := make([]chunkPos, 0, len(chunksList))
	for _, c := range chunksList {
		start, length, errE := s.GetChunk(ctx, session, c)
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return errE
		}
		chunks = append(chunks, chunkPos{
			start:  start,
			length: length,
			chunk:  c,
		})
	}
	// chunksList is sorted from newest to the oldest chunk and we use a stable sort here, so the
	// result is that if there are multiple chunks at the same start, newer will be will be used first.
	slices.SortStableFunc(chunks, func(a, b chunkPos) int {
		return cmp.Compare(a.start, b.start)
	})

	size := int64(0)

	// This should match implementation in completeStorageSession.
	for _, p := range chunks {
		if p.start > size {
			errE = errors.Errorf("%w: gap between chunks", ErrEndNotPossible)
			errors.Details(errE)["end"] = size
			errors.Details(errE)["start"] = p.start
			errors.Details(errE)["chunk"] = p.chunk
			return errE
		}
		end := p.start + p.length
		if end > beginMetadata.Size {
			// This should have already been checked in UploadChunk so it is not an ErrEndNotPossible.
			errE = errors.New("chunk larger than file")
			errors.Details(errE)["start"] = p.start
			errors.Details(errE)["end"] = end
			errors.Details(errE)["size"] = beginMetadata.Size
			errors.Details(errE)["chunk"] = p.chunk
			return errE
		}
		if end <= size {
			continue
		}
		size = end
	}

	if size < beginMetadata.Size {
		errE = errors.Errorf("%w: chunks smaller than file", ErrEndNotPossible)
		errors.Details(errE)["chunks"] = size
		errors.Details(errE)["size"] = beginMetadata.Size
		return errE
	}

	return nil
}

// DiscardUpload discards an upload session without saving the file.
func (s *Storage) DiscardUpload(ctx context.Context, session identifier.Identifier) errors.E {
	metadata := &endMetadata{
		At:        types.Time(time.Now().UTC()),
		Discarded: true,
	}
	return s.coordinator.End(ctx, session, metadata)
}
