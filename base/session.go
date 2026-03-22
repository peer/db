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
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// DocumentBeginMetadata contains metadata captured at the beginning of document edit session.
type DocumentBeginMetadata struct {
	At       internalStore.Time    `json:"at"`
	Document identifier.Identifier `json:"document"`
	Version  store.Version         `json:"version"`
}

// documentEndMetadata contains metadata captured at the end of document edit session.
type documentEndMetadata struct {
	At        internalStore.Time `json:"at"`
	Discarded bool               `json:"discarded,omitempty"`
}

// documentCompleteData contains JSON serialized document with metadata to be
// passed between CompleteSession and CompleteSessionTx.
type documentCompleteData struct {
	BeginMetadata *DocumentBeginMetadata
	EndMetadata   *documentEndMetadata
	Changes       document.Changes
	Doc           json.RawMessage
	// ParentVersion is the resolved version (with actual revision) of the parent document
	// at which metadata was fetched and changes were validated.
	ParentVersion store.Version
	// Metadata is the new metadata for the updated document, with system-managed
	// fields carried over from the parent version.
	Metadata *internalStore.DocumentMetadata
}

// documentCompleteMetadata contains metadata captured when document edit session completes.
type documentCompleteMetadata struct {
	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

// documentChangeMetadata contains metadata about document changes.
type documentChangeMetadata struct {
	At internalStore.Time `json:"at"`
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
			ParentVersion: store.Version{},
			Metadata:      nil,
		}, nil
	}

	// TODO: Support more than 5000 changes.
	changesList, errE := b.coordinator.List(ctx, session, nil)
	if errE != nil {
		return nil, errE
	}

	// changesList is sorted from newest to oldest change, but we want the opposite as we have forward patches.
	slices.Reverse(changesList)

	// If there are no changes, treat the session as discarded.
	if len(changesList) == 0 {
		return &documentCompleteData{
			BeginMetadata: beginMetadata,
			EndMetadata:   endMetadata,
			Changes:       nil,
			Doc:           nil,
			ParentVersion: store.Version{},
			Metadata:      nil,
		}, nil
	}

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

	// Version has Revision 0, so Get returns the latest revision for the changeset,
	// picking up any metadata updates made by the system (e.g., bridge) since the session began.
	docJSON, oldMetadata, resolvedVersion, _, errE := b.documents.Get(ctx, beginMetadata.Document, beginMetadata.Version)
	if errE != nil {
		return nil, errE
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(docJSON, &doc)
	if errE != nil {
		return nil, errE
	}

	base := []string{doc.ID.String(), "SESSION", session.String()}

	errE = changes.Validate(base)
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

	// Compute new metadata, carrying over system-managed fields from the parent version.
	newMetadata := &internalStore.DocumentMetadata{
		At:               endMetadata.At,
		InverseRelations: nil,
	}
	newMetadata.CarryOver(oldMetadata)

	return &documentCompleteData{
		BeginMetadata: beginMetadata,
		EndMetadata:   endMetadata,
		Changes:       changes,
		Doc:           docJSON,
		ParentVersion: resolvedVersion,
		Metadata:      newMetadata,
	}, nil
}

func (b *B) completeDocumentSessionTx(
	ctx context.Context,
	_ pgx.Tx,
	_ identifier.Identifier,
	data *documentCompleteData,
) (*documentCompleteMetadata, errors.E) {
	// No changes to commit: either the session was explicitly discarded or
	// it was ended without any changes (treated the same way).
	if data.Doc == nil {
		return &documentCompleteMetadata{
			Changeset: nil,
			Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	// We do not have to use the "tx" parameter because we access the transaction through ctx.
	// We use the parent version's changeset so the update is based on the same version (with actual revision)
	// at which metadata was fetched and changes were validated in completeDocumentSession.
	version, errE := b.documents.Update(ctx, data.BeginMetadata.Document, data.ParentVersion.Changeset, data.Doc, data.Changes, data.Metadata, &internalStore.NoMetadata{})
	if errE != nil {
		return nil, errE
	}

	return &documentCompleteMetadata{
		Changeset: &version.Changeset,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}
