package peerdb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

// checkDocumentPermission reports whether the caller in ctx holds the permission action on the
// document on the given site (see auth.HasDocumentPermission), with the subject and the roles taken
// from ctx.
func checkDocumentPermission(ctx context.Context, site *internalSite.Site, action identifier.Identifier, doc *document.D) bool {
	subject, _ := auth.Subject(ctx)
	return auth.HasDocumentPermission(site.Roles, action, subject, auth.Roles(ctx), doc)
}

// checkFilePermission reports whether the caller in ctx holds the permission action on files on the
// given site (see auth.HasFilePermission), with the roles taken from ctx.
func checkFilePermission(ctx context.Context, site *internalSite.Site, action identifier.Identifier) bool {
	return auth.HasFilePermission(site.Roles, action, auth.Roles(ctx))
}

// checkRoleDocumentPermission reports whether the caller in ctx holds the permission action on the
// document through role grants alone: document-level permission claims are left out of the check (the
// subject is not passed), so it is used for operations which document-level claims must not allow.
func checkRoleDocumentPermission(ctx context.Context, site *internalSite.Site, action identifier.Identifier, doc *document.D) bool {
	return auth.HasDocumentPermission(site.Roles, action, "", auth.Roles(ctx), doc)
}

// checkVersionedReadPermission reports whether the caller in ctx may read the given version of the
// document on the given site: the read action on the document at its latest version, together with
// the read action at the given version or the historic read action on the document (the same rule
// DefaultDocumentPostHook enforces). Documents are fetched raw and are used only for the check.
func checkVersionedReadPermission(ctx context.Context, site *internalSite.Site, id identifier.Identifier, version store.Version) (bool, errors.E) {
	latestJSON, _, _, _, errE := site.Base.Documents().GetLatest(ctx, id) //nolint:dogsled
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		// TODO: Expose deleted documents and their versions once we have an undelete action.
		//       Deletion currently makes the document and all of its versions inaccessible to
		//       everyone. With an undelete action, callers holding it would keep seeing deleted
		//       documents and could undelete them.
		return false, nil
	} else if errE != nil {
		return false, errE
	}
	latest := new(document.D)
	errE = x.UnmarshalWithoutUnknownFields(latestJSON, latest)
	if errE != nil {
		return false, errE
	}
	if !checkDocumentPermission(ctx, site, auth.ActionRead, latest) {
		// The caller does not have the read action on the latest version of the document: the document
		// is fully inaccessible to the caller.
		return false, nil
	}
	if checkDocumentPermission(ctx, site, auth.ActionReadHistoric, latest) {
		// The caller has the historic read action on the latest version of the document: we do not have
		// to check further, any version is accessible to the caller.
		return true, nil
	}
	docJSON, _, _, _, errE := site.Base.Documents().Get(ctx, id, version) //nolint:dogsled
	if errors.Is(errE, store.ErrValueDeleted) {
		// A deleted version carries no claims through which era readers could be allowed, so only
		// callers holding the historic read action see it.
		return false, nil
	} else if errE != nil {
		return false, errE
	}
	doc := new(document.D)
	errE = x.UnmarshalWithoutUnknownFields(docJSON, doc)
	if errE != nil {
		return false, errE
	}
	// Does the caller have read access at that particular version?
	return checkDocumentPermission(ctx, site, auth.ActionRead, doc), nil
}

// topLevelClaim is a top-level claim of a document as compared by changedClaimProperties: its
// property, its concrete type, and its serialized form (which includes its sub-claims). The type is
// compared separately because it is not part of a claim's own serialization (in a document the type
// is encoded by the claim list the claim is in), so a cast between claim types with equal fields
// (e.g. between a has and a none claim) would otherwise not register as a change.
type topLevelClaim struct {
	Prop identifier.Identifier
	Type reflect.Type
	Data json.RawMessage
}

// topLevelClaimsByID returns the document's top-level claims (with their sub-claims), serialized, by
// their IDs.
func topLevelClaimsByID(doc *document.D) (map[identifier.Identifier]topLevelClaim, errors.E) {
	claims := map[identifier.Identifier]topLevelClaim{}
	for claim := range doc.AllClaims() {
		data, errE := x.MarshalWithoutEscapeHTML(claim)
		if errE != nil {
			return nil, errE
		}
		claims[claim.GetID()] = topLevelClaim{Prop: claim.GetProp().ID, Type: reflect.TypeOf(claim), Data: data}
	}
	return claims, nil
}

