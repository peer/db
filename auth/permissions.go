package auth

import (
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/internal/shortcut"
)

// ErrAccessDenied is returned when the caller may not access an object: a permission check found that
// the caller does not hold the checked permission action, or a hook denied the access.
var ErrAccessDenied = errors.Base("access denied")

// RoleEveryone is a reserved role name under which sites can declare permissions which apply
// to every caller, authenticated or not. It is not a real role: tokens cannot claim it (empty
// role names are dropped at authentication time), it is never attached to the context, and it
// is not granted by mock sign-in.
//
// Keep in sync with src/auth/index.ts.
const RoleEveryone = ""

// Permission action codes. They are used as the action keys in role configuration and are the
// code claims of the corresponding PERMISSION_ACTIONS vocabulary values.
//
// Keep in sync with core/vocabularies.go.
const (
	// ActionCreateCode allows creating new objects.
	ActionCreateCode = "ACTION_CREATE"
	// ActionReadCode allows fetching objects.
	ActionReadCode = "ACTION_READ"
	// ActionReadBulkCode allows bulk reading of objects. It supplements the read action: bulk reading
	// fetches the objects through the ordinary read path, so the caller also has to be allowed to
	// read the objects themselves. Claim scopes are not supported: a bulk read is not about a
	// particular document.
	ActionReadBulkCode = "ACTION_READ_BULK"
	// ActionReadHistoricCode allows reading versions of an object at which the caller did not have
	// read access. It supplements the read action: a caller who is allowed to read the object (its
	// latest version) and holds this action on it can fetch any of its versions.
	ActionReadHistoricCode = "ACTION_READ_HISTORIC"
	// ActionUpdateCode allows updating existing objects.
	ActionUpdateCode = "ACTION_UPDATE"
	// ActionUpdatePermissionsCode allows managing document-level permissions of objects. It
	// supplements the update action: permission claims are changed through the ordinary edit path,
	// so the caller also has to be allowed to update the document itself.
	ActionUpdatePermissionsCode = "ACTION_UPDATE_PERMISSIONS"
	// ActionDeleteCode allows deleting objects.
	ActionDeleteCode = "ACTION_DELETE"
)

// Document IDs of the PERMISSION_ACTIONS vocabulary values, by their codes.
//
// Keep in sync with src/core/permissions.ts.
//
//nolint:gochecknoglobals
var (
	ActionCreate            = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionCreateCode)
	ActionRead              = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionReadCode)
	ActionReadBulk          = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionReadBulkCode)
	ActionReadHistoric      = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionReadHistoricCode)
	ActionUpdate            = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionUpdateCode)
	ActionUpdatePermissions = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionUpdatePermissionsCode)
	ActionDelete            = identifier.From(internalCore.Namespace, "PERMISSION_ACTIONS", ActionDeleteCode)
)

// Actions maps permission action codes to the document IDs of the PERMISSION_ACTIONS vocabulary values.
//
//nolint:gochecknoglobals
var Actions = map[string]identifier.Identifier{
	ActionCreateCode:            ActionCreate,
	ActionReadCode:              ActionRead,
	ActionReadBulkCode:          ActionReadBulk,
	ActionReadHistoricCode:      ActionReadHistoric,
	ActionUpdateCode:            ActionUpdate,
	ActionUpdatePermissionsCode: ActionUpdatePermissions,
	ActionDeleteCode:            ActionDelete,
}

// IsAction reports whether the identifier is one of the permission action document IDs (see Actions).
func IsAction(id identifier.Identifier) bool {
	for _, action := range Actions {
		if action == id {
			return true
		}
	}
	return false
}

// actionRequirements are, per permission action, the actions it directly requires: an action is
// meaningful only together with them, because it builds on what they allow. Reading is the base of
// every access to an object, and managing permissions goes through the ordinary edit path, so it
// requires updating. The create action requires nothing: it is about objects which do not exist yet.
//
// Requirements are about the actions alone and say nothing about where a caller holds them from, so
// they are checked where a set of actions is chosen (an access request, the permissions form) and
// not against a document's own claims: a document can legitimately grant only the update action to
// a user who reads it through role grants.
//
// Keep in sync with permissionActions in src/permissions.ts.
//
//nolint:gochecknoglobals
var actionRequirements = map[identifier.Identifier][]identifier.Identifier{
	ActionReadBulk:          {ActionRead},
	ActionReadHistoric:      {ActionRead},
	ActionUpdate:            {ActionRead},
	ActionUpdatePermissions: {ActionUpdate},
	ActionDelete:            {ActionRead},
}

