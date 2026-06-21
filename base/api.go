package base

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// GetDocument returns the document at the given version as raw JSON.
//
// It returns also document metadata, the version of the document (if requested version
// has 0 for revision, a document with the latest revision is returned and returned version
// contains this revision number), and parent changesets of the document at this version.
func (b *B) GetDocument(
	ctx context.Context, id identifier.Identifier, version store.Version,
) (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return b.withHooks(ctx, id, &version, func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		return b.documents.Get(ctx, id, version)
	})
}

// GetDocumentLatest returns the latest version of the document as raw JSON.
//
// It returns also document metadata, the version of the document, and parent
// changesets of the document at this version.
func (b *B) GetDocumentLatest(
	ctx context.Context, id identifier.Identifier,
) (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return b.withHooks(ctx, id, nil, func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
		return b.documents.GetLatest(ctx, id)
	})
}

// GetDocumentLatestDoc returns the latest version of the document as document.D.
//
// It returns also document metadata, the version of the document, and parent
// changesets of the document at this version.
func (b *B) GetDocumentLatestDoc(ctx context.Context, id identifier.Identifier) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	return b.withDocumentHooks(ctx, id, nil,
		func() (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
			return b.documents.GetLatest(ctx, id)
		},
	)
}

// Documents returns the underlying documents store. It reads the raw stored documents directly and unfiltered,
// without the read-path document hooks (and thus any permission checks).
func (b *B) Documents() *store.Store[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes] {
	return b.documents
}

// DocumentChangeset returns the requested changeset from the document store.
func (b *B) DocumentChangeset(ctx context.Context, id identifier.Identifier) (
	store.Changeset[json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes],
	errors.E,
) {
	return b.documents.Changeset(ctx, id)
}

// GetDocumentFromChangeset returns the document at the given revision in the changeset as raw JSON.
//
// If revision is 0, the latest revision is returned.
//
// If the document has been deleted in the changeset, it returns ErrValueDeleted,
// but other returned values are valid as well.
func (b *B) GetDocumentFromChangeset(
	ctx context.Context, changesetID, id identifier.Identifier, revision int64,
) (json.RawMessage, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	changeset, errE := b.documents.Changeset(ctx, changesetID)
	if errE != nil {
		return nil, nil, store.Version{}, nil, errE
	}
	return changeset.Get(ctx, id, revision)
}

// FileChangeset returns the requested changeset from the file store.
func (b *B) FileChangeset(ctx context.Context, id identifier.Identifier) (
	store.Changeset[string, *storage.FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None],
	errors.E,
) {
	return b.files.Store().Changeset(ctx, id)
}

// GetFileFromChangeset returns an open handle on the file at the given revision in the changeset.
// The caller is responsible for closing the returned handle.
//
// If revision is 0, the latest revision is returned.
//
// If the file has been deleted in the changeset, it returns ErrValueDeleted,
// but other returned values are valid as well.
func (b *B) GetFileFromChangeset(
	ctx context.Context, changesetID, id identifier.Identifier, revision int64,
) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	return b.files.GetFromChangeset(ctx, changesetID, id, revision)
}

// InsertDocument inserts a new document.
//
// Document with same ID cannot yet exist in the base.
func (b *B) InsertDocument(ctx context.Context, doc *document.D) errors.E {
	errE := doc.Validate()
	if errE != nil {
		return errE
	}

	documentJSON, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}

	// Each doc.ID has to be unique, so each doc.Base is unique as well
	// (validate above validated the link between ID and Base).
	changesetBase := slices.Clone(doc.Base)
	changesetBase = append(changesetBase, "CHANGESET", "FIRST")
	user := store.UserFromContext(ctx)
	_, errE = b.documents.Insert(ctx, doc.ID, documentJSON, &store.DocumentMetadata{
		At:               store.Time(time.Now().UTC()),
		Users:            internalStore.SortedUniqueUsers([]*store.User{user}),
		InverseRelations: nil,
		Embedding:        nil,
	}, &store.CommitMetadata{
		Base: changesetBase,
		User: user,
	})
	return errE
}