// changedClaimProperties returns the properties of the top-level claims which differ (in themselves,
// including their property, or in their sub-claims) between the two documents, together with the
// actions granted by the HAS_PERMISSION claims among the added or changed ones. A claim present in
// only one of the documents contributes its property in that document; a differing claim present in
// both contributes its property in each of them, so a change setting a claim's property to another
// one contributes both properties. Granted actions do not include removed claims (removing a grant
// is not granting) nor HAS_REQUESTED_PERMISSION claims (they request an action and grant nothing).
func changedClaimProperties(before, after *document.D) (map[identifier.Identifier]bool, []identifier.Identifier, errors.E) {
	beforeClaims, errE := topLevelClaimsByID(before)
	if errE != nil {
		return nil, nil, errE
	}
	afterClaims, errE := topLevelClaimsByID(after)
	if errE != nil {
		return nil, nil, errE
	}
	changed := map[identifier.Identifier]bool{}
	for id, beforeClaim := range beforeClaims {
		afterClaim, ok := afterClaims[id]
		if !ok {
			changed[beforeClaim.Prop] = true
		} else if beforeClaim.Type != afterClaim.Type || !bytes.Equal(beforeClaim.Data, afterClaim.Data) {
			changed[beforeClaim.Prop] = true
			changed[afterClaim.Prop] = true
		}
	}
	for id, afterClaim := range afterClaims {
		if _, ok := beforeClaims[id]; !ok {
			changed[afterClaim.Prop] = true
		}
	}
	var grantedActions []identifier.Identifier
	// The confidence threshold matches auth.PermissionClaimGrants: a claim below it grants nothing,
	// and raising its confidence later is a change of the claim, so it is included here then.
	for _, claim := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](after, internalCore.HasPermissionPropID, document.LowConfidence) {
		beforeClaim, ok := beforeClaims[claim.ID]
		afterClaim := afterClaims[claim.ID]
		if ok && beforeClaim.Type == afterClaim.Type && bytes.Equal(beforeClaim.Data, afterClaim.Data) {
			continue
		}
		grantedActions = append(grantedActions, claim.To.ID)
	}
	return changed, grantedActions, nil
}

// checkChangePermission returns nil when the caller in ctx may append a change transforming an edit
// session's document from before to after on the given site. The change is classified by the
// properties of the top-level claims it modifies and it has to satisfy the requirement of every kind
// of property it touches: a single change can touch several kinds at once (setting or casting a
// claim's property to another one modifies claims of both properties), so the requirements are
// checked conjunctively and not as alternatives. A change granting a permission (adding or modifying
// a HAS_PERMISSION claim) additionally requires the granted action itself, so the caller cannot
// grant (to anyone, including themselves) a permission they do not hold.
func checkChangePermission(ctx context.Context, site *internalSite.Site, before, after *document.D) errors.E {
	changed, grantedActions, errE := changedClaimProperties(before, after)
	if errE != nil {
		return errE
	}

	permissionChanged := false
	scopeChanged := false
	otherChanged := false
	for prop := range changed {
		isPermission := prop == internalCore.HasPermissionPropID || prop == internalCore.HasRequestedPermissionPropID
		if isPermission {
			permissionChanged = true
		}
		if site.ScopeProperties[prop] {
			scopeChanged = true
		}
		if !isPermission && !site.ScopeProperties[prop] {
			otherChanged = true
		}
	}

	// Permission claims (HAS_PERMISSION and HAS_REQUESTED_PERMISSION, with their sub-claims) require the permissions
	// action, with the document's own claims consulted in the state before the change, so permissions authority cannot
	// be minted by the change itself.
	if permissionChanged && !checkDocumentPermission(ctx, site, auth.ActionUpdatePermissions, before) {
		errE = errors.WithStack(auth.ErrAccessDenied)
		errors.Details(errE)["action"] = auth.ActionUpdatePermissions.String()
		return errE
	}
	// Granting a permission (a HAS_PERMISSION claim among the added or changed claims) also requires the granted
	// action itself, again held on the document in the state before the change, so the caller cannot amplify their
	// permissions by granting an action they do not hold.
	for _, action := range grantedActions {
		if !checkDocumentPermission(ctx, site, action, before) {
			errE = errors.WithStack(auth.ErrAccessDenied)
			errors.Details(errE)["action"] = action.String()
			return errE
		}
	}
	// Claims of properties participating in permission scopes of any role grant (e.g. instance_of when class scopes are
	// configured) require the update action from role grants alone, both before and after the change: document-level
	// claims are blind to those properties, so they must not allow moving the document across scopes.
	if scopeChanged && (!checkRoleDocumentPermission(ctx, site, auth.ActionUpdate, before) || !checkRoleDocumentPermission(ctx, site, auth.ActionUpdate, after)) {
		errE = errors.WithStack(auth.ErrAccessDenied)
		errors.Details(errE)["action"] = auth.ActionUpdate.String()
		return errE
	}
	// Claims of other properties, and a change modifying no top-level claims at all, require the update action, both before
	// and after the change, so a caller can neither produce a state they may not update nor lock themselves out.
	if (otherChanged || len(changed) == 0) &&
		(!checkDocumentPermission(ctx, site, auth.ActionUpdate, before) || !checkDocumentPermission(ctx, site, auth.ActionUpdate, after)) {
		errE = errors.WithStack(auth.ErrAccessDenied)
		errors.Details(errE)["action"] = auth.ActionUpdate.String()
		return errE
	}
	return nil
}