// ActionRequirements returns the actions the given action directly requires (see actionRequirements),
// and nothing for an action which requires nothing or is not a permission action at all.
func ActionRequirements(action identifier.Identifier) []identifier.Identifier {
	return slices.Clone(actionRequirements[action])
}

// ActionsClosure returns the actions together with everything they require, transitively: the given
// actions come first, in their order and without duplicates, followed by the added requirements.
func ActionsClosure(actions []identifier.Identifier) []identifier.Identifier {
	closure := make([]identifier.Identifier, 0, len(actions))
	pending := slices.Clone(actions)
	for len(pending) > 0 {
		action := pending[0]
		pending = pending[1:]
		if slices.Contains(closure, action) {
			continue
		}
		closure = append(closure, action)
		pending = append(pending, actionRequirements[action]...)
	}
	return closure
}

// ActionsRequiring returns the actions which require the given action, directly or through another
// action, transitively and without duplicates, in no particular order. Giving up an action means
// giving up these as well, because they build on it.
func ActionsRequiring(action identifier.Identifier) []identifier.Identifier {
	requiring := []identifier.Identifier{}
	pending := []identifier.Identifier{action}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for other, required := range actionRequirements {
			if !slices.Contains(required, current) || slices.Contains(requiring, other) {
				continue
			}
			requiring = append(requiring, other)
			pending = append(pending, other)
		}
	}
	return requiring
}

