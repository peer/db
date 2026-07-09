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
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
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
	// User is the user who opened the edit session. nil when unauthenticated.
	// Feeds the per-changeset Users union assembled at completion.
	User *store.User `json:"user,omitempty"`
}

// documentEndMetadata contains metadata captured at the end of document edit session.
type documentEndMetadata struct {
	At        store.Time `json:"at"`
	Discarded bool       `json:"discarded,omitempty"`
	// User is the user who ended the session (the committer). nil when
	// unauthenticated. Lands in CommitMetadata.User at completion. NOT included in
	// the Users union on changeset metadata.
	User *store.User `json:"user,omitempty"`
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
	// Metadata is the new metadata for the updated document.
	Metadata *store.DocumentMetadata
}

// DocumentCompleteMetadata contains metadata captured when document edit session completes.
type DocumentCompleteMetadata struct {
	Discarded bool `json:"discarded,omitempty"`
	Errored   bool `json:"errored,omitempty"`

	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

// documentChangeMetadata contains metadata about document changes.
type documentChangeMetadata struct {
	At store.Time `json:"at"`
	// User is the user who appended this change. nil when unauthenticated.
	// Feeds the per-changeset Users union assembled at completion.
	User *store.User `json:"user,omitempty"`
}

// applySessionChanges fetches the session's committed operations past fromOperation,
// validates them, and applies them in order to doc. It returns the collected changes, the
// users who appended them, and the number of the last committed operation (fromOperation when
// there are no further ones).
//
// Operations are validated when they are appended, so validation and apply failures here mean
// the session data itself is broken. Such failures are deterministic and are wrapped with
// coordinator.ErrInvalidSessionData, which cancels the complete-session job instead of
// retrying it when called from completion.
func (b *B) applySessionChanges(
	ctx context.Context, session identifier.Identifier, changesetBase []string, doc *document.D, fromOperation int64,
) (document.Changes, []*store.User, int64, errors.E) {
	lastOperation, errE := b.coordinator.LastOperation(ctx, session)
	if errE != nil {
		return nil, nil, 0, errE
	}
	if lastOperation <= fromOperation {
		return nil, nil, fromOperation, nil
	}

	// Operations are numbered sequentially without gaps, so the committed operations past
	// fromOperation are exactly fromOperation+1 through lastOperation.
	changes := make(document.Changes, 0, lastOperation-fromOperation)
	changeUsers := make([]*store.User, 0, lastOperation-fromOperation)
	for ch := fromOperation + 1; ch <= lastOperation; ch++ {
		data, changeMetadata, errE := b.coordinator.GetData(ctx, session, ch)
		if errE != nil {
			errors.Details(errE)["change"] = ch
			return nil, nil, 0, errE
		}
		change, errE := document.ChangeUnmarshalJSON(data)
		if errE != nil {
			errE = errors.WrapWith(errE, coordinator.ErrInvalidSessionData)
			errors.Details(errE)["change"] = ch
			return nil, nil, 0, errE
		}
		changes = append(changes, change)
		changeUsers = append(changeUsers, changeMetadata.User)
	}

	errE = changes.Validate(changesetBase, fromOperation)
	if errE != nil {
		return nil, nil, 0, errors.WrapWith(errE, coordinator.ErrInvalidSessionData)
	}

	errE = changes.Apply(doc)
	if errE != nil {
		return nil, nil, 0, errors.WrapWith(errE, coordinator.ErrInvalidSessionData)
	}

	return changes, changeUsers, lastOperation, nil
}

// rebuildSessionDocument returns the session's document state with all committed operations
// applied, together with the number of the last committed operation (fromOperation when there
// are no further ones).
//
// With a nil from it rebuilds from scratch: the session's parent document (or an empty
// document for a create session) with all committed operations applied. With a non-nil from,
// which is the session's state after fromOperation (the caller owns it and it may be mutated),
// only the operations past fromOperation are applied.
func (b *B) rebuildSessionDocument(
	ctx context.Context, session identifier.Identifier, beginMetadata *DocumentBeginMetadata, from *document.D, fromOperation int64,
) (*document.D, int64, errors.E) {
	doc := from
	if doc == nil {
		fromOperation = 0
		if beginMetadata.Version != nil {
			// Edit session: load parent document at the session's begin version.
			docJSON, _, _, _, errE := b.documents.Get(ctx, beginMetadata.DocumentID, *beginMetadata.Version)
			if errE != nil {
				return nil, 0, errE
			}
			doc = new(document.D)
			errE = x.UnmarshalWithoutUnknownFields(docJSON, doc)
			if errE != nil {
				return nil, 0, errE
			}
		} else {
			// Create session: start from an empty document with the pre-allocated id/base.
			doc = &document.D{
				CoreDocument: document.CoreDocument{
					ID:   beginMetadata.DocumentID,
					Base: slices.Clone(beginMetadata.Base),
				},
			}
		}
	}

	// doc.Base should be equal to beginMetadata.Base. We use doc.Base here on purpose, to
	// validate that use of beginMetadata.Base in AppendDocumentChange matches.
	changesetBase := slices.Clone(doc.Base)
	changesetBase = append(changesetBase, "SESSION", session.String())

	_, _, lastOperation, errE := b.applySessionChanges(ctx, session, changesetBase, doc, fromOperation)
	if errE != nil {
		return nil, 0, errE
	}

	return doc, lastOperation, nil
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

	var doc document.D
	var resolvedVersion store.Version
	if beginMetadata.Version != nil {
		// Edit session: load parent document at the session's begin version.
		// Version has Revision 0, so Get returns the latest revision for the changeset.
		var docJSON json.RawMessage
		docJSON, _, resolvedVersion, _, errE = b.documents.Get(ctx, beginMetadata.DocumentID, *beginMetadata.Version)
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

	changes, changeUsers, _, errE := b.applySessionChanges(ctx, session, base, &doc, 0)
	if errE != nil {
		return nil, errE
	}

	// If there are no changes, treat the session as discarded.
	if len(changes) == 0 {
		return &documentCompleteData{
			BeginMetadata: beginMetadata,
			EndMetadata:   endMetadata,
			Changes:       nil,
			Document:      nil,
			ParentVersion: store.Version{},
			Metadata:      nil,
		}, nil
	}

	docJSON, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		return nil, errE
	}

	// The users are the begin user and every per-change user; the end user is intentionally
	// excluded (it belongs on CommitMetadata.User). internalStore.SortedUniqueUsers drops
	// nils, so unauthenticated participants are skipped.
	users := make([]*store.User, 0, len(changeUsers)+1)
	users = append(users, beginMetadata.User)
	users = append(users, changeUsers...)

	// Compute new metadata for this version.
	newMetadata := &store.DocumentMetadata{
		At:    endMetadata.At,
		Users: internalStore.SortedUniqueUsers(users),
	}

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
			Errored:   false,
			Changeset: nil,
			Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
		}, nil
	}

	// We do not have to use the "tx" parameter because we access the transaction through ctx.
	commitMetadata := &store.CommitMetadata{
		Base: changesetBase,
		User: data.EndMetadata.User,
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
		// The synthesized empty-document insert is attributed to the begin user
		// (sole contributor at this synthetic step) and committed by the end user
		// as part of the same End call.
		firstVersion, errE := b.documents.Insert(
			ctx, data.BeginMetadata.DocumentID, emptyJSON,
			&store.DocumentMetadata{
				At:    data.EndMetadata.At,
				Users: internalStore.SortedUniqueUsers([]*store.User{data.BeginMetadata.User}),
			},
			&store.CommitMetadata{Base: firstBase, User: data.EndMetadata.User},
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
		Errored:   false,
		Changeset: &version.Changeset,
		Time:      time.Since(time.Time(data.EndMetadata.At)).Milliseconds(),
	}, nil
}

func (b *B) completeSessionOnErrorTx(
	ctx context.Context,
	_ pgx.Tx,
	session identifier.Identifier,
	completeErr error,
) (*DocumentCompleteMetadata, errors.E) {
	beginMetadata, endMetadata, _, errE := b.coordinator.Get(ctx, session)
	if errE != nil {
		return nil, errE
	}

	changesetBase := slices.Clone(beginMetadata.Base)
	changesetBase = append(changesetBase, "SESSION", session.String())

	changesetID := identifier.From(changesetBase...)
	changeset, errE := b.files.Store().Changeset(ctx, changesetID)
	if errE != nil {
		return nil, errE
	}

	// There might be files uploaded into a changeset in file storage store.
	// We discard the changeset here to remove them.
	// Discarding an empty (or an already discarded) changeset is not an error,
	// so this should not error if no file uploads were made into the document edit session.
	errE = changeset.Discard(ctx)
	if errE != nil {
		return nil, errE
	}

	return &DocumentCompleteMetadata{
		Discarded: completeErr == nil,
		Errored:   completeErr != nil,
		Changeset: nil,
		Time:      time.Since(time.Time(endMetadata.At)).Milliseconds(),
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
