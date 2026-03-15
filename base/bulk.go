package base

import (
	"context"
	"mime"
	"path/filepath"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
)

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceDocument(ctx context.Context, doc *document.D) errors.E {
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}
	// TODO: Implement "or replace" part. Currently we just insert.
	_, errE = b.documents.Insert(ctx, doc.ID, data, &DocumentMetadata{
		At: store.Time(time.Now().UTC()),
	}, &store.NoMetadata{})
	return errE
}

// InsertOrReplaceFile inserts or replaces the file based on the ID.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceFile(ctx context.Context, id identifier.Identifier, data []byte, filename string) errors.E {
	mediaType := mime.TypeByExtension(filepath.Ext(filename))
	if mediaType == "" {
		// Unable to determine media type by extension. Try to detect it by content.
		mtype := mimetype.Detect(data)
		mediaType = mtype.String()
	}

	metadata := &storage.FileMetadata{
		At:        store.Time(time.Now().UTC()),
		Size:      int64(len(data)),
		MediaType: mediaType,
		Filename:  filename,
		Etag:      x.ComputeEtag(data),
	}

	// TODO: Implement "or replace" part. Currently we just insert.
	_, errE := b.files.Store().Insert(ctx, id, data, metadata, &store.NoMetadata{})
	return errE
}

// WaitUntilCaughtUp blocks until the base has indexed all currently committed documents.
//
// It is useful for waiting after a bulk import before searching.
func (b *B) WaitUntilCaughtUp(ctx context.Context) errors.E {
	return b.bridge.WaitUntilCaughtUp(ctx)
}
