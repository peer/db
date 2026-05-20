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

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// DocumentBeginMetadata contains metadata captured at the beginning of document edit session.
//
// Version is nil for create sessions (the session creates a new document; no parent
// version exists in the store yet). Otherwise it is the parent version the edit branches from.
type DocumentBeginMetadata struct {
	At         store.Time            `json:"at"`
	DocumentID identifier.Identifier `json:"documentId"`
	Base       []string              `json:"base"`
	Version    *store.Version        `json:"version,omitempty"`
}

// documentEndMetadata contains metadata captured at the end of document edit session.
type documentEndMetadata struct {
	At        store.Time `json:"at"`
	Discarded bool       `json:"discarded,omitempty"`
}

// documentCompleteData contains JSON serialized document with metadata to be
// passed between CompleteSession and CompleteSessionTx.
type documentCompleteData struct {
	BeginMetadata *DocumentBeginMetadata
	EndMetadata   *documentEndMetadata
	Changes       document.Changes
	Document      json.RawMessage
	// ParentVersion is the resolved version (with actual revision) of the parent document
	// at which metadata was fetched and changes were validated.
	ParentVersion store.Version
	// Metadata is the new metadata for the updated document, with system-managed
	// fields carried over from the parent version.
	Metadata *store.DocumentMetadata
}

// DocumentCompleteMetadata contains metadata captured when document edit session completes.
type DocumentCompleteMetadata struct {
	Discarded bool `json:"discarded,omitempty"`

	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

// documentChangeMetadata contains metadata about document changes.
type documentChangeMetadata struct {
	At store.Time `json:"at"`
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
			Document:      nil,
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
			Document:      nil,
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

	var doc document.D
	var oldMetadata *store.DocumentMetadata
	var resolvedVersion store.Version
	if beginMetadata.Version != nil {
		// Edit session: load parent document at the session's begin version.
		// Version has Revision 0, so Get returns the latest revision for the changeset,
		// picking up any metadata updates made by the system (e.g., bridge) since the session began.
		var docJSON json.RawMessage
		docJSON, oldMetadata, resolvedVersion, _, errE = b.documents.Get(ctx, beginMetadata.DocumentID, *beginMetadata.Version)
		if errE != nil {
			return nil, errE
		}

		errE = x.UnmarshalWithoutUnknownFields(docJSON, &doc)
		if errE != nil {
			return nil, errE
		}
	} else {
		// Create session: start from an empty document with the pre-allocated id/base.
		// The actual store insert happens in completeDocumentSessionTx.
		doc = document.D{
			CoreDocument: document.CoreDocument{
				ID:   beginMetadata.DocumentID,
				Base: slices.Clone(beginMetadata.Base),
			},
		}
	}

	// doc.Base should be equal to beginMetadata.Base.
	// For edit sessions we use doc.Base here on purpose, to validate that use of
	// beginMetadata.Base in AppendDocumentChange matches.
	base := slices.Clone(doc.Base)
	base = append(base, "SESSION", session.String())

	errE = changes.Validate(base)
	if errE != nil {
		return nil, errE
	}

	errE = changes.Apply(&doc)
	if errE != nil {
		return nil, errE
	}

	docJSON, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return nil, errE
	}

	// Compute new metadata, carrying over system-managed fields from the parent version
	// (no-op when oldMetadata is nil, which is the create-session case).
	newMetadata := &store.DocumentMetadata{
		At:               endMetadata.At,
		InverseRelations: nil,
	}
	newMetadata.CarryOver(oldMetadata)

	return &documentCompleteData{
		BeginMetadata: beginMetadata,
		EndMetadata:   endMetadata,
		Changes:       changes,
		Document:      docJSON,
		ParentVersion: resolvedVersion,
		Metadata:      newMetadata,
	}, nil
}

