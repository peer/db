package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

func initDatabase(t *testing.T) (
	context.Context,
	*storage.Storage,
	*testutils.LockableSlice[store.CommittedChangesets[
		string, *storage.FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None,
	]],
) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"

	ctx = internalStore.WithFallbackDBContext(ctx, schema, "tests")

	// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
	// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
	dbCtx := internalStore.WithMaxDBPoolConnections(context.WithoutCancel(ctx), internalStore.TestMaxDBPoolConnections)
	dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(dbCtx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpoolCleanup)

	errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internalStore.NewListener(dbpool)

	r, errE := internalStore.NewRiver(ctx, logger, nil, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &storage.Storage{
		Schema:             schema,
		Prefix:             prefix,
		Dir:                t.TempDir(),
		PrimaryCoordinator: nil,
	}

	errE = s.Init(ctx, dbpool, listener, r)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = r.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-r.Client.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	channelContents := new(testutils.LockableSlice[store.CommittedChangesets[
		string, *storage.FileMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, store.None,
	]])

	go func() {
		for {
			ch, _ := s.Store().Committed.Get(ctx)
			select {
			case co, ok := <-ch:
				if ok {
					channelContents.Append(co)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ctx, s, channelContents
}

// readAndClose reads all contents from the handle and closes it.
func readAndClose(t *testing.T, file io.ReadSeekCloser) []byte {
	t.Helper()
	contents, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	return contents
}

// hashOf returns the lowercase hex SHA-256 of data, the form a client provides to EndUpload.
func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestHappyPath(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase(t)

	fileBase := []string{"test", "happypath"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 10, "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("bar"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("qrxzy"), 5)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 2)
	require.NoError(t, errE, "% -+#.1v", errE)

	chunks, errE := s.ListChunks(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{4, 3, 2, 1}, chunks)

	start, length, errE := s.GetChunk(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(3), length)

	start, length, errE = s.GetChunk(ctx, session, 2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(3), length)

	start, length, errE = s.GetChunk(ctx, session, 3)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(5), start)
	assert.Equal(t, int64(5), length)

	start, length, errE = s.GetChunk(ctx, session, 4)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), start)
	assert.Equal(t, int64(3), length)

	errE = s.EndUpload(ctx, session, nil, hashOf([]byte("bafooqrxzy")))
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s.Store())
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			if assert.Len(t, committed.Changesets, 1) {
				changes, errE := committed.Changesets[0].Changes(ctx, nil)
				if assert.NoError(t, errE, "% -+#.1v", errE) {
					if assert.Len(t, changes, 1) {
						// The file ID is derived from base + "STORAGE" + session.
						expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
						expectedFileID := identifier.From(expectedBase...)
						assert.Equal(t, expectedFileID, changes[0].ID)
					}
				}
			}
		}
	}

	// The file ID is derived from base + "STORAGE" + session.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
	expectedFileID := identifier.From(expectedBase...)

	// The on-disk file name and the stored data are the lowercase hex SHA-256 of the contents, while
	// the strong ETag keeps the base64url encoding (and quotes) for HTTP responses.
	const expectedHash = "a53a0071cc1c713b7d01b50733955021e17b42c816d03bf929af9e652e4edb66"
	const expectedEtag = `"pToAccwccTt9AbUHM5VQIeF7QsgW0Dv5Ka-eZS5O22Y"`

	// The underlying store now holds only the file's content hash (no quotes), not its contents.
	storedData, metadata, _, _, errE := s.Store().GetLatest(ctx, expectedFileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expectedHash, storedData)

	// Storage.GetLatest resolves the stored hash into an open handle on the contents on disk.
	file, _, _, _, errE := s.GetLatest(ctx, expectedFileID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("bafooqrxzy"), readAndClose(t, file))

	assert.False(t, time.Time(metadata.At).IsZero())
	assert.Equal(t, int64(10), metadata.Size)
	assert.Equal(t, "text/plain", metadata.MediaType)
	assert.Equal(t, "test.txt", metadata.Filename)
	// The strong ETag in metadata keeps its quotes for HTTP responses.
	assert.Equal(t, expectedEtag, metadata.Etag)

	// The contents live on disk under dir/a/b/hash, addressed by the bare content hash.
	onDisk, err := os.ReadFile(filepath.Join(s.Dir, expectedHash[0:1], expectedHash[1:2], expectedHash))
	require.NoError(t, err)
	assert.Equal(t, []byte("bafooqrxzy"), onDisk)

	// Get at the resolved version returns an open handle on the same contents.
	_, _, version, _, errE := s.Store().GetLatest(ctx, expectedFileID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	fileAtVersion, _, _, _, errE := s.Get(ctx, expectedFileID, version) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("bafooqrxzy"), readAndClose(t, fileAtVersion))

	// Verify file metadata Base is recorded and file ID is derivable from it.
	assert.Equal(t, expectedBase, metadata.Base)
	assert.Equal(t, expectedFileID, identifier.From(metadata.Base...))

	// Verify changeset ID is derivable from its base.
	changesetBase := append(append([]string{}, expectedBase...), "SESSION", session.String())
	assert.Equal(t, identifier.From(changesetBase...), version.Changeset)

	// Storage now holds exactly the one stored file.
	count, errE := s.Store().Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase(t)

	fileBase := []string{"test", "errors"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 10, "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("bar"), 5)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session, nil, "")
	assert.ErrorContains(t, errE, "gap between chunks")

	errE = s.UploadChunk(ctx, session, []byte("zy"), 3)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session, nil, "")
	assert.ErrorContains(t, errE, "chunks smaller than file")

	errE = s.UploadChunk(ctx, session, []byte("large"), 8)
	assert.ErrorContains(t, errE, "chunk larger than file")

	errE = s.DiscardUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No upload completed: storage remains empty.
	count, errE := s.Store().Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(0), count)
	}
}