// DeleteDocument deletes the latest version of the document, committing a delete changeset.
//
// The delete is recorded as a new version (the document's data becomes nil) so the change history is
// preserved and the document can be undeleted later. System-managed metadata (inverse relations) is
// carried over so it is restored if the document is undeleted.
func (b *B) DeleteDocument(ctx context.Context, id identifier.Identifier) errors.E {
	doc, oldMetadata, version, _, errE := b.GetDocumentLatestDoc(ctx, id)
	if errE != nil {
		return errE
	}

	user := store.UserFromContext(ctx)

	// The changeset base mirrors the convention used elsewhere (doc base plus a marker and a unique
	// component), so the derived changeset ID does not collide with the document's other changesets.
	changesetBase := slices.Clone(doc.Base)
	changesetBase = append(changesetBase, "CHANGESET", identifier.New().String())

	metadata := &store.DocumentMetadata{
		At:               store.Time(time.Now().UTC()),
		Users:            internalStore.SortedUniqueUsers([]*store.User{user}),
		InverseRelations: nil,
		Embedding:        nil,
	}
	metadata.CarryOver(oldMetadata)

	_, errE = b.documents.Delete(ctx, id, version.Changeset, metadata, &store.CommitMetadata{
		Base: changesetBase,
		User: user,
	})
	return errE
}

// TODO: Add also a version of BeginEditDocumentLatest method which allows you to specify the version of the document from which the edit session should start.

