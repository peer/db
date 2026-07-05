// Package storage provides file storage functionality for PeerDB.
//
// This is a low-level component.
package storage

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// storedFileMode is the permission mode applied to stored files. os.CreateTemp creates files as 0o600
// (owner only), so we relax them to 0o644 (adjusted by the process umask, the same way the OS applies it
// to the 0o755 storage directories) so that a process running as a different user than the writer can
// read the blobs, for example a containerized server reading files a bulk import wrote.
//
//nolint:gochecknoglobals,mnd
var storedFileMode = modeWithUmask(0o644)

// modeWithUmask returns mode with the current process umask applied, the same masking the OS performs
// when creating a file. It reads the umask by momentarily setting it to 0 and restoring it. This runs at
// package initialization, before the program creates files concurrently, so the transient change is safe.
func modeWithUmask(mode os.FileMode) os.FileMode {
	um := syscall.Umask(0)
	syscall.Umask(um)
	return mode &^ os.FileMode(um) //nolint:gosec
}

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
	// Hash is the lowercase hex SHA-256 of the file contents as computed by the client while uploading.
	// Completion compares it against the hash of the assembled file and fails the upload (it is not
	// stored) on a mismatch, so corruption between the client and the assembled file is detected.
	Hash string `json:"hash,omitempty"`
	// User is the user who ended the upload session (the committer). nil when
	// unauthenticated. Lands in CommitMetadata.User when the file is committed
	// standalone. NOT included in the Users union on FileMetadata.
	User *store.User `json:"user,omitempty"`
}

type completeData struct {
	// Hash is the content hash addressing the assembled file on disk.
	// It is stored in the underlying store in data column.
	Hash         string
	FileMetadata *FileMetadata
	EndMetadata  *endMetadata
	Chunks       int64
}

// CompleteMetadata contains metadata captured when file upload session completes.
type CompleteMetadata struct {
	Discarded bool `json:"discarded,omitempty"`
	Errored   bool `json:"errored,omitempty"`

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

type chunkPos struct {
	Start  int64
	Length int64
	Chunk  int64
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

	// Dir is the directory under which file contents are stored. Files are content-addressed:
	// the underlying store holds only a file's content hash while the contents themselves live on
	// disk under Dir, sharded into two levels of subdirectories by the first two characters of the
	// hash. It is required.
	Dir string

	// PrimaryCoordinator can be set to the primary session coordinator which allows one to
	// upload files into changesets managed by the primary session coordinator.
	PrimaryCoordinator PrimaryCoordinator

	store       *store.Store[string, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None]
	coordinator *coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata]
}

// Init initializes the Storage.
func (s *Storage) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internalStore.Listener, r *internalStore.River,
) errors.E {
	if s.store != nil {
		return errors.New("already initialized")
	}

	if s.Dir == "" {
		return errors.New("storage directory not configured")
	}

	storageStore := &store.Store[string, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None]{
		Schema:       s.Schema,
		Prefix:       s.Prefix,
		DataType:     "text",
		MetadataType: "jsonb",
		PatchType:    "",
	}
	errE := storageStore.Init(ctx, dbpool, listener)
	if errE != nil {
		return errE
	}

	storageCoordinator := &coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata]{
		Prefix:                   s.Prefix,
		DataType:                 "bytea",
		MetadataType:             "jsonb",
		CompleteSession:          s.completeStorageSession,
		CompleteSessionTx:        s.completeStorageSessionTx,
		CompleteSessionOnErrorTx: s.completeSessionOnErrorTx,
		CompleteSessionTimeout:   completeSessionTimeout,
	}
	// We do not use Appended and Ended channels here so we pass nil for listener.
	errE = storageCoordinator.Init(ctx, dbpool, nil, r)
	if errE != nil {
		return errE
	}

	s.store = storageStore
	s.coordinator = storageCoordinator

	return nil
}

// Store returns the underlying store.Store instance.
func (s *Storage) Store() *store.Store[string, *FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None] {
	return s.store
}

// Coordinator returns the underlying coordinator.Coordinator instance.
func (s *Storage) Coordinator() *coordinator.Coordinator[[]byte, *chunkMetadata, *beginMetadata, *endMetadata, *completeData, *CompleteMetadata] {
	return s.coordinator
}

