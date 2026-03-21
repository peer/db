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
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceDocument(ctx context.Context, doc *document.D) errors.E {
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}
	newMetadata := &internal.DocumentMetadata{
		At:               internal.Time(time.Now().UTC()),
		InverseRelations: nil,
	}
	_, errE = b.documents.Insert(ctx, doc.ID, data, newMetadata, &internal.NoMetadata{})
	if errors.Is(errE, store.ErrConflict) {
		_, oldMetadata, version, _, errE := b.documents.GetLatest(ctx, doc.ID)
		if errE != nil {
			return errE
		}
		newMetadata.CarryOver(oldMetadata)
		// TODO: What to do once we have document melding and target document got melded into some other document?
		_, errE = b.documents.Replace(ctx, doc.ID, version.Changeset, data, newMetadata, &internal.NoMetadata{})
		return errE
	}
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
		At:        internal.Time(time.Now().UTC()),
		Size:      int64(len(data)),
		MediaType: mediaType,
		Filename:  filename,
		Etag:      x.ComputeEtag(data),
	}

	_, errE := b.files.Store().Insert(ctx, id, data, metadata, &internal.NoMetadata{})
	if errors.Is(errE, store.ErrConflict) {
		_, _, version, _, errE := b.files.Store().GetLatest(ctx, id)
		if errE != nil {
			return errE
		}
		_, errE = b.files.Store().Replace(ctx, id, version.Changeset, data, metadata, &internal.NoMetadata{})
		return errE
	}
	return errE
}

// WaitUntilCaughtUp blocks until the base has indexed all currently committed documents.
//
// It is useful for waiting after a bulk import before searching.
func (b *B) WaitUntilCaughtUp(ctx context.Context) errors.E {
	return b.bridge.WaitUntilCaughtUp(ctx)
}
