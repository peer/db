package base

import (
	"context"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/types"
)

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
//
// It is useful for bulk importing data where you do not care about metadata and history tracking.
func (b *B) InsertOrReplaceDocument(ctx context.Context, doc *document.D) errors.E {
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return errE
	}
	_, errE = b.documents.Insert(ctx, doc.ID, data, &DocumentMetadata{At: types.Time(time.Now().UTC())}, &types.NoMetadata{})
	return errE
}

// WaitUntilCaughtUp blocks until the base has indexed all currently committed documents.
//
// It is useful for waiting after a bulk import before searching.
func (b *B) WaitUntilCaughtUp(ctx context.Context) errors.E {
	return b.bridge.WaitUntilCaughtUp(ctx)
}