// filePath returns the on-disk path of the file contents addressed by the given content hash.
//
// Files are content-addressed: the hash is the file name, placed under two levels of subdirectories
// named after its first and second characters so that no single directory holds too many files.
func (s *Storage) filePath(hash string) string {
	return filepath.Join(s.Dir, hash[0:1], hash[1:2], hash)
}

// etagToHash converts a strong ETag (a quoted, base64url-encoded SHA-256 digest) into the lowercase
// hex encoding of the same digest. The hex form is what addresses the file on disk and is stored as
// the file data, because hex is safe on case-insensitive filesystems (such as Windows) while base64 is
// not (it would let two distinct digests collide by case).
func etagToHash(etag string) (string, errors.E) {
	sum, err := base64.RawURLEncoding.DecodeString(strings.Trim(etag, `"`))
	if err != nil {
		return "", errors.WithStack(err)
	}
	return hex.EncodeToString(sum), nil
}

// WriteFile streams the contents read from reader into the storage directory and returns the content
// hash that addresses them (and is stored as the file data in the underlying store), the strong ETag
// used for HTTP responses, and the number of bytes written.
//
// reader must be seekable: the contents are read once to compute the hash (which determines the final
// path) and, if not already stored, read again from the start to write them, so they are never fully
// buffered in memory.
//
// Storage is content-addressed, so writing contents that are already stored (same hash, hence the
// same bytes) is idempotent: if a file is already present at its final path it is skipped. The hash
// is later resolved back into the contents by Get, GetLatest, and GetFromChangeset.
//
// The contents are written to a uniquely named temporary file, fsynced, and then atomically renamed
// to the final path, and finally the directory is fsynced. This way a file present at the final path
// is always complete, even after an interrupted write or a crash, so the skip above is safe. A unique
// temporary name lets concurrent writers of the same contents proceed without clashing; whichever
// rename lands last wins and the contents are equal.
func (s *Storage) WriteFile(reader io.ReadSeeker) (string, string, int64, errors.E) {
	// First pass: hash the contents and measure their size by streaming, without buffering them.
	etag, size, errE := x.ComputeEtagReader(reader)
	if errE != nil {
		return "", "", 0, errE
	}
	// The file is content-addressed by the hex digest derived from the strong ETag.
	hash, errE := etagToHash(etag)
	if errE != nil {
		return "", "", 0, errE
	}
	path := s.filePath(hash)

	_, err := os.Stat(path)
	if err == nil {
		// The file is already stored and is therefore complete; there is nothing to do.
		return hash, etag, size, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return "", "", 0, errE
	}

	// Second pass: rewind to the start so we can stream the contents into the temporary file.
	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return "", "", 0, errE
	}

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0o755) //nolint:gosec,mnd
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return "", "", 0, errE
	}

	tmp, err := os.CreateTemp(dir, hash+".*.new")
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return "", "", 0, errE
	}
	tmpPath := tmp.Name()
	// On any error path the temporary file is closed and removed. After a successful rename it no
	// longer exists under tmpPath, so the Remove is then a no-op, as is the second Close.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	// os.CreateTemp creates the file 0o600. Relax it so a reader running as a different user than the writer can read the blob.
	err = tmp.Chmod(storedFileMode)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmpPath
		return "", "", 0, errE
	}

	n, err := io.Copy(tmp, reader)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmpPath
		return "", "", n, errE
	}

	if n != size {
		errE := errors.New("file size mismatch")
		errors.Details(errE)["expected"] = size
		errors.Details(errE)["got"] = n
		errors.Details(errE)["path"] = tmpPath
		return "", "", n, errE
	}

	errE = s.finalizeTempFile(tmp, hash)
	if errE != nil {
		return "", "", n, errE
	}

	return hash, etag, n, nil
}

