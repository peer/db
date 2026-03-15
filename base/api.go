package base

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// GetDocument returns the document at the given version.
func (b *B) GetDocument(ctx context.Context, id identifier.Identifier, version store.Version) (json.RawMessage, *DocumentMetadata, errors.E) {
	return b.documents.Get(ctx, id, version)
}

// GetDocumentLatest returns the latest version of the document.
func (b *B) GetDocumentLatest(ctx context.Context, id identifier.Identifier) (json.RawMessage, *DocumentMetadata, store.Version, errors.E) {
	return b.documents.GetLatest(ctx, id)
}

// InsertDocument inserts a new document with the given ID.
func (b *B) InsertDocument(ctx context.Context, id identifier.Identifier, documentJSON json.RawMessage) errors.E {
	_, errE := b.documents.Insert(ctx, id, documentJSON, &DocumentMetadata{
		At: internal.Time(time.Now().UTC()),
	}, &internal.NoMetadata{})
	return errE
}

// BeginDocumentEdit begins an edit session for the document at the given version.
func (b *B) BeginDocumentEdit(ctx context.Context, id identifier.Identifier, version store.Version) (identifier.Identifier, errors.E) {
	return b.coordinator.Begin(ctx, &DocumentBeginMetadata{
		At:      internal.Time(time.Now().UTC()),
		ID:      id,
		Version: version,
	})
}

// AppendDocumentChange appends a change to an edit session at the given sequence number.
func (b *B) AppendDocumentChange(ctx context.Context, session identifier.Identifier, data json.RawMessage, seqNo *int64) (int64, errors.E) {
	return b.coordinator.Append(ctx, session, data, &documentChangeMetadata{
		At: internal.Time(time.Now().UTC()),
	}, seqNo)
}

// ListDocumentChanges returns the sequence numbers of all changes in an edit session.
func (b *B) ListDocumentChanges(ctx context.Context, session identifier.Identifier) ([]int64, errors.E) {
	return b.coordinator.List(ctx, session, nil)
}

// GetDocumentChange returns the change data at the given sequence number in an edit session.
func (b *B) GetDocumentChange(ctx context.Context, session identifier.Identifier, seqNo int64) (json.RawMessage, errors.E) {
	data, _, errE := b.coordinator.GetData(ctx, session, seqNo)
	return data, errE
}

// EndDocumentEdit ends an edit session, committing or discarding its changes.
func (b *B) EndDocumentEdit(ctx context.Context, session identifier.Identifier, discard bool) errors.E {
	return b.coordinator.End(ctx, session, &documentEndMetadata{
		At:        internal.Time(time.Now().UTC()),
		Discarded: discard,
	})
}

// GetDocumentEditSession returns the begin metadata for an active edit session.
func (b *B) GetDocumentEditSession(ctx context.Context, session identifier.Identifier) (*DocumentBeginMetadata, errors.E) {
	beginMetadata, endMetadata, _, errE := b.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	} else if endMetadata != nil {
		return nil, errors.WithStack(coordinator.ErrAlreadyEnded)
	}
	return beginMetadata, nil
}

// BeginUpload begins a chunked file upload session.
func (b *B) BeginUpload(ctx context.Context, size int64, mediaType, filename string) (identifier.Identifier, errors.E) {
	return b.files.BeginUpload(ctx, size, mediaType, filename)
}

// UploadChunk appends a chunk of data to a file upload session at the given byte offset.
func (b *B) UploadChunk(ctx context.Context, session identifier.Identifier, chunk []byte, start int64) errors.E {
	return b.files.UploadChunk(ctx, session, chunk, start)
}

// ListChunks returns the sequence numbers of all uploaded chunks in a file upload session.
func (b *B) ListChunks(ctx context.Context, session identifier.Identifier) ([]int64, errors.E) {
	return b.files.ListChunks(ctx, session)
}

// GetChunk returns the byte offset and length of a chunk in a file upload session.
func (b *B) GetChunk(ctx context.Context, session identifier.Identifier, chunk int64) (int64, int64, errors.E) {
	return b.files.GetChunk(ctx, session, chunk)
}

// EndUpload finalizes a file upload session, assembling the uploaded chunks.
func (b *B) EndUpload(ctx context.Context, session identifier.Identifier) errors.E {
	return b.files.EndUpload(ctx, session)
}

// DiscardUpload discards a file upload session without storing the file.
func (b *B) DiscardUpload(ctx context.Context, session identifier.Identifier) errors.E {
	return b.files.DiscardUpload(ctx, session)
}

// GetFile returns the data and metadata for a stored file.
func (b *B) GetFile(ctx context.Context, id identifier.Identifier) ([]byte, *storage.FileMetadata, errors.E) {
	data, metadata, _, errE := b.files.Store().GetLatest(ctx, id)
	return data, metadata, errE
}