// ValidateActions returns an error when the actions are not closed under their requirements, that is
// when an action among them requires an action which is not. The missing actions are attached to the
// error as details.
func ValidateActions(actions []identifier.Identifier) errors.E {
	missing := []string{}
	for _, action := range actions {
		for _, required := range actionRequirements[action] {
			if !slices.Contains(actions, required) && !slices.Contains(missing, required.String()) {
				missing = append(missing, required.String())
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	errE := errors.New("permission actions are missing actions they require")
	slices.Sort(missing)
	errors.Details(errE)["missing"] = missing
	return errE
}

// Literal permission scopes. Besides them, a scope entry in role configuration can be a search shortcut
// entry of the form "property=value" (each side a single identifier token, e.g. "core.peerdb.org,INSTANCE_OF=example.org,ITEM"),
// which scopes the action to documents carrying a reference claim with that property and value.
//
// Keep in sync with src/auth/index.ts.
const (
	// ScopeAll scopes an action to all objects (documents and files). It is valid only in role
	// configuration.
	ScopeAll = "all"
	// ScopeFiles scopes an action to all files. It is valid only in role configuration.
	ScopeFiles = "files"
	// ScopeDocuments scopes an action to all documents. It is valid only in role configuration.
	ScopeDocuments = "documents"
	// ScopeSelf scopes an action to the document carrying the permission claim. It is valid only in
	// document-level permission claims.
	ScopeSelf = "self"
)

// Scope is one parsed entry of a permission scope expression. Either Literal holds one of the literal
// scopes, or Prop and Value hold a claim scope: the scope then matches a document carrying a reference
// claim with that property and value.
type Scope struct {
	Literal string
	Prop    identifier.Identifier
	Value   identifier.Identifier
}

// String returns the scope in the scope expression grammar, with identifiers in their resolved form.
func (s Scope) String() string {
	if s.Literal != "" {
		return s.Literal
	}
	return s.Prop.String() + shortcut.KeyValueSeparator + s.Value.String()
}

// MarshalJSON marshals the scope as its resolved string form, which ParseScopes parses back.
func (s Scope) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// CoversDocuments reports whether the scope can cover any document at all. It is the optimistic
// document check used when no concrete document is available, with the definitive check through
// scopesAllowDocument following once one is.
func (s Scope) CoversDocuments() bool {
	switch s.Literal {
	case ScopeAll, ScopeDocuments:
		// The all and documents scopes cover every document.
		return true
	case ScopeSelf:
		// The self scope never appears in role grants.
		errE := errors.New("unexpected scope")
		errors.Details(errE)["scope"] = s.Literal
		panic(errE)
	case ScopeFiles:
		// The files scope covers no document.
		return false
	case "":
		// A claim scope covers the documents carrying the matching reference claim.
		// We return true here and expect that the definitive scopesAllowDocument check follows
		// this call once the document is available.
		return true
	default:
		errE := errors.New("unknown scope")
		errors.Details(errE)["scope"] = s.Literal
		panic(errE)
	}
}

// MatchesFiles reports whether the scope covers files.
func (s Scope) MatchesFiles() bool {
	switch s.Literal {
	case ScopeAll, ScopeFiles:
		return true
	case ScopeDocuments:
		return false
	case ScopeSelf:
		// The self scope never appears in role grants.
		errE := errors.New("unexpected scope")
		errors.Details(errE)["scope"] = s.Literal
		panic(errE)
	case "":
		// A claim scope covers only documents.
		return false
	default:
		errE := errors.New("unknown scope")
		errors.Details(errE)["scope"] = s.Literal
		panic(errE)
	}
}

// ParseScopes parses a permission scope expression: "&"-separated entries, each either a literal scope
// (all, files, documents, or self) or a search shortcut entry of the form "property=value" with each side
// a single identifier token.
func ParseScopes(expression string) ([]Scope, errors.E) {
	var scopes []Scope
	for term := range strings.SplitSeq(expression, shortcut.EntrySeparator) {
		switch term {
		case ScopeAll, ScopeFiles, ScopeDocuments, ScopeSelf:
			scopes = append(scopes, Scope{Literal: term, Prop: identifier.Identifier{}, Value: identifier.Identifier{}})
			continue
		}
		entry, errE := shortcut.ParseEntry(term)
		if errE != nil {
			return nil, errE
		}
		if len(entry.Key) != 1 || !entry.Key[0].IsIdentifier() || len(entry.Value) != 1 || !entry.Value[0].IsIdentifier() {
			errE := errors.New("scope entry must be a property and a value, each a single identifier")
			errors.Details(errE)["scope"] = term
			return nil, errE
		}
		scopes = append(scopes, Scope{Literal: "", Prop: entry.Key[0].Identifier(), Value: entry.Value[0].Identifier()})
	}
	return scopes, nil
}

// scopesAllowDocument reports whether the scopes, evaluated together, allow an action on the given
// document. The all and documents scopes allow every document on their own. Claim scopes have to be
// fully satisfied by the document: for every property among them which the document carries, every
// value the document carries has to be granted (so a document with multiple INSTANCE_OF claims
// requires all of its classes to be granted), and at least one such property has to be present on
// the document. The files scope allows no document, and the self scope never appears here: it is
// valid only in document-level permission claims, which are evaluated by PermissionClaimGrants, not
// as scopes.
func scopesAllowDocument(scopes []Scope, doc *document.D) bool {
	values := map[identifier.Identifier]map[identifier.Identifier]bool{}
	for _, scope := range scopes {
		switch scope.Literal {
		case ScopeAll, ScopeDocuments:
			return true
		case ScopeSelf:
			// The self scope never appears in role grants (ParseRoleGrants rejects it).
			errE := errors.New("unexpected scope")
			errors.Details(errE)["scope"] = scope.Literal
			panic(errE)
		case ScopeFiles:
			continue
		case "":
			if values[scope.Prop] == nil {
				values[scope.Prop] = map[identifier.Identifier]bool{}
			}
			values[scope.Prop][scope.Value] = true
		default:
			errE := errors.New("unknown scope")
			errors.Details(errE)["scope"] = scope.Literal
			panic(errE)
		}
	}

	present := false
	for prop, granted := range values {
		claims := document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, prop, document.LowConfidence)
		if len(claims) == 0 {
			continue
		}
		for _, claim := range claims {
			if !granted[claim.To.ID] {
				return false
			}
		}
		present = true
	}
	return present
}

// RoleGrants maps permission actions (by the document IDs of the PERMISSION_ACTIONS vocabulary values) to the
// scopes they are granted with. In YAML configuration, grants are written as a map of permission action
// codes to lists of permission scope expressions (see ParseRoleGrants); they marshal to JSON in their
// resolved form (action document IDs and resolved scope strings), so JSON consumers do not need to parse
// expressions.
//
//nolint:recvcheck
type RoleGrants map[identifier.Identifier][]Scope

// UnmarshalYAML parses a map of permission action codes to lists of permission scope expressions and
// resolves it with ParseRoleGrants.
func (g *RoleGrants) UnmarshalYAML(data []byte) error {
	var actions map[string][]string
	err := yaml.Unmarshal(data, &actions)
	if err != nil {
		return errors.WithStack(err)
	}
	grants, errE := ParseRoleGrants(actions)
	if errE != nil {
		return errE
	}
	*g = grants
	return nil
}

// ParseRoleGrants validates and resolves one role's action configuration (a map from action codes to
// permission scope expressions) into RoleGrants. The self scope is rejected (it is valid only in
// document-level permission claims) and the bulk read action supports only the all scope.
func ParseRoleGrants(actions map[string][]string) (RoleGrants, errors.E) {
	grants := RoleGrants{}
	for code, expressions := range actions {
		action, ok := Actions[code]
		if !ok {
			errE := errors.New("unknown permission action")
			errors.Details(errE)["action"] = code
			return nil, errE
		}
		var scopes []Scope
		for _, expression := range expressions {
			parsed, errE := ParseScopes(expression)
			if errE != nil {
				errors.Details(errE)["action"] = code
				return nil, errE
			}
			scopes = append(scopes, parsed...)
		}
		for _, scope := range scopes {
			if scope.Literal == ScopeSelf {
				errE := errors.New("the self scope is valid only in document-level permission claims")
				errors.Details(errE)["action"] = code
				return nil, errE
			}
			if action == ActionReadBulk && scope.Literal == "" {
				errE := errors.New("the bulk read action supports only literal scopes")
				errors.Details(errE)["action"] = code
				errors.Details(errE)["scope"] = scope.String()
				return nil, errE
			}
		}
		grants[action] = scopes
	}
	return grants, nil
}

// MustParseRoleGrants is like ParseRoleGrants but panics on error. It is intended for role
// configurations fixed in code.
func MustParseRoleGrants(actions map[string][]string) RoleGrants {
	grants, errE := ParseRoleGrants(actions)
	if errE != nil {
		panic(errE)
	}
	return grants
}

// PermissionClaimGrants returns, per permission action, the users the document's own permission
// claims grant that action. A HAS_PERMISSION claim grants the action it references to the users its
// PERMISSION_USER sub-claims name when one of its PERMISSION_SCOPE sub-claims carries the self
// scope, the only scope valid in document-level permission claims (other scopes are ignored). The
// claim and both sub-claims count only at or above low confidence, and an empty user is ignored
// (permission claims always name a user). The create action is never granted: a document's own
// claims cannot grant creating it, because when they are evaluated during creation they were
// written by the creating session itself and must not self-authorize it, and afterwards the
// document already exists. Users are unique and sorted per action.
func PermissionClaimGrants(doc *document.D) map[identifier.Identifier][]string {
	return permissionClaimGrants(internalCore.HasPermissionPropID, doc)
}

// RequestedPermissionClaimGrants returns, per permission action, the users the document's own
// permission claims record as having requested that action: HAS_REQUESTED_PERMISSION claims carry
// the same shape as the HAS_PERMISSION grants of PermissionClaimGrants (the requested action as the
// value, PERMISSION_USER sub-claims naming the requesting users, and a PERMISSION_SCOPE sub-claim
// with the self scope) and are evaluated by the same rules. A request grants nothing: users with
// the permissions action decide it by replacing the request claim with a grant, or removing it.
func RequestedPermissionClaimGrants(doc *document.D) map[identifier.Identifier][]string {
	return permissionClaimGrants(internalCore.HasRequestedPermissionPropID, doc)
}

// permissionClaims iterates the document's own permission claims of the given property (HAS_PERMISSION
// or HAS_REQUESTED_PERMISSION) which count at document level, yielding each together with the action
// it references: the claim counts at or above low confidence, and a claim not scoped to the document
// carrying it is skipped (see PermissionClaimGrants for the rules).
func permissionClaims(prop identifier.Identifier, doc *document.D) iter.Seq2[*document.ReferenceClaim, identifier.Identifier] {
	return func(yield func(*document.ReferenceClaim, identifier.Identifier) bool) {
		for _, claim := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, prop, document.LowConfidence) {
			if !hasSelfScope(claim) {
				continue
			}
			if !yield(claim, claim.To.ID) {
				return
			}
		}
	}
}

// permissionClaimUsers iterates the users a permission claim names through its PERMISSION_USER
// sub-claims at or above low confidence, skipping an empty one: permission claims always name a user.
func permissionClaimUsers(claim *document.ReferenceClaim) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, sub := range document.GetClaimsOfTypeWithConfidence[document.IdentifierClaim](claim, internalCore.PermissionUserPropID, document.LowConfidence) {
			if sub.Value == "" {
				continue
			}
			if !yield(sub.Value) {
				return
			}
		}
	}
}