// finalizeTempFile durably places the open temporary file, whose contents hash to hash, at its
// content-addressed final path. It fsyncs the contents, then either discards the temporary file when a
// file is already stored at the final path (a concurrent writer placed identical contents) or
// atomically renames it into place and fsyncs the directory. It closes the temporary file; the
// caller's own removal of the temporary path becomes a no-op after a successful rename.
func (s *Storage) finalizeTempFile(tmp *os.File, hash string) errors.E {
	tmpPath := tmp.Name()

	// We fsync the contents so they are durably on disk before the rename publishes the file at its
	// final path; otherwise a crash could leave a visible but incomplete file there.
	err := tmp.Sync()
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmpPath
		return errE
	}

	err = tmp.Close()
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmpPath
		return errE
	}

	path := s.filePath(hash)
	_, err = os.Stat(path)
	if err == nil {
		// The file is already stored and is therefore complete; there is nothing to do.
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return errE
	}

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0o755) //nolint:gosec,mnd
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return errE
	}

	err = os.Rename(tmpPath, path)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return errE
	}

	// We fsync the directory so the rename itself is durable across a crash.
	return fsyncDir(dir)
}

// fsyncDir flushes a directory's entries (such as a rename just performed in it) to disk so they
// survive a crash.
func fsyncDir(dir string) errors.E {
	d, err := os.Open(dir) //nolint:gosec
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = dir
		return errE
	}
	defer func() { _ = d.Close() }()

	err = d.Sync()
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = dir
		return errE
	}
	return nil
}

// openFile opens the file addressed by the given content hash in the storage directory. The caller
// is responsible for closing the returned handle.
func (s *Storage) openFile(hash string) (*os.File, errors.E) {
	path := s.filePath(hash)
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = path
		return nil, errE
	}
	return file, nil
}

// resolveFile turns a read from the underlying store, where the stored data is the file's content
// hash, into an open handle on the file contents in the storage directory. The caller is responsible
// for closing the returned handle.
//
// Store errors (including ErrValueNotFound and ErrValueDeleted) are passed through unchanged with a
// nil handle; for a deleted version there is no hash to resolve, but the metadata, version, and
// parent changesets the store returned remain valid.
func (s *Storage) resolveFile(
	hash string, metadata *FileMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
) (io.ReadSeekCloser, *FileMetadata, store.Version, []store.Version, errors.E) {
	if errE != nil {
		return nil, metadata, version, parentChangesets, errE
	}
	if hash == "" {
		errE = errors.New("stored file hash is empty")
		errors.Details(errE)["version"] = version.String()
		return nil, metadata, version, parentChangesets, errE
	}
	file, errE := s.openFile(hash)
	if errE != nil {
		return nil, metadata, version, parentChangesets, errE
	}
	return file, metadata, version, parentChangesets, nil
}

// attachID records the file id on a non-nil error's details so that an error surfaced while resolving
// a file can be traced back to the file it concerns.
// It returns errE unchanged for convenient inline use at the call sites.
func attachID(id identifier.Identifier, errE errors.E) errors.E {
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
	}
	return errE
}

// Get returns an open handle on the contents of a stored file at the given version, resolving the
// hash stored in the underlying store into an open handle on the file in the storage directory. The
// caller is responsible for closing the returned handle.
//
// It returns also file metadata, the version of the file (if the requested version has 0 for
// revision, the file with the latest revision is returned and the returned version contains this
// revision number), and parent changesets of the file at this version.
func (s *Storage) Get(
	ctx context.Context, id identifier.Identifier, version store.Version,
) (io.ReadSeekCloser, *FileMetadata, store.Version, []store.Version, errors.E) {
	file, metadata, version, parentChangesets, errE := s.resolveFile(s.store.Get(ctx, id, version))
	return file, metadata, version, parentChangesets, attachID(id, errE)
}

// GetLatest returns an open handle on the contents of the latest version of a stored file, resolving
// the hash stored in the underlying store into an open handle on the file in the storage directory.
// The caller is responsible for closing the returned handle.
//
// It returns also file metadata, the version of the file, and parent changesets of the file at
// this version.
func (s *Storage) GetLatest(
	ctx context.Context, id identifier.Identifier,
) (io.ReadSeekCloser, *FileMetadata, store.Version, []store.Version, errors.E) {
	file, metadata, version, parentChangesets, errE := s.resolveFile(s.store.GetLatest(ctx, id))
	return file, metadata, version, parentChangesets, attachID(id, errE)
}