// HasDocumentPermission reports whether the caller currently holds the permission action on documents of the
// site this request targets: through the caller's role grants or, when doc is non-nil, through the
// document's own permission claims (see auth.HasPermissionClaim). With a nil doc only role grants are
// checked, against documents in general. Returns nil on success and an "access denied" error otherwise
// (including when no site is in ctx). In sync with hasPermission in src/auth/index.ts.
func (s *Service) HasDocumentPermission(ctx context.Context, action identifier.Identifier, doc *document.D) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return errors.WithStack(auth.ErrAccessDenied)
	}
	if checkDocumentPermission(ctx, site, action, doc) {
		return nil
	}
	errE := errors.WithStack(auth.ErrAccessDenied)
	errors.Details(errE)["action"] = action.String()
	return errE
}

// HasFilePermission reports whether the caller currently holds the permission action on files of the site
// this request targets: a role grant with a scope covering files (files or all). Returns nil on success
// and an "access denied" error otherwise.
func (s *Service) HasFilePermission(ctx context.Context, action identifier.Identifier) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return errors.WithStack(auth.ErrAccessDenied)
	}
	if checkFilePermission(ctx, site, action) {
		return nil
	}
	errE := errors.WithStack(auth.ErrAccessDenied)
	errors.Details(errE)["action"] = action.String()
	return errE
}

// HasSessionPermission reports whether the caller may access the edit session: its state, its
// changes, and its status. The check is made against the document related to the session at the
// current point of the session's lifecycle (its in-progress state, the version it committed, or the
// version it started from), so who may access the session can change as the session progresses.
// Which action a change appended to a session requires is checked separately, per change (see
// checkChangePermission); this method gates general access to the session's endpoints. A session
// which does not exist is reported with coordinator.ErrSessionNotFound so callers can distinguish
// it from a permission denial.
func (s *Service) HasSessionPermission(ctx context.Context, session identifier.Identifier) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return errors.WithStack(auth.ErrAccessDenied)
	}

	// For an active session (or an ended one which is still completing asynchronously) the update
	// action is checked against the session's current document state, so users granted an action
	// only within the session have access, too.
	_, doc, errE := site.Base.SessionDocument(ctx, session)
	if errors.Is(errE, coordinator.ErrAlreadyCompleted) {
		beginMetadata, _, completeMetadata, errE := site.Base.GetEditDocumentSession(ctx, session)
		if errE != nil {
			return errE
		}
		// The document version related to the completed session.
		var version *store.Version
		if completeMetadata.Changeset != nil {
			// The version the session committed (permission claims added within the session are part of it,
			// so access does not change across the completion boundary). Revision 0 resolves to the
			// changeset's latest revision.
			version = &store.Version{Changeset: *completeMetadata.Changeset, Revision: 0}
		} else {
			// The version the session branched from, because it committed nothing (it was discarded or
			// errored): permissions reset to the session's beginning, matching the check made when the
			// session was opened, so users granted the update action before the session keep access,
			// users granted it only within the session lose it.
			version = beginMetadata.Version
		}
		// A create session which committed nothing relates to no document anymore and the create
		// action is checked without a document (only role grants can allow it), again matching the
		// check made when the session was opened.
		if version == nil {
			return s.HasDocumentPermission(ctx, auth.ActionCreate, nil)
		}
		// The document is obtained raw (its permission claims have to count also for callers who may
		// not read it) and is used only for the permission check.
		docJSON, _, _, _, errE := site.Base.Documents().Get(ctx, beginMetadata.DocumentID, *version)
		if errE != nil {
			return errE
		}
		doc = new(document.D)
		errE = x.UnmarshalWithoutUnknownFields(docJSON, doc)
		if errE != nil {
			return errE
		}
	} else if errE != nil {
		return errE
	}

	return s.HasDocumentPermission(ctx, auth.ActionUpdate, doc)
}