// TestInitRequiresDir verifies that initializing storage without a directory configured fails
// before any database access.
func TestInitRequiresDir(t *testing.T) {
	t.Parallel()

	s := &storage.Storage{ //nolint:exhaustruct
		Schema: "test",
		Prefix: "test_",
	}
	errE := s.Init(t.Context(), nil, nil, nil)
	assert.ErrorContains(t, errE, "storage directory not configured")
}

// TestWriteFileAtomicAndIdempotent verifies that WriteFile writes the contents atomically, leaving no
// temporary file behind, and skips rewriting contents that are already stored at their final path.
//
// WriteFile only touches the storage directory, so this exercises it without a database.
func TestWriteFileAtomicAndIdempotent(t *testing.T) {
	t.Parallel()

	s := &storage.Storage{Dir: t.TempDir()} //nolint:exhaustruct

	hash, etag, size, errE := s.WriteFile(strings.NewReader("hello world"))
	require.NoError(t, errE, "% -+#.1v", errE)
	// The hash is the lowercase hex digest; the etag keeps the base64url encoding (and quotes).
	assert.Equal(t, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", hash)
	assert.Equal(t, `"uU0nuZNNPgilLlLX2n2r-sSE7-N6U4DukIj3rOLvzek"`, etag)
	assert.Equal(t, int64(len("hello world")), size)

	leafDir := filepath.Join(s.Dir, hash[0:1], hash[1:2])
	path := filepath.Join(leafDir, hash)

	// The contents are at the final path and no temporary file is left behind.
	onDisk, err := os.ReadFile(path) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), onDisk)
	entries, err := os.ReadDir(leafDir)
	require.NoError(t, err)
	if assert.Len(t, entries, 1) {
		assert.Equal(t, hash, entries[0].Name())
	}

	// A file already present at the final path is trusted as complete and not rewritten. We tamper
	// with it and verify that a second WriteFile of the same contents leaves the tampered bytes in place.
	require.NoError(t, os.WriteFile(path, []byte("tampered"), 0o644)) //nolint:gosec
	hash2, etag2, size2, errE := s.WriteFile(strings.NewReader("hello world"))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, hash, hash2)
	assert.Equal(t, etag, etag2)
	assert.Equal(t, size, size2)
	onDisk, err = os.ReadFile(path) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, []byte("tampered"), onDisk)
}

