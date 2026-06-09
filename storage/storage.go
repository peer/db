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
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

type beginMetadata struct {
	At        store.Time `json:"at"`
	Base      []string   `json:"base"`
	Size      int64      `json:"size"`
	MediaType string     `json:"mediaType"`
	Filename  string     `json:"filename,omitempty"`
	// User is the user who opened the upload session. nil when unauthenticated.
	// Feeds the per-file Users union assembled at completion.
	User *store.User `json:"user,omitempty"`
}

type endMetadata struct {
	At             store.Time             `json:"at"`
	PrimarySession *identifier.Identifier `json:"primarySession,omitempty"`
	Discarded      bool                   `json:"discarded,omitempty"`
	// User is the user who ended the upload session (the committer). nil when
	// unauthenticated. Lands in CommitMetadata.User when the file is committed
	// standalone. NOT included in the Users union on FileMetadata.
	User *store.User `json:"user,omitempty"`
}

type completeData struct {
	Buffer       []byte
	FileMetadata *FileMetadata
	EndMetadata  *endMetadata
	Chunks       int64
}

// CompleteMetadata contains metadata captured when file upload session completes.
type CompleteMetadata struct {
	Discarded bool `json:"discarded,omitempty"`

	ID *identifier.Identifier `json:"id,omitempty"`

	Chunks int64 `json:"chunks,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

type chunkMetadata struct {
	At     store.Time `json:"at"`
	Start  int64      `json:"start"`
	Length int64      `json:"length"`
	// User is the user who uploaded this chunk. nil when unauthenticated.
	// Feeds the per-file Users union assembled at completion.
	User *store.User `json:"user,omitempty"`
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
	At        store.Time `json:"at"`
	Base      []string   `json:"base"`
	Size      int64      `json:"size"`
	MediaType string     `json:"mediaType"`
	Filename  string     `json:"filename,omitempty"`
	Etag      string     `json:"etag"`
	// Users is the deduplicated, sorted-by-ID union of users who contributed
	// to this upload: the user who began the session plus every user who
	// uploaded a chunk. The user who ended the session is NOT included; that
	// user goes to CommitMetadata.User on the committing changeset instead.
	Users []store.User `json:"users,omitempty"`
}

// PrimaryCoordinator is an interface enabling uploading files into
// changesets managed by the primary session coordinator.
type PrimaryCoordinator interface {
	// ChangesetID is run inside a transaction and should return the ID of the changeset
	// to upload the file into based on the session ID in the primary coordinator.
	ChangesetID(ctx context.Context, session identifier.Identifier) (identifier.Identifier, errors.E)
}

// completeSessionTimeout bounds how long completing a file-upload session may take. Completion copies the
// uploaded chunks into the file, which can be slow for large files, so it is more generous than the River
// client default used for fast jobs.
const completeSessionTimeout = time.Hour

// Storage provides file storage operations.
type Storage struct {
	// Schema is PostgreSQL schema used by this storage.
	Schema string

	// Prefix to use when initializing PostgreSQL objects used by this storage.
	Prefix string

	// PrimaryCoordinator can be set to the primary session coordinator which allows one to
	// upload files into changesets managed by the primary session coordinator.
	PrimaryCoordinator PrimaryCoordinator

	store       *store.Store[[]byte, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None]
	coordinator *coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata]
}

// Init initializes the Storage.
func (s *Storage) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internalStore.Listener, riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if s.store != nil {
		return errors.New("already initialized")
	}

	storageStore := &store.Store[[]byte, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None]{
		Schema:       s.Schema,
		Prefix:       s.Prefix,
		DataType:     "bytea",
		MetadataType: "jsonb",
		PatchType:    "",
	}
	errE := storageStore.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	storageCoordinator := &coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata]{
		Prefix:                 s.Prefix,
		DataType:               "bytea",
		MetadataType:           "jsonb",
		CompleteSession:        s.completeStorageSession,
		CompleteSessionTx:      s.completeStorageSessionTx,
		CompleteSessionTimeout: completeSessionTimeout,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = storageCoordinator.Init(ctx, dbpool, nil, s.Schema, riverClient, workers)
	if errE != nil {
		return errE
	}

	s.store = storageStore
	s.coordinator = storageCoordinator

	return nil
}

// Store returns the underlying store.Store instance.
func (s *Storage) Store() *store.Store[[]byte, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None] {
	return s.store
}

// Coordinator returns the underlying coordinator.Coordinator instance.
func (s *Storage) Coordinator() *coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata] {
	return s.coordinator
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

	// TODO: Support more than 5000 chunks.
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
	// result is that if there are multiple chunks at the same start, newer will be used first.
	// For chunks with different starts that partially overlap, the chunk with the larger start
	// is processed last and overwrites the overlapping region, regardless of upload order.
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

	base := slices.Clone(beginMetadata.Base)
	base = append(base, "STORAGE", session.String())

	// chunkUsers collects the begin user and every per-chunk user. The end
	// user is intentionally excluded (it belongs on CommitMetadata.User).
	// internalStore.SortedUniqueUsers drops nils, so unauthenticated participants are skipped.
	chunkUsers := make([]*store.User, 0, len(chunks)+1)
	chunkUsers = append(chunkUsers, beginMetadata.User)
	for i := range chunks {
		chunkUsers = append(chunkUsers, chunks[i].Metadata.User)
	}

	metadata := &FileMetadata{
		At:        endMetadata.At,
		Base:      base,
		Size:      beginMetadata.Size,
		MediaType: beginMetadata.MediaType,
		Filename:  beginMetadata.Filename,
		Etag:      x.ComputeEtag(buffer),
		Users:     internalStore.SortedUniqueUsers(chunkUsers),
	}

	return &completeData{
		Buffer:       buffer,
		FileMetadata: metadata,
		EndMetadata:  endMetadata,
		Chunks:       int64(len(chunksList)),
	}, nil
}

func (s *Storage) completeStorageSessionTx(ctx context.Context, _ pgx.Tx, session identifier.Identifier, data *completeData) (*CompleteMetadata, errors.E) {
	if data.EndMetadata.Discarded {
		return &CompleteMetadata{
			Discarded: true,
			ID:        nil,
			Chunks:    0,
			Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	id := identifier.From(data.FileMetadata.Base...)
	if data.EndMetadata.PrimarySession != nil {
		// Primary session was provided. We use it to obtain a changeset ID and then insert
		// the file into the changeset with that changeset ID, but we do NOT commit the changeset.
		changesetID, errE := s.PrimaryCoordinator.ChangesetID(ctx, *data.EndMetadata.PrimarySession)
		if errors.Is(errE, coordinator.ErrAlreadyEnded) || errors.Is(errE, coordinator.ErrAlreadyCompleted) {
			// The primary session has already ended or completed. We discard the file upload.
			return &CompleteMetadata{
				Discarded: true,
				ID:        nil,
				Chunks:    0,
				Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
			}, nil
		} else if errE != nil {
			return nil, errE
		}
		changeset, errE := s.store.Changeset(ctx, changesetID)
		if errE != nil {
			return nil, errE
		}
		_, errE = changeset.Insert(ctx, id, data.Buffer, data.FileMetadata)
		if errE != nil {
			return nil, errE
		}
	} else {
		// Changeset base was not provided, so we construct one from the file base.
		// That is the same construction we use for changeset base for ending a document session.
		changesetBase := slices.Clone(data.FileMetadata.Base)
		changesetBase = append(changesetBase, "SESSION", session.String())

		// We do not have to use the "tx" parameter because we access the transaction through ctx.
		_, errE := s.store.Insert(ctx, id, data.Buffer, data.FileMetadata, &store.CommitMetadata{
			Base: changesetBase,
			User: data.EndMetadata.User,
		})
		if errE != nil {
			return nil, errE
		}
	}

	return &CompleteMetadata{
		Discarded: false,
		ID:        &id,
		Chunks:    data.Chunks,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}

// BeginUploadNew starts a new file upload session.
func (s *Storage) BeginUploadNew(ctx context.Context, base []string, size int64, mediaType, filename string) (identifier.Identifier, errors.E) {
	metadata := &beginMetadata{
		At:        store.Time(time.Now().UTC()),
		Base:      base,
		Size:      size,
		MediaType: mediaType,
		Filename:  filename,
		User:      store.UserFromContext(ctx),
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
		At:     store.Time(time.Now().UTC()),
		Start:  start,
		Length: int64(len(chunk)),
		User:   store.UserFromContext(ctx),
	}
	_, errE = s.coordinator.Append(ctx, session, chunk, metadata, nil)
	return errE
}

// ListChunks returns a list of chunk IDs for an upload session, ordered from newest to oldest.
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
//
// It returns the ID of the file.
func (s *Storage) EndUpload(ctx context.Context, session identifier.Identifier, primarySession *identifier.Identifier) errors.E {
	if primarySession != nil && s.PrimaryCoordinator == nil {
		return errors.New("primary session coordinator not set")
	}

	// Validate chunks before ending the session so that errors are returned synchronously
	// and the session remains active for user to attempt to fix any error.
	errE := s.validateChunks(ctx, session)
	if errE != nil {
		return errE
	}

	metadata := &endMetadata{
		At:             store.Time(time.Now().UTC()),
		PrimarySession: primarySession,
		Discarded:      false,
		User:           store.UserFromContext(ctx),
	}
	return s.coordinator.End(ctx, session, metadata)
}

// validateChunks checks that the uploaded chunks cover the full file without gaps.
func (s *Storage) validateChunks(ctx context.Context, session identifier.Identifier) errors.E {
	beginMetadata, _, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return errE
	}

	// TODO: Support more than 5000 chunks.
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
	// result is that if there are multiple chunks at the same start, newer will be used first.
	// For chunks with different starts that partially overlap, the chunk with the larger start
	// is processed last and overwrites the overlapping region, regardless of upload order.
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
		At:             store.Time(time.Now().UTC()),
		PrimarySession: nil,
		Discarded:      true,
		User:           store.UserFromContext(ctx),
	}
	return s.coordinator.End(ctx, session, metadata)
}