// The default hooks below enforce the read action on the read path from the site's role grants and (for
// documents) document-level permission claims. A site which customizes a hook list and still wants the
// default enforcement has to include the corresponding default hook in the list itself. Outside of a
// request context (no site in ctx) they allow everything, because such reads are internal (e.g.
// populating).

// DefaultDocumentPreHook denies fetching documents to callers who can never read any document. It is a
// partial check: callers it passes are fully checked by DefaultDocumentPostHook once the document is
// available.
func DefaultDocumentPreHook(ctx context.Context, _ identifier.Identifier, _ *store.Version) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return nil
	}
	// All authenticated callers pass, because we can fully check them only in DefaultDocumentPostHook.
	// The fetched document's own permission claims may grant authenticated callers the read action,
	// so they can be fully checked only once the document is available.
	if _, ok := auth.Subject(ctx); ok {
		return nil
	}
	// Anonymous callers might have permissions to access documents. Document-level permission claims
	// cannot grant anything to anonymous callers, so an anonymous caller without a role grant of the
	// read action covering documents can be denied already.
	if checkDocumentPermission(ctx, site, auth.ActionRead, nil) {
		return nil
	}
	return errors.WithStack(auth.ErrAccessDenied)
}

// DefaultDocumentPostHook enforces the read actions on the fetched document, from role grants with a
// scope covering the document and from the document's own permission claims: the caller has to hold
// the read action on the document at its latest version, and a fetched version other than the latest
// additionally has to be readable at that version or the caller has to hold the historic read action
// on the document.
func DefaultDocumentPostHook(
	ctx context.Context, doc, latest *document.D, metadata *store.DocumentMetadata, version store.Version, parentChangesets []store.Version, errE errors.E,
) (*document.D, *store.DocumentMetadata, store.Version, []store.Version, errors.E) {
	if errE != nil || doc == nil {
		return doc, metadata, version, parentChangesets, errE
	}
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return doc, metadata, version, parentChangesets, nil
	}
	// A document deleted at its latest version is not accessible at any version. This should generally
	// not be possible because errE should be set to store.ErrValueNotFound (which includes ErrValueDeleted)
	// if the latest document is deleted, but just in case, we check here.
	// TODO: Expose deleted documents and their versions once we have an undelete action.
	//       Deletion currently makes the document and all of its versions inaccessible to everyone.
	//       With an undelete action, callers holding it would keep seeing deleted documents and could
	//       undelete them. An undelete hook has to run after this hook (or replace it), because this
	//       check denies a nil latest even when an earlier hook cleared the deleted error.
	if latest == nil {
		return nil, metadata, version, parentChangesets, errors.WithStack(auth.ErrAccessDenied)
	}
	// The read action on the document at its latest version gates every version: removing a grant
	// from the document removes access to all of its versions.
	if !checkDocumentPermission(ctx, site, auth.ActionRead, latest) {
		return doc, metadata, version, parentChangesets, errors.WithStack(auth.ErrAccessDenied)
	}
	// A version is readable when the read action passes at that version (for a latest read this is
	// the check above, as doc and latest have the same content) or when the caller holds the
	// historic read action on the document.
	if checkDocumentPermission(ctx, site, auth.ActionRead, doc) || checkDocumentPermission(ctx, site, auth.ActionReadHistoric, latest) {
		return doc, metadata, version, parentChangesets, nil
	}
	return doc, metadata, version, parentChangesets, errors.WithStack(auth.ErrAccessDenied)
}