func (b *B) completeDocumentSessionTx(
	ctx context.Context,
	_ pgx.Tx,
	session identifier.Identifier,
	data *documentCompleteData,
) (*DocumentCompleteMetadata, errors.E) {
	// BeginMetadata.Base is the same as doc.Base.
	changesetBase := slices.Clone(data.BeginMetadata.Base)
	changesetBase = append(changesetBase, "SESSION", session.String())

	changesetID := identifier.From(changesetBase...)
	changeset, errE := b.files.Store().Changeset(ctx, changesetID)
	if errE != nil {
		return nil, errE
	}

	// No changes to commit: either the session was explicitly discarded or
	// it was ended without any changes (treated the same way).
	if data.Document == nil {
		// There might be files uploaded into a changeset in file storage store.
		// We discard the changeset here to remove them.
		// Discarding an empty (or an already discarded) changeset is not an error,
		// so this should not error if no file uploads were made into the document edit session.
		errE := changeset.Discard(ctx)
		if errE != nil {
			return nil, errE
		}

		return &DocumentCompleteMetadata{
			Discarded: true,
			Changeset: nil,
			Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	// We do not have to use the "tx" parameter because we access the transaction through ctx.
	commitMetadata := &store.CommitMetadata{
		Base: changesetBase,
	}

	// Resolve the parent changeset the Update will branch from. For edit sessions
	// it is the parent version's changeset (so the update is based on the same
	// version, with actual revision, at which metadata was fetched and changes
	// were validated in completeDocumentSession). For create sessions the parent
	// is the FIRST changeset we synthesize here by inserting an empty document.
	// We do the insert + update in the same River-job transaction so a discard
	// or failure in the second step leaves the store unchanged.
	var parentChangeset identifier.Identifier
	if data.BeginMetadata.Version != nil {
		parentChangeset = data.ParentVersion.Changeset
	} else {
		emptyDoc := &document.D{
			CoreDocument: document.CoreDocument{
				ID:   data.BeginMetadata.DocumentID,
				Base: data.BeginMetadata.Base,
			},
		}
		emptyJSON, errE := x.MarshalWithoutEscapeHTML(emptyDoc)
		if errE != nil {
			return nil, errE
		}
		firstBase := slices.Clone(data.BeginMetadata.Base)
		firstBase = append(firstBase, "CHANGESET", "FIRST")
		firstVersion, errE := b.documents.Insert(
			ctx, data.BeginMetadata.DocumentID, emptyJSON,
			&store.DocumentMetadata{
				At:               data.EndMetadata.At,
				InverseRelations: nil,
			},
			&store.CommitMetadata{Base: firstBase},
		)
		if errE != nil {
			return nil, errE
		}
		parentChangeset = firstVersion.Changeset
	}

	version, errE := b.documents.Update(
		ctx, data.BeginMetadata.DocumentID, parentChangeset,
		data.Document, data.Changes, data.Metadata, commitMetadata,
	)
	if errE != nil {
		return nil, errE
	}

	// There might be files uploaded into a changeset in file storage store.
	// We commit the changeset here to persist them.
	_, errE = b.files.Store().Commit(ctx, changeset, commitMetadata)
	if errE != nil && !errors.Is(errE, store.ErrChangesetNotFound) {
		// ErrChangesetNotFound is fine. It means no file uploads were made into the document edit session.
		return nil, errE
	}

	return &DocumentCompleteMetadata{
		Discarded: false,
		Changeset: &version.Changeset,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}

type primaryCoordinator struct {
	*coordinator.Coordinator[json.RawMessage, *documentChangeMetadata, *DocumentBeginMetadata, *documentEndMetadata, *documentCompleteData, *DocumentCompleteMetadata]
}

// ChangesetID implements storage.PrimaryCoordinator interface.
func (p *primaryCoordinator) ChangesetID(ctx context.Context, session identifier.Identifier) (identifier.Identifier, errors.E) {
	// This check runs inside a transaction.
	beginMetadata, endMetadata, completeMetadata, errE := p.Get(ctx, session)
	if errE != nil {
		return identifier.Identifier{}, errE
	} else if endMetadata != nil {
		return identifier.Identifier{}, errors.WithStack(coordinator.ErrAlreadyEnded)
	} else if completeMetadata != nil {
		return identifier.Identifier{}, errors.WithStack(coordinator.ErrAlreadyCompleted)
	}

	// Here we use changeset base for ending a document session.
	changesetBase := slices.Clone(beginMetadata.Base)
	changesetBase = append(changesetBase, "SESSION", session.String())

	return identifier.From(changesetBase...), nil
}