// GetFromChangeset returns an open handle on the contents of a stored file at the given revision in
// the changeset, resolving the hash stored in the underlying store into an open handle on the file in
// the storage directory. The caller is responsible for closing the returned handle.
//
// If revision is 0, the latest revision is returned. If the file has been deleted in the changeset,
// it returns ErrValueDeleted, but other returned values are valid as well.
func (s *Storage) GetFromChangeset(
	ctx context.Context, changesetID, id identifier.Identifier, revision int64,
) (io.ReadSeekCloser, *FileMetadata, store.Version, []store.Version, errors.E) {
	changeset, errE := s.store.Changeset(ctx, changesetID)
	if errE != nil {
		return nil, nil, store.Version{}, nil, attachID(id, errE)
	}
	file, metadata, version, parentChangesets, errE := s.resolveFile(changeset.Get(ctx, id, revision))
	return file, metadata, version, parentChangesets, attachID(id, errE)
}

func (s *Storage) completeStorageSession(ctx context.Context, session identifier.Identifier) (*completeData, errors.E) {
	beginMetadata, endMetadata, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	if endMetadata.Discarded {
		return &completeData{
			Hash:         "",
			FileMetadata: nil,
			EndMetadata:  endMetadata,
			Chunks:       0,
		}, nil
	}

	lastChunk, errE := s.LastChunk(ctx, session)
	if errE != nil {
		return nil, errE
	}

	// Chunks are numbered sequentially without gaps starting at 1. The list goes from the
	// newest chunk to the oldest, which assembleChunks relies on when resolving chunks
	// with the same start.
	chunksList := make([]int64, 0, lastChunk)
	for c := lastChunk; c >= 1; c-- {
		chunksList = append(chunksList, c)
	}

	// We assemble the uploaded chunks into a temporary file on disk and then store it.
	// The temporary file is created in the storage directory root because its final,
	// content-addressed location is only known once the assembled contents have been hashed.
	err := os.MkdirAll(s.Dir, 0o755) //nolint:gosec,mnd
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = s.Dir
		return nil, errE
	}
	tmp, err := os.CreateTemp(s.Dir, "assemble-"+session.String()+".*.tmp")
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = s.Dir
		return nil, errE
	}
	// On any error path the temporary file is closed and removed. After finalizeTempFile renames it
	// into place the Remove is a no-op, as is the second Close.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	// os.CreateTemp creates the file 0o600. Relax it so a reader running as a different user than the writer can read the blob.
	err = tmp.Chmod(storedFileMode)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmp.Name()
		return nil, errE
	}

	users, errE := s.assembleChunks(ctx, session, beginMetadata, chunksList, tmp)
	if errE != nil {
		return nil, errE
	}

	// We hash the assembled file and place it at its content-addressed final path before the changeset
	// is committed (in completeStorageSessionTx), so the file is on disk by the time the hash
	// referencing it is stored. The hash is what gets stored as the file data in the underlying store.
	_, err = tmp.Seek(0, io.SeekStart)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = tmp.Name()
		return nil, errE
	}
	etag, _, errE := x.ComputeEtagReader(tmp)
	if errE != nil {
		errors.Details(errE)["path"] = tmp.Name()
		return nil, errE
	}
	hash, errE := etagToHash(etag)
	if errE != nil {
		return nil, errE
	}

	// The assembled file must hash to the value the client computed over the contents it uploaded. On a
	// mismatch the contents were corrupted between the client and the assembled file, so we do not store
	// the file (the temporary file is discarded) and fail the completion. The assembled file hashes the
	// same on every retry, so this is permanent: we wrap it with ErrInvalidSessionData so the completion
	// job is cancelled instead of retried.
	if endMetadata.Hash != hash {
		errE := errors.New("uploaded file hash does not match the client-provided hash")
		errors.Details(errE)["expected"] = endMetadata.Hash
		errors.Details(errE)["got"] = hash
		return nil, errors.WrapWith(errE, coordinator.ErrInvalidSessionData)
	}

	errE = s.finalizeTempFile(tmp, hash)
	if errE != nil {
		return nil, errE
	}

	base := slices.Clone(beginMetadata.Base)
	base = append(base, "STORAGE", session.String())

	metadata := &FileMetadata{
		At:        endMetadata.At,
		Base:      base,
		Size:      beginMetadata.Size,
		MediaType: beginMetadata.MediaType,
		Filename:  beginMetadata.Filename,
		Etag:      etag,
		Users:     users,
	}

	return &completeData{
		Hash:         hash,
		FileMetadata: metadata,
		EndMetadata:  endMetadata,
		Chunks:       int64(len(chunksList)),
	}, nil
}