// DefaultFilePreHook enforces the read action on files, from role grants with a scope covering files.
func DefaultFilePreHook(ctx context.Context, _ identifier.Identifier, _ *store.Version) errors.E {
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return nil
	}
	// In contrast with DefaultDocumentPreHook we do not pass all authenticated callers here because
	// currently permission checks on files do not rely on a file itself.
	if checkFilePermission(ctx, site, auth.ActionRead) {
		return nil
	}
	return errors.WithStack(auth.ErrAccessDenied)
}

// DefaultFilePostHook enforces the read actions on the fetched file, from role grants with a scope
// covering files: the caller has to hold the read action on files, and a fetched version other than
// the file's latest version additionally requires the historic read action on files. When it denies
// access it closes the file handle, per the FilePostHooks contract.
func DefaultFilePostHook(
	ctx context.Context, file io.ReadSeekCloser, latestVersion *store.Version, metadata *storage.FileMetadata,
	version store.Version, parentChangesets []store.Version, errE errors.E,
) (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
	if errE != nil {
		return file, metadata, version, parentChangesets, errE
	}
	site, ok := waf.GetSite[*internalSite.Site](ctx)
	if !ok {
		return file, metadata, version, parentChangesets, nil
	}
	deny := func() (io.ReadSeekCloser, *storage.FileMetadata, store.Version, []store.Version, errors.E) {
		if file != nil {
			_ = file.Close()
		}
		return nil, metadata, version, parentChangesets, errors.WithStack(auth.ErrAccessDenied)
	}
	// A file deleted at its latest version is not accessible at any version. This should generally
	// not be possible because errE should be set to store.ErrValueNotFound (which includes
	// ErrValueDeleted) if the latest file is deleted, but just in case, we check here.
	// TODO: Expose deleted files and their versions once we have an undelete action.
	//       Deletion currently makes the file and all of its versions inaccessible to everyone. With
	//       an undelete action, callers holding it would keep seeing deleted files and could
	//       undelete them. An undelete hook has to run after this hook (or replace it), because this
	//       check denies a nil latest version even when an earlier hook cleared the deleted error.
	if latestVersion == nil {
		return deny()
	}
	// The read action on files gates every version: without it no file is accessible.
	if !checkFilePermission(ctx, site, auth.ActionRead) {
		return deny()
	}
	// A version other than the file's latest version requires the historic read action on files.
	// File permission checks do not depend on the file's version (files have no file-level
	// permission claims), so no other check is needed.
	if version.Equals(*latestVersion) || checkFilePermission(ctx, site, auth.ActionReadHistoric) {
		return file, metadata, version, parentChangesets, nil
	}
	return deny()
}

// checkVersionedFileReadPermission reports whether the caller in ctx may read the given version of
// the file on the given site: the read action on files, together with the version being the file's
// latest version or the historic read action on files (the same rule DefaultFilePostHook enforces).
func checkVersionedFileReadPermission(ctx context.Context, site *internalSite.Site, id identifier.Identifier, version store.Version) (bool, errors.E) {
	_, _, latestVersion, _, errE := site.Base.Files().GetLatest(ctx, id) //nolint:dogsled
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		// TODO: Expose deleted files and their versions once we have an undelete action.
		return false, nil
	} else if errE != nil {
		return false, errE
	}
	if !checkFilePermission(ctx, site, auth.ActionRead) {
		// The caller does not have the read action on files: files are fully inaccessible to the
		// caller.
		return false, nil
	}
	if version.Equals(latestVersion) {
		// The version is the file's latest version, which the read action alone allows.
		return true, nil
	}
	// Any other version requires the historic read action on files.
	return checkFilePermission(ctx, site, auth.ActionReadHistoric), nil
}
