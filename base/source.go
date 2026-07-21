package base

import (
	"context"
	"slices"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// DefaultIndexingSourceCheck returns the default indexing source check for the given visibility levels
// and role grants: role grants must make the document readable by every searcher of the level.
//
// A searcher of a level holds at least one of the level's roles, except at a no-roles first level (the
// floor), which anonymous requests and holders of roles assigned to no level resolve to. Grants are
// additive across roles, so it is enough to check the minimal searchers: each single role of the level,
// or the anonymous searcher (the everyone grants alone) at a no-roles floor. Document-level permission
// claims are not consulted: they grant actions to specific subjects and never take access away, so they
// cannot make a document readable by everyone, nor stop it from being so. The check is exact: it denies
// precisely the documents which some searcher of the level may not read through role grants.
//
// The indexing hooks need no checking here: they run with only the level's visibility in ctx and no
// caller identity, so their output at a level is one shared rendering which cannot differ between the
// level's searchers. What this check cannot see is whether the read-path document hooks agree with the
// indexing hooks: a site whose read path hides more than its indexing hooks do leaks through the
// documents' own entries, which no source check can prevent.
//
// When logWarning is set, a denial is logged as a warning: on a site whose role grants make every
// level's documents readable by that level's searchers the check should never deny, so a denial
// signals misconfigured grants or visibility levels. Otherwise denials are logged at the debug level:
// on a site with read-restricted documents they are the expected outcome.
//
// A site whose rules this default does not capture can replace it, and a site whose role grants alone
// already guarantee that every level's documents are readable by the level's searchers can set the
// check to nil, taking over the invariant documented on B.IndexingSourceCheck itself.
func DefaultIndexingSourceCheck(
	visibility []auth.VisibilityLevel, grants map[string]auth.Grants, logWarning bool,
) func(ctx context.Context, doc *document.D, metadata *store.DocumentMetadata) errors.E {
	// Precompute per level the minimal searcher audiences. A request resolves to the highest level among
	// its roles, with a no-roles first level as the floor for requests (including anonymous ones) whose
	// roles match no level, so a searcher of a level holds a role of that level, except at a no-roles
	// floor, where the anonymous searcher is minimal (holders of roles assigned to no level resolve to
	// the floor too, but their grants only add to the everyone grants). A no-roles last level is
	// resolved to by no request at all (it exists as the unfiltered superset for internal paths), so it
	// gets no audiences and every document is a source there.
	readAudiences := map[string][][]string{}
	for i, level := range visibility {
		if i == 0 && len(level.Roles) == 0 {
			readAudiences[level.Name] = [][]string{nil}
			continue
		}
		levelRoles := slices.Clone(level.Roles)
		slices.Sort(levelRoles)
		audiences := make([][]string, 0, len(levelRoles))
		for _, role := range levelRoles {
			audiences = append(audiences, []string{role})
		}
		readAudiences[level.Name] = audiences
	}

	return func(ctx context.Context, doc *document.D, _ *store.DocumentMetadata) errors.E {
		level := auth.Visibility(ctx)
		audiences, ok := readAudiences[level]
		if !ok {
			// The check is called only with a configured level's visibility in ctx (every configured
			// level is a key, a no-roles level with an empty audience list), so a missing level
			// (including an unset one) is a programming error or a mismatch between the levels indexed
			// and the levels configured: fail indexing loudly.
			errE := errors.New("unknown visibility level")
			errors.Details(errE)["level"] = level
			return errE
		}
		// Every searcher of the level must be able to read the document through role grants. The subject
		// is empty so document-level permission claims (which grant to specific subjects) match nothing.
		for _, audience := range audiences {
			if !auth.HasDocumentPermission(grants, auth.ActionRead, "", audience, doc) {
				errE := errors.Errorf("%w: not readable by every searcher of the level", auth.ErrAccessDenied)
				errors.Details(errE)["document"] = doc.ID.String()
				logLevel := zerolog.DebugLevel
				if logWarning {
					logLevel = zerolog.WarnLevel
				}
				ev := zerolog.Ctx(ctx).WithLevel(logLevel).Str("document", doc.ID.String())
				if len(audience) > 0 {
					errors.Details(errE)["role"] = audience[0]
					ev = ev.Str("role", audience[0])
				}
				ev.Msg("document is not readable by every searcher of the level")
				return errE
			}
		}
		return nil
	}
}