// BeginEditDocumentLatest begins an edit session for the latest version of the document.
//
// It returns session ID and the version of the document from which the edit session started.
func (b *B) BeginEditDocumentLatest(ctx context.Context, id identifier.Identifier) (identifier.Identifier, store.Version, errors.E) {
	documentJSON, _, version, _, errE := b.GetDocumentLatest(ctx, id)
	if errE != nil {
		// TODO: ErrValueNotFound error should make the caller return NotFoundWithError.
		return identifier.Identifier{}, store.Version{}, errE
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(documentJSON, &doc)
	if errE != nil {
		return identifier.Identifier{}, store.Version{}, errE
	}

	session, errE := b.coordinator.Begin(ctx, &DocumentBeginMetadata{
		At:         store.Time(time.Now().UTC()),
		DocumentID: id,
		Base:       doc.Base,
		Version: &store.Version{
			Changeset: version.Changeset,
			// We set revision to 0 so that system metadata updates (e.g., inverse relations)
			// that bump the revision do not invalidate the session.
			Revision: 0,
		},
		User: store.UserFromContext(ctx),
	})
	return session, version, errE
}

// BeginCreateDocument opens a coordinator session for creating a brand-new document.
//
// The document is not inserted into the store at this point. The session
// accumulates changes (claim additions, etc.) and EndEditDocument commits them
// by inserting an empty document with the given id/base and then applying the
// accumulated changes as a second changeset (so the patch history records the
// transition from empty to populated).
func (b *B) BeginCreateDocument(ctx context.Context, base []string) (identifier.Identifier, errors.E) {
	id := identifier.From(base...)
	return b.coordinator.Begin(ctx, &DocumentBeginMetadata{
		At:         store.Time(time.Now().UTC()),
		DocumentID: id,
		Base:       base,
		Version:    nil,
		User:       store.UserFromContext(ctx),
	})
}

// ErrInvalidChange is a base error for changes which are malformed or fail validation.
var ErrInvalidChange = errors.Base("invalid change")

// AppendDocumentChange appends a change to an edit session at the given sequence number.
func (b *B) AppendDocumentChange(ctx context.Context, session identifier.Identifier, data json.RawMessage, seqNo int64) (int64, errors.E) {
	change, errE := document.ChangeUnmarshalJSON(data)
	if errE != nil {
		return 0, errors.WrapWith(errE, ErrInvalidChange)
	}

	beginMetadata, _, _, errE := b.coordinator.Get(ctx, session)
	if errE != nil {
		return 0, errE
	}

	changesetBase := slices.Clone(beginMetadata.Base)
	changesetBase = append(changesetBase, "SESSION", session.String())

	errE = change.Validate(changesetBase, seqNo)
	if errE != nil {
		return 0, errors.WrapWith(errE, ErrInvalidChange)
	}

	return b.coordinator.Append(ctx, session, data, &documentChangeMetadata{
		At:   store.Time(time.Now().UTC()),
		User: store.UserFromContext(ctx),
	}, &seqNo)
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

// EndEditDocument ends an edit session, committing or discarding its changes.
func (b *B) EndEditDocument(ctx context.Context, session identifier.Identifier, discard bool) errors.E {
	return b.coordinator.End(ctx, session, &documentEndMetadata{
		At:        store.Time(time.Now().UTC()),
		Discarded: discard,
		User:      store.UserFromContext(ctx),
	})
}

// GetEditDocumentSession returns the begin metadata of the edit session, a flag indicating
// whether the session has ended, and the complete metadata if the session has completed.
//
// The begin metadata's Version is nil for create sessions and non-nil for edit sessions,
// which lets callers distinguish the two without a separate flag.
func (b *B) GetEditDocumentSession(ctx context.Context, session identifier.Identifier) (*DocumentBeginMetadata, bool, *DocumentCompleteMetadata, errors.E) {
	beginMetadata, endMetadata, completeMetadata, errE := b.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, false, nil, errE
	}
	return beginMetadata, endMetadata != nil, completeMetadata, nil
}

// BeginUploadNew begins a chunked file upload session for a new file.
func (b *B) BeginUploadNew(ctx context.Context, base []string, size int64, mediaType, filename string) (identifier.Identifier, errors.E) {
	return b.files.BeginUploadNew(ctx, base, size, mediaType, filename)
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

// EndUpload finalizes a file upload session outside of a document edit session, assembling the uploaded chunks.
func (b *B) EndUpload(ctx context.Context, session identifier.Identifier) errors.E {
	return b.files.EndUpload(ctx, session, nil)
}

// EndEditDocumentUpload finalizes a file upload session inside of a document edit session, assembling the uploaded chunks.
func (b *B) EndEditDocumentUpload(ctx context.Context, session, documentSession identifier.Identifier) errors.E {
	_, endMetadata, completeMetadata, errE := b.coordinator.Get(ctx, documentSession)
	if errE != nil {
		return errE
	} else if endMetadata != nil {
		// We check this also inside completeStorageSessionTx (inside PrimaryCoordinator.ChangesetID),
		// but we check to return the error early if possible.
		return errors.WithStack(coordinator.ErrAlreadyEnded)
	} else if completeMetadata != nil {
		// We check this also inside completeStorageSessionTx (inside PrimaryCoordinator.ChangesetID),
		// but we check to return the error early if possible.
		return errors.WithStack(coordinator.ErrAlreadyCompleted)
	}

	return b.files.EndUpload(ctx, session, &documentSession)
}

// DiscardUpload discards a file upload session without storing the file.
func (b *B) DiscardUpload(ctx context.Context, session identifier.Identifier) errors.E {
	return b.files.DiscardUpload(ctx, session)
}

// GetUploadSession returns flag if file upload session has ended and the complete metadata if completed.
func (b *B) GetUploadSession(ctx context.Context, session identifier.Identifier) (bool, *storage.CompleteMetadata, errors.E) {
	_, endMetadata, completeMetadata, errE := b.files.Coordinator().Get(ctx, session)
	if errE != nil {
		return false, nil, errE
	}
	return endMetadata != nil, completeMetadata, errE
}

// CountFiles returns the number of stored files currently in storage,
// excluding files whose latest committed version has been deleted.
func (b *B) CountFiles(ctx context.Context) (int64, errors.E) {
	return b.files.Store().Count(ctx, false)
}

// GetFile returns an open handle on a stored file at the given version. The caller is responsible
// for closing the returned handle.
//
// It returns also file metadata, the version of the file (if requested version
// has 0 for revision, a file with the latest revision is returned and returned version
// contains this revision number), and parent changesets of the file at this version.
func (b *B) GetFile(
	ctx context.Context, id identifier.Identifier, version store.Version,
) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.FilePreHooks {
		errE := hook(ctx, id, &version)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	file, metadata, version, parentChangesets, errE := b.files.Get(ctx, id, version)
	for _, hook := range b.FilePostHooks {
		file, metadata, version, parentChangesets, errE = hook(ctx, file, metadata, version, parentChangesets, errE)
	}
	return file, metadata, version, parentChangesets, errE
}

// GetFileLatest returns an open handle on the latest version of a stored file. The caller is
// responsible for closing the returned handle.
//
// It returns also file metadata, the version of the file, and parent
// changesets of the file at this version.
func (b *B) GetFileLatest(ctx context.Context, id identifier.Identifier) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	for _, hook := range b.FilePreHooks {
		errE := hook(ctx, id, nil)
		if errE != nil {
			return nil, nil, store.Version{}, nil, errE
		}
	}
	file, metadata, version, parentChangesets, errE := b.files.GetLatest(ctx, id)
	for _, hook := range b.FilePostHooks {
		file, metadata, version, parentChangesets, errE = hook(ctx, file, metadata, version, parentChangesets, errE)
	}
	return file, metadata, version, parentChangesets, errE
}
