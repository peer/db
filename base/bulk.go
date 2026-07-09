package base

import (
	"context"
	"io"
	"mime"
	"path/filepath"
	"slices"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceDocument(ctx context.Context, doc *document.D) errors.E {
	errE := doc.Validate()
	if errE != nil {
		return errE
	}

	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}

	metadata := &store.DocumentMetadata{
		At:    store.Time(time.Now().UTC()),
		Users: nil,
	}

	// Each doc.Id has to be unique, so each doc.Base is unique as well.
	changesetBase := slices.Clone(doc.Base)
	changesetBase = append(changesetBase, "CHANGESET", "FIRST")
	_, errE = b.documents.Insert(ctx, doc.ID, data, metadata, &store.CommitMetadata{
		Base: changesetBase,
		User: nil,
	})
	// If commit with ID from changesetBase already exists, this means that also the doc
	// with its ID already exist. So we replace the doc.
	if errors.Is(errE, store.ErrAlreadyCommitted) {
		_, _, version, _, errE := b.documents.GetLatest(ctx, doc.ID)
		if errE != nil {
			return errE
		}
		changesetBase := slices.Clone(doc.Base)
		changesetBase = append(changesetBase, "CHANGESET", "REPLACE", version.Changeset.String())
		// TODO: What to do once we have document melding and target document got melded into some other document?
		_, errE = b.documents.Replace(ctx, doc.ID, version.Changeset, data, metadata, &store.CommitMetadata{
			Base: changesetBase,
			User: nil,
		})
		return errE
	}
	return errE
}

// InsertOrReplaceFile inserts or replaces the file based on the ID computed from base.
//
// The contents are read from reader, which must be seekable: it is read to detect the media type
// (when not determined from the filename), then rewound and read again to hash and store the contents.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceFile(ctx context.Context, base []string, reader io.ReadSeeker, filename string) (identifier.Identifier, errors.E) {
	id := identifier.From(base...)

	mediaType := mime.TypeByExtension(filepath.Ext(filename))
	if mediaType == "" {
		// Unable to determine media type by extension. Detect it from the contents, then rewind so
		// the contents can be read again in full.
		mtype, err := mimetype.DetectReader(reader)
		if err != nil {
			return id, errors.WithStack(err)
		}
		mediaType = mtype.String()
		_, err = reader.Seek(0, io.SeekStart)
		if err != nil {
			return id, errors.WithStack(err)
		}
	}

	// The contents go to disk; the underlying store holds only the content hash referencing them.
	hash, etag, size, errE := b.files.WriteFile(reader)
	if errE != nil {
		return id, errE
	}

	metadata := &storage.FileMetadata{
		At:        store.Time(time.Now().UTC()),
		Base:      base,
		Size:      size,
		MediaType: mediaType,
		Filename:  filename,
		Etag:      etag,
		Users:     nil,
	}

	// Each base is unique.
	changesetBase := slices.Clone(base)
	changesetBase = append(changesetBase, "CHANGESET", "FIRST")
	_, errE = b.files.Store().Insert(ctx, id, hash, metadata, &store.CommitMetadata{
		Base: changesetBase,
		User: nil,
	})
	// If commit with ID from changesetBase already exists, this means that also the file
	// with its ID already exist. So we replace the file.
	if errors.Is(errE, store.ErrAlreadyCommitted) {
		_, _, version, _, errE := b.files.Store().GetLatest(ctx, id)
		if errE != nil {
			return id, errE
		}
		changesetBase := slices.Clone(base)
		changesetBase = append(changesetBase, "CHANGESET", "REPLACE", version.Changeset.String())
		_, errE = b.files.Store().Replace(ctx, id, version.Changeset, hash, metadata, &store.CommitMetadata{
			Base: changesetBase,
			User: nil,
		})
		return id, errE
	}
	return id, errE
}

// WaitUntilCaughtUp blocks until the base has indexed all currently committed documents.
//
// It is useful for waiting after a bulk import before searching.
//
// Optional count and size counters can be provided to track ES indexing progress.
// If provided, size is increased for the number of commits to process, and count is
// incremented as commits are indexed.
func (b *B) WaitUntilCaughtUp(ctx context.Context, count, size *x.Counter) errors.E {
	return b.bridge.WaitUntilCaughtUp(ctx, count, size)
}
