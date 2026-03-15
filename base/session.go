package base

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// DocumentBeginMetadata contains metadata captured at the beginning of document edit session.
type DocumentBeginMetadata struct {
	At      internal.Time         `json:"at"`
	ID      identifier.Identifier `json:"id"`
	Version store.Version         `json:"version"`
}

// documentEndMetadata contains metadata captured at the end of document edit session.
type documentEndMetadata struct {
	At        internal.Time `json:"at"`
	Discarded bool          `json:"discarded,omitempty"`
}

// documentCompleteData contains JSON serialized document with metadata to be
// passed between CompleteSession and CompleteSessionTx.
type documentCompleteData struct {
	BeginMetadata *DocumentBeginMetadata
	EndMetadata   *documentEndMetadata
	Changes       document.Changes
	Doc           json.RawMessage
}

// documentCompleteMetadata contains metadata captured when document edit session completes.
type documentCompleteMetadata struct {
	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

// documentChangeMetadata contains metadata about document changes.
type documentChangeMetadata struct {
	At internal.Time `json:"at"`
}

func (b *B) completeDocumentSession(ctx context.Context, session identifier.Identifier) (*documentCompleteData, errors.E) {
	beginMetadata, endMetadata, _, errE := b.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	if endMetadata.Discarded {
		return &documentCompleteData{
			BeginMetadata: beginMetadata,
			EndMetadata:   endMetadata,
			Changes:       nil,
			Doc:           nil,
		}, nil
	}

	// TODO: Support more than 5000 changes.
	changesList, errE := b.coordinator.List(ctx, session, nil)
	if errE != nil {
		return nil, errE
	}

	// changesList is sorted from newest to oldest change, but we want the opposite as we have forward patches.
	slices.Reverse(changesList)

	changes := make(document.Changes, 0, len(changesList))
	for _, ch := range changesList {
		data, _, errE := b.coordinator.GetData(ctx, session, ch)
		if errE != nil {
			errors.Details(errE)["change"] = ch
			return nil, errE
		}
		change, errE := document.ChangeUnmarshalJSON(data)
		if errE != nil {
			errors.Details(errE)["change"] = ch
			return nil, errE
		}
		changes = append(changes, change)
	}

	// TODO: Get latest revision at the same changeset?
	docJSON, _, errE := b.documents.Get(ctx, beginMetadata.ID, beginMetadata.Version)
	if errE != nil {
		return nil, errE
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(docJSON, &doc)
	if errE != nil {
		return nil, errE
	}

	errE = changes.Apply(&doc)
	if errE != nil {
		return nil, errE
	}

	docJSON, errE = x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return nil, errE
	}

	return &documentCompleteData{
		BeginMetadata: beginMetadata,
		EndMetadata:   endMetadata,
		Changes:       changes,
		Doc:           docJSON,
	}, nil
}

func (b *B) completeDocumentSessionTx(
	ctx context.Context,
	_ pgx.Tx,
	_ identifier.Identifier,
	data *documentCompleteData,
) (*documentCompleteMetadata, errors.E) {
	if data.EndMetadata.Discarded {
		return &documentCompleteMetadata{
			Changeset: nil,
			Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	// We do not have to use the "tx" parameter because we access the transaction through ctx.
	version, errE := b.documents.Update(ctx, data.BeginMetadata.ID, data.BeginMetadata.Version.Changeset, data.Doc, data.Changes, &DocumentMetadata{
		At: data.BeginMetadata.At,
	}, &internal.NoMetadata{})
	if errE != nil {
		return nil, errE
	}

	return &documentCompleteMetadata{
		Changeset: &version.Changeset,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}