// assembleChunks writes the uploaded chunks for the session into dst at their respective offsets,
// validating that they cover the whole file without gaps (matching validateChunks), and returns the
// deduplicated set of users who contributed to the upload (the begin user plus every chunk uploader).
//
// Chunk contents are read one chunk at a time and written at the chunk's offset, so the whole file is
// never held in memory.
func (s *Storage) assembleChunks(
	ctx context.Context, session identifier.Identifier, beginMetadata *beginMetadata, chunksList []int64, dst *os.File,
) ([]store.User, errors.E) {
	// First we read every chunk's position and uploader (but not its contents) so we can order the
	// chunks by start and collect the contributing users.
	chunks := make([]chunkPos, 0, len(chunksList))
	chunkUsers := make([]*store.User, 0, len(chunksList)+1)
	// chunkUsers collects the begin user and every per-chunk user. The end user is intentionally
	// excluded (it belongs on CommitMetadata.User). internalStore.SortedUniqueUsers drops nils, so
	// unauthenticated participants are skipped.
	chunkUsers = append(chunkUsers, beginMetadata.User)
	for _, c := range chunksList {
		metadata, errE := s.coordinator.GetMetadata(ctx, session, c)
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return nil, errE
		}
		chunks = append(chunks, chunkPos{Start: metadata.Start, Length: metadata.Length, Chunk: c})
		chunkUsers = append(chunkUsers, metadata.User)
	}
	// chunksList is sorted from newest to the oldest chunk and we use a stable sort here, so the
	// result is that if there are multiple chunks at the same start, newer will be used first.
	// For chunks with different starts that partially overlap, the chunk with the larger start
	// is processed last and overwrites the overlapping region, regardless of upload order.
	slices.SortStableFunc(chunks, func(a, b chunkPos) int {
		return cmp.Compare(a.Start, b.Start)
	})

	// This should match implementation in validateChunks.
	size := int64(0)
	for _, c := range chunks {
		if c.Start > size {
			errE := errors.Errorf("%w: gap between chunks", ErrEndNotPossible)
			errors.Details(errE)["end"] = size
			errors.Details(errE)["start"] = c.Start
			errors.Details(errE)["chunk"] = c.Chunk
			return nil, errE
		}
		end := c.Start + c.Length
		if end > beginMetadata.Size {
			// This should have already been checked in UploadChunk so it is not an ErrEndNotPossible.
			errE := errors.New("chunk larger than file")
			errors.Details(errE)["start"] = c.Start
			errors.Details(errE)["end"] = end
			errors.Details(errE)["size"] = beginMetadata.Size
			errors.Details(errE)["chunk"] = c.Chunk
			return nil, errE
		}
		if end <= size {
			// We already have this data, so we do not need to read or write the chunk.
			continue
		}
		data, _, errE := s.coordinator.GetData(ctx, session, c.Chunk)
		if errE != nil {
			errors.Details(errE)["chunk"] = c.Chunk
			return nil, errE
		}
		_, err := dst.WriteAt(data, c.Start)
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["chunk"] = c.Chunk
			errors.Details(errE)["path"] = dst.Name()
			return nil, errE
		}
		size = end
	}

	if size < beginMetadata.Size {
		errE := errors.Errorf("%w: chunks smaller than file", ErrEndNotPossible)
		errors.Details(errE)["chunks"] = size
		errors.Details(errE)["size"] = beginMetadata.Size
		return nil, errE
	}

	return internalStore.SortedUniqueUsers(chunkUsers), nil
}