// TestContentAddressedDeduplication verifies that two distinct files with identical contents are
// stored once on disk: both resolve to the same bytes and share a single content-addressed file.
func TestContentAddressedDeduplication(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase(t)

	upload := func(base []string) identifier.Identifier {
		t.Helper()
		session, errE := s.BeginUploadNew(ctx, base, 5, "text/plain", "f.txt")
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = s.UploadChunk(ctx, session, []byte("hello"), 0)
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = s.EndUpload(ctx, session, nil, helloHash)
		require.NoError(t, errE, "% -+#.1v", errE)
		return identifier.From(append(append([]string{}, base...), "STORAGE", session.String())...)
	}

	id1 := upload([]string{"dedup", "one"})
	id2 := upload([]string{"dedup", "two"})

	require.Eventually(t, func() bool { return channelContents.Len() >= 2 }, 5*time.Second, 10*time.Millisecond)

	file1, meta1, _, _, errE := s.GetLatest(ctx, id1)
	require.NoError(t, errE, "% -+#.1v", errE)
	data1 := readAndClose(t, file1)
	file2, meta2, _, _, errE := s.GetLatest(ctx, id2)
	require.NoError(t, errE, "% -+#.1v", errE)
	data2 := readAndClose(t, file2)

	// Two distinct files with the same contents resolve to the same bytes and share one etag.
	assert.NotEqual(t, id1, id2)
	assert.Equal(t, []byte("hello"), data1)
	assert.Equal(t, []byte("hello"), data2)
	assert.Equal(t, meta1.Etag, meta2.Etag)

	// The underlying store holds the bare lowercase-hex content hash, distinct from the base64url etag.
	storedData, _, _, _, errE := s.Store().GetLatest(ctx, id1) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, helloHash, storedData)

	// They are backed by a single file on disk addressed by the shared content hash.
	onDisk, err := os.ReadFile(filepath.Join(s.Dir, helloHash[0:1], helloHash[1:2], helloHash))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), onDisk)
}

// helloHash is the lowercase hex SHA-256 of "hello".
const helloHash = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

// TestEndUploadHashMismatch verifies that when the client-provided hash does not match the assembled
// file, completion fails the hash check permanently: the file is not stored, but the session is still
// completed (its uploaded chunks are deleted) and marked as errored.
func TestEndUploadHashMismatch(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase(t)

	fileBase := []string{"test", "mismatch"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 5, "text/plain", "f.txt")
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = s.UploadChunk(ctx, session, []byte("hello"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End with a hash that does not match the uploaded contents.
	errE = s.EndUpload(ctx, session, nil, "0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, errE, "% -+#.1v", errE)

	// The completion job fails the hash check, and the on-error completion still completes the session,
	// deleting its chunks and marking it errored.
	var cm *storage.CompleteMetadata
	require.Eventually(t, func() bool {
		_, _, cm, errE = s.Coordinator().Get(ctx, session)
		return errE == nil && cm != nil
	}, 5*time.Second, 10*time.Millisecond)

	assert.True(t, cm.Errored)
	assert.False(t, cm.Discarded)
	assert.Nil(t, cm.ID)

	// The uploaded chunks were deleted when the session completed.
	_, errE = s.ListChunks(ctx, session)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	// The file was not stored.
	count, errE := s.Store().Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), count)
}

// TestEndUploadHashMatch verifies that a correct client-provided hash lets the upload complete and the
// file is stored.
func TestEndUploadHashMatch(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase(t)

	fileBase := []string{"test", "match"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 5, "text/plain", "f.txt")
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = s.UploadChunk(ctx, session, []byte("hello"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// End with the correct lowercase hex SHA-256 of the uploaded contents.
	errE = s.EndUpload(ctx, session, nil, helloHash)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)

	_, _, cm, errE := s.Coordinator().Get(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, cm)
	assert.NotNil(t, cm.ID)

	// The file is stored and resolves to the uploaded contents.
	expectedID := identifier.From(append(append([]string{}, fileBase...), "STORAGE", session.String())...)
	file, _, _, _, errE := s.GetLatest(ctx, expectedID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("hello"), readAndClose(t, file))
}