// permissionClaimGrants evaluates the document's own permission claims of the given property
// (HAS_PERMISSION or HAS_REQUESTED_PERMISSION) into a map of actions to the users the claims name
// (see PermissionClaimGrants for the rules).
func permissionClaimGrants(prop identifier.Identifier, doc *document.D) map[identifier.Identifier][]string {
	users := map[identifier.Identifier]map[string]bool{}
	for claim, action := range permissionClaims(prop, doc) {
		// A document's own claims never grant creating it, so a claim of the create action grants
		// nobody anything.
		if action == ActionCreate {
			continue
		}
		for user := range permissionClaimUsers(claim) {
			if users[action] == nil {
				users[action] = map[string]bool{}
			}
			users[action][user] = true
		}
	}
	grants := make(map[identifier.Identifier][]string, len(users))
	for action, us := range users {
		grants[action] = slices.Sorted(maps.Keys(us))
	}
	return grants
}

// hasSelfScope reports whether the permission claim is scoped to the document carrying it, the only
// scope which counts at document level. Other scopes are not allowed there but are ignored.
func hasSelfScope(claim *document.ReferenceClaim) bool {
	for _, sub := range document.GetClaimsOfTypeWithConfidence[document.StringClaim](claim, internalCore.PermissionScopePropID, document.LowConfidence) {
		parsed, errE := ParseScopes(sub.String)
		if errE != nil {
			continue
		}
		for _, scope := range parsed {
			if scope.Literal == ScopeSelf {
				return true
			}
		}
	}
	return false
}