func (s *Storage) completeStorageSessionTx(ctx context.Context, _ pgx.Tx, session identifier.Identifier, data *completeData) (*CompleteMetadata, errors.E) {
	if data.EndMetadata.Discarded {
		return &CompleteMetadata{
			Discarded: true,
			Errored:   false,
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
				Errored:   false,
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
		// The contents are already on disk (written in completeStorageSession); we store only the hash.
		_, errE = changeset.Insert(ctx, id, data.Hash, data.FileMetadata)
		if errE != nil {
			return nil, errE
		}
	} else {
		// Changeset base was not provided, so we construct one from the file base.
		// That is the same construction we use for changeset base for ending a document session.
		changesetBase := slices.Clone(data.FileMetadata.Base)
		changesetBase = append(changesetBase, "SESSION", session.String())

		// We do not have to use the "tx" parameter because we access the transaction through ctx.
		// The contents are already on disk (written in completeStorageSession); we store only the hash.
		_, errE := s.store.Insert(ctx, id, data.Hash, data.FileMetadata, &store.CommitMetadata{
			Base: changesetBase,
			User: data.EndMetadata.User,
		})
		if errE != nil {
			return nil, errE
		}
	}

	return &CompleteMetadata{
		Discarded: false,
		Errored:   false,
		ID:        &id,
		Chunks:    data.Chunks,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}

func (s *Storage) completeSessionOnErrorTx(ctx context.Context, _ pgx.Tx, session identifier.Identifier, completeErr error) (*CompleteMetadata, errors.E) {
	_, endMetadata, _, errE := s.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	return &CompleteMetadata{
		Discarded: completeErr == nil,
		Errored:   completeErr != nil,
		ID:        nil,
		Chunks:    0,
		Time:      time.Since(time.Time(endMetadata.At)).Milliseconds(),
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

// LastChunk returns the sequence number of the latest chunk uploaded to the session, 0 when
// there are none. Chunks are numbered sequentially without gaps starting at 1.
func (s *Storage) LastChunk(ctx context.Context, session identifier.Identifier) (int64, errors.E) {
	return s.coordinator.LastOperation(ctx, session)
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
// hash is the lowercase hex SHA-256 of the file contents computed by the client; the assembled file's
// hash is checked against it at completion and the upload fails on a mismatch.
//
// It returns the ID of the file.
func (s *Storage) EndUpload(ctx context.Context, session identifier.Identifier, primarySession *identifier.Identifier, hash string) errors.E {
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
		Hash:           hash,
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

	lastChunk, errE := s.LastChunk(ctx, session)
	if errE != nil {
		return errE
	}

	// Chunks are numbered sequentially without gaps starting at 1. They are iterated from
	// the newest to the oldest, matching assembleChunks.
	chunks := make([]chunkPos, 0, lastChunk)
	for c := lastChunk; c >= 1; c-- {
		start, length, errE := s.GetChunk(ctx, session, c)
		if errE != nil {
			errors.Details(errE)["chunk"] = c
			return errE
		}
		chunks = append(chunks, chunkPos{
			Start:  start,
			Length: length,
			Chunk:  c,
		})
	}
	// chunksList is sorted from newest to the oldest chunk and we use a stable sort here, so the
	// result is that if there are multiple chunks at the same start, newer will be used first.
	// For chunks with different starts that partially overlap, the chunk with the larger start
	// is processed last and overwrites the overlapping region, regardless of upload order.
	slices.SortStableFunc(chunks, func(a, b chunkPos) int {
		return cmp.Compare(a.Start, b.Start)
	})

	size := int64(0)

	// This should match implementation in completeStorageSession.
	for _, p := range chunks {
		if p.Start > size {
			errE = errors.Errorf("%w: gap between chunks", ErrEndNotPossible)
			errors.Details(errE)["end"] = size
			errors.Details(errE)["start"] = p.Start
			errors.Details(errE)["chunk"] = p.Chunk
			return errE
		}
		end := p.Start + p.Length
		if end > beginMetadata.Size {
			// This should have already been checked in UploadChunk so it is not an ErrEndNotPossible.
			errE = errors.New("chunk larger than file")
			errors.Details(errE)["start"] = p.Start
			errors.Details(errE)["end"] = end
			errors.Details(errE)["size"] = beginMetadata.Size
			errors.Details(errE)["chunk"] = p.Chunk
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
		Hash:           "",
		User:           store.UserFromContext(ctx),
	}
	return s.coordinator.End(ctx, session, metadata)
}
