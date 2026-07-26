package auth

import (
	"slices"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gopkg.in/yaml.v3"

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
// document. The all and documents scopes allow every document on their own, and so does the self scope
// (it comes only from the document's own permission claims, extracted by PermissionClaimScopes, and
// covers the document carrying them). Claim scopes have to be fully satisfied by the document: for
// every property among them which the document carries, every value the document carries has to be
// granted (so a document with multiple instance_of claims requires all of its classes to be granted),
// and at least one such property has to be present on the document. The files scope allows no document.
func scopesAllowDocument(scopes []Scope, doc *document.D) bool {
	values := map[identifier.Identifier]map[identifier.Identifier]bool{}
	for _, scope := range scopes {
		switch scope.Literal {
		case ScopeAll, ScopeDocuments, ScopeSelf:
			return true
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

// Grants maps permission actions (by the document IDs of the PERMISSION_ACTIONS vocabulary values) to the
// scopes they are granted with. In YAML configuration, grants are written as a map of permission action
// codes to lists of permission scope expressions (see ParseRoleGrants); they marshal to JSON in their
// resolved form (action document IDs and resolved scope strings), so JSON consumers do not need to parse
// expressions.
//
//nolint:recvcheck
type Grants map[identifier.Identifier][]Scope

// UnmarshalYAML implements yaml.Unmarshaler for Grants: it parses a map of permission action codes to
// lists of permission scope expressions and resolves it with ParseRoleGrants.
func (g *Grants) UnmarshalYAML(value *yaml.Node) error {
	var actions map[string][]string
	err := value.Decode(&actions)
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
// permission scope expressions) into Grants. The self scope is rejected (it is valid only in
// document-level permission claims) and the bulk read action supports only the all scope.
func ParseRoleGrants(actions map[string][]string) (Grants, errors.E) {
	grants := Grants{}
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
func MustParseRoleGrants(actions map[string][]string) Grants {
	grants, errE := ParseRoleGrants(actions)
	if errE != nil {
		panic(errE)
	}
	return grants
}

// PermissionClaimScopes returns the scopes of the document's own permission claims granting the user
// the action: for every HAS_PERMISSION claim with the action as the value and a PERMISSION_USER
// sub-claim with the user, the scopes of its PERMISSION_SCOPE sub-claims which are valid in
// document-level permission claims (currently only the self scope). Both sub-claims are required, so a
// permission claim without them (or with only invalid scopes) contributes nothing, and an empty user
// (an anonymous caller) has no scopes, because permission claims always name a user. The create action
// never has claim scopes: a document's own claims cannot grant creating it, because when they are
// evaluated during creation they were written by the creating session itself and must not
// self-authorize it, and afterwards the document already exists.
func PermissionClaimScopes(action identifier.Identifier, user string, doc *document.D) []Scope {
	if user == "" || action == ActionCreate {
		return nil
	}
	var scopes []Scope
	for _, claim := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, internalCore.HasPermissionPropID, document.LowConfidence) {
		if claim.To.ID != action {
			continue
		}
		userMatches := false
		for _, sub := range document.GetClaimsOfTypeWithConfidence[document.IdentifierClaim](claim, internalCore.PermissionUserPropID, document.LowConfidence) {
			if sub.Value == user {
				userMatches = true
				break
			}
		}
		if !userMatches {
			continue
		}
		for _, sub := range document.GetClaimsOfTypeWithConfidence[document.StringClaim](claim, internalCore.PermissionScopePropID, document.LowConfidence) {
			parsed, errE := ParseScopes(sub.String)
			if errE != nil {
				continue
			}
			for _, scope := range parsed {
				// Other scopes are not allowed at document-level permissions, but we ignore them.
				if scope.Literal == ScopeSelf {
					scopes = append(scopes, scope)
				}
			}
		}
	}
	return scopes
}

// HasPermissionClaim reports whether the document's own permission claims grant the user the action:
// whether the scopes extracted from them by PermissionClaimScopes allow the document.
func HasPermissionClaim(action identifier.Identifier, user string, doc *document.D) bool {
	return scopesAllowDocument(PermissionClaimScopes(action, user, doc), doc)
}

// AllowsDocument reports whether the grants allow the user the action on the given document: the
// scopes granted for the action and the scopes extracted from the document's own permission claims
// granting the user the action (see PermissionClaimScopes) together form one list which has to allow
// the document (see scopesAllowDocument for how the document has to fully satisfy claim scopes). With
// a nil document it reports whether the grants allow the action on documents at all (any scope which
// can cover a document counts); there are then no document claims to consult.
func (g Grants) AllowsDocument(action identifier.Identifier, user string, doc *document.D) bool {
	if doc == nil {
		for _, scope := range g[action] {
			if scope.CoversDocuments() {
				return true
			}
		}
		return false
	}

	return scopesAllowDocument(slices.Concat(g[action], PermissionClaimScopes(action, user, doc)), doc)
}

// AllowsFiles reports whether the grants allow the action on files.
func (g Grants) AllowsFiles(action identifier.Identifier) bool {
	for _, scope := range g[action] {
		if scope.MatchesFiles() {
			return true
		}
	}
	return false
}

// HasDocumentPermission reports whether the subject with the given roles holds the permission action
// on the document under the given role grants: through a grant of the reserved everyone role or one
// of the roles with a scope covering the document, or, when doc is non-nil, through the document's
// own permission claims. With a nil doc only role grants are checked, against documents in general.
func HasDocumentPermission(grants map[string]Grants, action identifier.Identifier, subject string, roles []string, doc *document.D) bool {
	if grants[RoleEveryone].AllowsDocument(action, subject, doc) {
		return true
	}
	for _, role := range roles {
		if grants[role].AllowsDocument(action, subject, doc) {
			return true
		}
	}
	return false
}

// HasFilePermission reports whether a caller with the given roles holds the permission action on
// files under the given role grants: through a grant of the reserved everyone role or one of the
// roles with a scope covering files.
func HasFilePermission(grants map[string]Grants, action identifier.Identifier, roles []string) bool {
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
func ScopeProperties(roleGrants map[string]Grants) map[identifier.Identifier]bool {
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