// RequestedPermissionClaims returns the IDs of the document's claims which record the subject as
// having requested one of the actions, by the rules of RequestedPermissionClaimGrants (the claim
// scoped to the document itself and naming the subject). An empty subject matches nothing: permission
// claims always name a user.
func RequestedPermissionClaims(actions []identifier.Identifier, subject string, doc *document.D) []identifier.Identifier {
	if subject == "" {
		return nil
	}
	claims := []identifier.Identifier{}
	for claim, action := range permissionClaims(internalCore.HasRequestedPermissionPropID, doc) {
		if !slices.Contains(actions, action) {
			continue
		}
		for user := range permissionClaimUsers(claim) {
			if user == subject {
				claims = append(claims, claim.ID)
				break
			}
		}
	}
	return claims
}

// HasPermissionClaim reports whether the document's own permission claims grant the subject the action
// (see PermissionClaimGrants).
func HasPermissionClaim(action identifier.Identifier, subject string, doc *document.D) bool {
	return slices.Contains(PermissionClaimGrants(doc)[action], subject)
}

// HasRequestedPermissionClaim reports whether the document's own permission claims record the subject
// as having requested the action (see RequestedPermissionClaimGrants).
func HasRequestedPermissionClaim(action identifier.Identifier, subject string, doc *document.D) bool {
	return slices.Contains(RequestedPermissionClaimGrants(doc)[action], subject)
}

