package base

import (
	"context"
	"slices"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/auth"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// DefaultSearchQueryHook is the default search query hook (see SearchQueryHook) enforcing document
// read permissions: a caller sees in search (results, facets, and counts) exactly the documents
// auth.HasDocumentPermission reports them permitted to read, mirroring its two arms: the filter
// matches the caller's roles against the roles indexing recorded from the role arm (readableByRoles)
// and the caller's subject against the users it recorded from the claim arm (readableByUsers).
//
// Both arms are evaluated against the document at indexing time from Roles, with the same auth
// functions the read path runs, so the returned filter only matches the caller's roles and subject
// against the indexed outcome (role grant changes therefore require a full reindex). A caller whose
// roles grant reading all documents gets no restriction (a nil query), so a site whose grants make
// everything readable gets unrestricted, cacheable searches.
func (b *B) DefaultSearchQueryHook(ctx context.Context) (types.QueryVariant, errors.E) { //nolint:ireturn
	roles := auth.Roles(ctx)
	// Claim scopes never cover every document, so only the literal all and documents scopes make a
	// role read-unrestricted.
	readsAll := func(role string) bool {
		for _, scope := range b.Roles[role][auth.ActionRead] {
			if scope.Literal == auth.ScopeAll || scope.Literal == auth.ScopeDocuments {
				return true
			}
		}
		return false
	}
	if readsAll(auth.RoleEveryone) || slices.ContainsFunc(roles, readsAll) {
		return nil, nil //nolint:nilnil
	}
	subject, _ := auth.Subject(ctx)
	// The reserved everyone role applies to every caller, so it is always among the matched roles.
	return internalSearch.ReadAccessQuery(append(slices.Clone(roles), auth.RoleEveryone), subject), nil
}
