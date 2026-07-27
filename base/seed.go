package base

import (
	"maps"
	"net/http"
	"slices"
	"strconv"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

type emptyRequest struct{}

// permissionClaimChanges returns the changes adding a claim of the given property (HAS_PERMISSION
// or HAS_REQUESTED_PERMISSION) with the action as the value, together with the changes adding under
// it a PERMISSION_USER sub-claim with the user, a PERMISSION_SCOPE sub-claim with the self scope,
// and, when note is not empty, a DESCRIPTION sub-claim with it: free text about the claim, which for
// a request is what the requester wrote to whoever decides it. The claim IDs are derived from the
// seed base as operations startOperation+1 onward. It returns the changes and the operation number
// of the last one.
func permissionClaimChanges(seedBase []string, prop, action identifier.Identifier, user, note string, startOperation int64) (document.Changes, int64) {
	operation := startOperation + 1
	confidence := document.HighConfidence
	permissionUser := internalCore.PermissionUserPropID
	permissionScope := internalCore.PermissionScopePropID

	claimBase := append(slices.Clone(seedBase), strconv.FormatInt(operation, 10))
	claimID := identifier.From(claimBase...)
	changes := make(document.Changes, 0, 3) //nolint:mnd
	changes = append(changes, document.AddClaimChange{
		Under: nil,
		ID:    claimID,
		Base:  claimBase,
		Patch: document.ReferenceClaimPatch{
			Confidence: &confidence,
			Prop:       &prop,
			To:         &action,
		},
	})
	operation++

	subBase := append(slices.Clone(seedBase), strconv.FormatInt(operation, 10))
	changes = append(changes, document.AddClaimChange{
		Under: &claimID,
		ID:    identifier.From(subBase...),
		Base:  subBase,
		Patch: document.IdentifierClaimPatch{
			Confidence: &confidence,
			Prop:       &permissionUser,
			Value:      user,
		},
	})
	operation++

	scopeBase := append(slices.Clone(seedBase), strconv.FormatInt(operation, 10))
	changes = append(changes, document.AddClaimChange{
		Under: &claimID,
		ID:    identifier.From(scopeBase...),
		Base:  scopeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &permissionScope,
			String:     auth.ScopeSelf,
		},
	})

	if note != "" {
		operation++
		description := internalCore.DescriptionPropID
		noteBase := append(slices.Clone(seedBase), strconv.FormatInt(operation, 10))
		changes = append(changes, document.AddClaimChange{
			Under: &claimID,
			ID:    identifier.From(noteBase...),
			Base:  noteBase,
			Patch: document.StringClaimPatch{
				Confidence: &confidence,
				Prop:       &description,
				String:     note,
			},
		})
	}

	return changes, operation
}

// seedBase returns the base from which the claim IDs of seeded claims are derived, together with the
// change's sequence number: the document base with the session appended, matching how clients derive
// claim IDs for their own changes, so the store's change validation accepts the seeded changes.
func seedBase(docBase []string, session identifier.Identifier) []string {
	base := slices.Clone(docBase)
	return append(base, "SESSION", session.String())
}

// DefaultCreatorActions are the permission actions the default create-session seeding grants to
// the creator of a document: with them the creator keeps full control over the document they
// create.
//
//nolint:gochecknoglobals
var DefaultCreatorActions = []identifier.Identifier{auth.ActionRead, auth.ActionReadHistoric, auth.ActionUpdate, auth.ActionDelete, auth.ActionUpdatePermissions}

// RequestedClaimsChanges returns the changes adding the initial claims requested through the create
// document API request: it decodes the (empty) request body and turns query string parameters
// (property=value, both resolved document IDs) into claims, with claim IDs derived for operations
// startOperation+1 onward. Only claims of the given scope-participating properties can be requested:
// such claims cannot be added by clients as ordinary changes without role-granted update permissions
// covering them, while at creation they are validated against the caller's create grants (see
// DocumentCreatePostAPI); claims of other properties can be added after creation by the client
// itself.
func RequestedClaimsChanges(
	req *http.Request, scopeProperties map[identifier.Identifier]bool, session identifier.Identifier, docBase []string, startOperation int64,
) (document.Changes, errors.E) {
	var ea emptyRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		return nil, errE
	}

	type seed struct {
		prop  identifier.Identifier
		value identifier.Identifier
	}
	seeds := []seed{}
	query := req.URL.Query()
	for _, key := range slices.Sorted(maps.Keys(query)) {
		prop, errE := identifier.MaybeString(key)
		if errE != nil {
			return nil, errors.WithMessage(errE, "invalid property in query string")
		}
		if !scopeProperties[prop] {
			// Only scope-participating properties are allowed in the query string. Claims with
			// other properties can be added after creation by the client without the help of the
			// backend.
			errE := errors.New("property does not participate in permission scopes")
			errors.Details(errE)["property"] = prop.String()
			return nil, errE
		}
		for _, value := range query[key] {
			valueID, errE := identifier.MaybeString(value)
			if errE != nil {
				return nil, errors.WithMessage(errE, "invalid value in query string")
			}
			seeds = append(seeds, seed{prop: prop, value: valueID})
		}
	}

	confidence := document.HighConfidence
	sb := seedBase(docBase, session)
	changes := document.Changes{}
	operation := startOperation + 1
	for _, sd := range seeds {
		claimBase := append(slices.Clone(sb), strconv.FormatInt(operation, 10))
		changes = append(changes, document.AddClaimChange{
			Under: nil,
			ID:    identifier.From(claimBase...),
			Base:  claimBase,
			Patch: document.ReferenceClaimPatch{
				Confidence: &confidence,
				Prop:       &sd.prop,
				To:         &sd.value,
			},
		})
		operation++
	}

	return changes, nil
}

// PermissionClaimsChanges returns the changes granting the user the actions on the document being
// created, through HAS_PERMISSION claims with the self scope, with claim IDs derived for operations
// startOperation+1 onward.
func PermissionClaimsChanges(
	user string, actions []identifier.Identifier, session identifier.Identifier, docBase []string, startOperation int64,
) document.Changes {
	sb := seedBase(docBase, session)
	changes := document.Changes{}
	for _, action := range actions {
		var claimChanges document.Changes
		claimChanges, startOperation = permissionClaimChanges(sb, internalCore.HasPermissionPropID, action, user, "", startOperation)
		changes = append(changes, claimChanges...)
	}
	return changes
}

// RequestedPermissionClaimsChanges returns the changes recording that the user requested the actions
// on the document, through a HAS_REQUESTED_PERMISSION claim with the self scope per action, with
// claim IDs derived for operations startOperation+1 onward. A non-empty note is added under every
// claim as a DESCRIPTION sub-claim, so whoever decides a request sees what the requester wrote,
// whichever of them they decide first. The requests carry the same claim shape as the grants of
// PermissionClaimsChanges, so approving one means replacing the request claim with a HAS_PERMISSION
// one, which drops the note with it.
func RequestedPermissionClaimsChanges(
	user string, actions []identifier.Identifier, note string, session identifier.Identifier, docBase []string, startOperation int64,
) document.Changes {
	sb := seedBase(docBase, session)
	changes := document.Changes{}
	for _, action := range actions {
		var claimChanges document.Changes
		claimChanges, startOperation = permissionClaimChanges(sb, internalCore.HasRequestedPermissionPropID, action, user, note, startOperation)
		changes = append(changes, claimChanges...)
	}
	return changes
}