// AllowsDocument reports whether the grants allow the action on the given document: whether the
// scopes granted for the action allow it (see scopesAllowDocument for how the document has to fully
// satisfy claim scopes). With a nil document it reports whether the grants allow the action on
// documents at all (any scope which can cover a document counts). The document's own permission
// claims are not consulted: they grant actions to named users independently of any role (see
// PermissionClaimGrants), and HasDocumentPermission combines both.
func (g RoleGrants) AllowsDocument(action identifier.Identifier, doc *document.D) bool {
	if doc == nil {
		for _, scope := range g[action] {
			if scope.CoversDocuments() {
				return true
			}
		}
		return false
	}

	return scopesAllowDocument(g[action], doc)
}

// AllowsFiles reports whether the grants allow the action on files.
func (g RoleGrants) AllowsFiles(action identifier.Identifier) bool {
	for _, scope := range g[action] {
		if scope.MatchesFiles() {
			return true
		}
	}
	return false
}

// HasDocumentPermission reports whether the subject with the given roles holds the permission action
// on the document, through either of two independent arms: the role arm (a grant of the reserved
// everyone role or of one of the roles allows the document, see RoleGrants.AllowsDocument) or, when doc
// is non-nil, the claim arm (the document's own permission claims grant the subject the action, see
// PermissionClaimGrants). Indexing materializes exactly these two arms for every document (the roles
// whose grants allow it and the users its claims grant), so the default search query filter admits
// precisely the documents this reports permitted. With a nil doc only role grants are checked,
// against documents in general.
func HasDocumentPermission(grants map[string]RoleGrants, action identifier.Identifier, subject string, roles []string, doc *document.D) bool {
	if grants[RoleEveryone].AllowsDocument(action, doc) {
		return true
	}
	for _, role := range roles {
		if grants[role].AllowsDocument(action, doc) {
			return true
		}
	}
	return doc != nil && HasPermissionClaim(action, subject, doc)
}

// HasFilePermission reports whether a caller with the given roles holds the permission action on
// files under the given role grants: through a grant of the reserved everyone role or one of the
// roles with a scope covering files.
func HasFilePermission(grants map[string]RoleGrants, action identifier.Identifier, roles []string) bool {
	if grants[RoleEveryone].AllowsFiles(action) {
		return true
	}
	for _, role := range roles {
		if grants[role].AllowsFiles(action) {
			return true
		}
	}
	return false
}

// ScopeProperties returns the set of properties participating in claim scopes across all grants of
// all given roles and all actions. Claims of these properties determine which documents role grants
// cover, so changing them is a role-level operation: document-level permission claims granting the
// update action do not allow changing them.
func ScopeProperties(roleGrants map[string]RoleGrants) map[identifier.Identifier]bool {
	properties := map[identifier.Identifier]bool{}
	for _, grants := range roleGrants {
		for _, scopes := range grants {
			for _, scope := range scopes {
				if scope.Literal == "" {
					properties[scope.Prop] = true
				}
			}
		}
	}
	return properties
}

// ActionScopeProperties returns the set of properties participating in claim scopes of the grants of
// the action a caller with the given roles holds: the grants of the reserved everyone role and of each
// of their roles. It is what ScopeProperties answers for the whole site, narrowed to one action and one
// caller, so it says which claims decide whether that caller's grants of the action cover a document.
//
// It is empty for a caller whose grants of the action use no claim scope, which is both a caller holding
// the action on documents in general and a caller not holding it at all.
func ActionScopeProperties(roleGrants map[string]RoleGrants, action identifier.Identifier, roles []string) map[identifier.Identifier]bool {
	properties := map[identifier.Identifier]bool{}
	collect := func(role string) {
		for _, scope := range roleGrants[role][action] {
			if scope.Literal == "" {
				properties[scope.Prop] = true
			}
		}
	}
	collect(RoleEveryone)
	for _, role := range roles {
		collect(role)
	}
	return properties
}
