// Package site defines the per-site configuration and runtime state for a
// PeerDB site.
package site

import (
	"context"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
	"gopkg.in/yaml.v3"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/store"
)

// AllVisibilityLevel is the default name for the top (highest) visibility level: the unfiltered superset
// that sees all documents. A site that does not configure Visibility indexes into a single level with this name.
const AllVisibilityLevel = "all"

// Build contains version and build metadata.
type Build struct {
	Version        string `json:"version,omitempty"`
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
	Revision       string `json:"revision,omitempty"`
}

// SiteFeatures contains enabled feature flags.
//
//nolint:revive
type SiteFeatures struct {
	SearchResultsTable bool `json:"searchResultsTable,omitempty" yaml:"searchResultsTable,omitempty"`
	DownloadButtons    bool `json:"downloadButtons,omitempty"    yaml:"downloadButtons,omitempty"`

	// IndexAncestorProperties enables claim propagation to transitive super-properties
	// when indexing: a claim for property X is also indexed for every ancestor of X
	// via SUBPROPERTY_OF. Disabled by default. Backend-only; not exposed to the frontend.
	IndexAncestorProperties bool `json:"-" yaml:"indexAncestorProperties,omitempty"`
}

// SiteAuthConfig contains per-site configuration for OIDC-based authentication.
// Each site has its own client because the redirect URI is per-domain and
// most OIDC providers register redirect URIs as fixed strings rather than
// templates.
//
//nolint:revive
type SiteAuthConfig struct {
	Issuer       string `json:"-" yaml:"issuer,omitempty"`
	ClientID     string `json:"-" yaml:"clientId,omitempty"`
	ClientSecret string `json:"-" yaml:"clientSecret,omitempty"`
}

// Site represents a single site in the PeerDB application with its configuration and state.
type Site struct {
	waf.Site `yaml:",inline"`

	Build *Build `json:"build,omitempty" yaml:"-"`

	IndexPrefix string `json:"-"                     yaml:"indexPrefix,omitempty"`
	Schema      string `json:"-"                     yaml:"schema,omitempty"`
	Title       string `json:"title,omitempty"       yaml:"title,omitempty"`
	Logo        string `json:"logo,omitempty"        yaml:"logo,omitempty"`
	LogoCompact string `json:"logoCompact,omitempty" yaml:"logoCompact,omitempty"`

	LanguagePriority map[string][]string `json:"languagePriority,omitempty" yaml:"languagePriority,omitempty"`
	DefaultLanguage  string              `json:"defaultLanguage,omitempty"  yaml:"defaultLanguage,omitempty"`

	// TODO: How to keep LanguageCodes in sync, if they are added or removed after initialization?
	LanguageCodes map[identifier.Identifier]string `json:"languageCodes,omitempty" yaml:"-"`

	Features SiteFeatures `json:"features" yaml:"features"`

	// Roles is a map of role names to permissions. Its keys also act as
	// the allowlist of roles a token may bind to a request: any role a
	// token claims that is not a key here is dropped at authentication
	// time so it cannot leak into auth.Roles or the Roles response header.
	Roles map[string][]string `json:"roles,omitempty" yaml:"roles,omitempty"`

	// Visibility is the ordered list of visibility levels, from the lowest
	// (least access) to the highest (most access). The order lets a request
	// whose roles map to several levels resolve to a single level: the
	// highest level among the request's roles applies. Every role listed
	// must be a key in Roles and must appear in at most one level, and level
	// names must be unique and non-empty. A role in no level, a level with
	// no roles, and an empty Visibility are all allowed.
	//
	// When non-empty, the bridge indexes each level into its own index and uses
	// the highest (last) level as the visibility-independent superset (for example
	// for inverse-relation accumulation), so that level must grant access to all
	// documents without any filtering: the site's document post-hooks must not drop
	// anything at the highest level. If the highest role-based level still filters,
	// add a no-roles level (e.g. {Name: "all", Roles: nil}) on top: no role resolves
	// to it, but it defines the unfiltered superset. An empty Visibility is the
	// degenerate case of a single such level: one unfiltered index for everyone.
	Visibility []auth.VisibilityLevel `json:"visibility,omitempty" yaml:"visibility,omitempty"`

	// Auth carries per-site OIDC configuration. When all three fields
	// (issuer, clientId, clientSecret) are set the site uses OIDC for
	// sign-in. Otherwise the site falls back to the MockAuthenticator
	// (intended for development).
	Auth SiteAuthConfig `json:"-" yaml:"auth,omitempty"`

	// MetadataHeaderPrefix mirrors waf.Service.MetadataHeaderPrefix so
	// that the frontend can compose the right header names when reading
	// the canonical Metadata header as well as the per-identity Roles /
	// UserInfo headers the auth middleware writes. It is the same string
	// for every site in a process.
	MetadataHeaderPrefix string `json:"metadataHeaderPrefix,omitempty" yaml:"-"`

	Base        *base.B                    `json:"-" yaml:"-"`
	DBPool      *pgxpool.Pool              `json:"-" yaml:"-"`
	ESClient    *elasticsearch.TypedClient `json:"-" yaml:"-"`
	RiverClient *river.Client[pgx.Tx]      `json:"-" yaml:"-"`

	// Authenticator drives sign-in (SignIn / Callback), sign-out (SignOut)
	// and request-time token validation (Authenticate) for this site.
	Authenticator auth.Authenticator

	// DebugRiverHandler is the River UI handler mounted at /debug/river.
	// Populated only in development mode.
	DebugRiverHandler http.Handler

	initialized bool
}

// Decode implements kong.MapperValue to decode Site from JSON/YAML configuration.
func (s *Site) Decode(ctx *kong.DecodeContext) error {
	var value string
	err := ctx.Scan.PopValueInto("value", &value)
	if err != nil {
		return errors.WithStack(err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(value))
	decoder.KnownFields(true)
	err = decoder.Decode(s) //nolint:musttag
	if err != nil {
		if yamlErr, ok := errors.AsType[*yaml.TypeError](err); ok {
			e := "error"
			if len(yamlErr.Errors) > 1 {
				e = "errors"
			}
			return errors.Errorf("yaml: unmarshal %s: %s", e, strings.Join(yamlErr.Errors, "; "))
		} else if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}

// Validate validates the site.
func (s *Site) Validate() error {
	err := s.Site.Validate()
	if err != nil {
		return errors.WithStack(err)
	}

	issuerSet := s.Auth.Issuer != ""
	clientIDSet := s.Auth.ClientID != ""
	clientSecretSet := s.Auth.ClientSecret != ""
	if issuerSet || clientIDSet || clientSecretSet {
		if !issuerSet || !clientIDSet || !clientSecretSet {
			errE := errors.New("site auth.issuer, auth.clientId, and auth.clientSecret must all be set to enable OIDC authentication, or none of them")
			errors.Details(errE)["domain"] = s.Domain
			return errE
		}
	}

	errE := s.validateVisibility()
	if errE != nil {
		return errE
	}

	return nil
}

// validateVisibility checks the Visibility configuration: level names must be unique and non-empty, every
// role assigned to a level must be a defined role (a key in Roles), and no role may appear in more than one
// level. Roles that are in no level and an empty Visibility are both allowed. In the latter case, Visibility
// is set to a default "all" level with empty roles. That default level is both the floor and the top, so
// every request resolves to it and ReadIndex never denies a no-levels site. A level with no roles must be the
// first (lowest) or the last (highest) level: as the first level it is the floor, granted to every request;
// as the last level it is granted to no request by role (no role matches it), but defines the unfiltered top
// level which is required to exist. The order of the levels (lowest to highest access) is otherwise not
// constrained by this method.
func (s *Site) validateVisibility() errors.E {
	if len(s.Visibility) == 0 {
		s.Visibility = []auth.VisibilityLevel{{Name: AllVisibilityLevel, Roles: nil}}
	}

	names := map[string]bool{}
	roleLevel := map[string]string{}
	for i, level := range s.Visibility {
		if level.Name == "" {
			errE := errors.New("visibility level has an empty name")
			errors.Details(errE)["domain"] = s.Domain
			return errE
		}
		if names[level.Name] {
			errE := errors.New("visibility level name is not unique")
			errors.Details(errE)["domain"] = s.Domain
			errors.Details(errE)["name"] = level.Name
			return errE
		}
		names[level.Name] = true
		if len(level.Roles) == 0 && i != 0 && i != len(s.Visibility)-1 {
			errE := errors.New("a visibility level with no roles must be the first or the last level")
			errors.Details(errE)["domain"] = s.Domain
			errors.Details(errE)["level"] = level.Name
			return errE
		}
		for _, role := range level.Roles {
			if _, ok := s.Roles[role]; !ok {
				errE := errors.New("visibility level references an unknown role")
				errors.Details(errE)["domain"] = s.Domain
				errors.Details(errE)["level"] = level.Name
				errors.Details(errE)["role"] = role
				return errE
			}
			if other, ok := roleLevel[role]; ok {
				errE := errors.New("role is assigned to more than one visibility level")
				errors.Details(errE)["domain"] = s.Domain
				errors.Details(errE)["role"] = role
				errors.Details(errE)["levels"] = []string{other, level.Name}
				return errE
			}
			roleLevel[role] = level.Name
		}
	}
	return nil
}

// LevelNames returns the configured visibility level names, from lowest to highest access.
func (s *Site) LevelNames() []string {
	if len(s.Visibility) == 0 {
		// Validation sets Visibility, but this might be called without validation, so we return the same.
		return []string{AllVisibilityLevel}
	}
	names := make([]string, len(s.Visibility))
	for i, level := range s.Visibility {
		names[i] = level.Name
	}
	return names
}

// EnabledLanguages returns the sorted enabled languages of the site, derived from LanguagePriority the same
// way the converter and the index mapping derive them: its keys plus the undetermined language, or the
// default when no language priority is configured.
func (s *Site) EnabledLanguages() []string {
	enabled, _ := internalSearch.EnabledLanguagesFromLanguagePriority(s.LanguagePriority)
	return slices.Sorted(maps.Keys(enabled))
}

// LevelIndexes returns the ElasticSearch index name for every visibility level, from lowest to highest.
func (s *Site) LevelIndexes() []string {
	names := s.LevelNames()
	indexes := make([]string, len(names))
	for i, name := range names {
		indexes[i] = internalSearch.LevelIndex(s.IndexPrefix, name)
	}
	return indexes
}

// TopIndex returns the ElasticSearch index for the highest (last) visibility level: the unfiltered superset
// that contains every document. Paths that must see all documents regardless of the caller use it.
func (s *Site) TopIndex() string {
	names := s.LevelNames()
	return internalSearch.LevelIndex(s.IndexPrefix, names[len(names)-1])
}

// ReadIndex returns the ElasticSearch index a request should read, derived from the caller's resolved
// visibility level, so a caller only ever reads the index filtered to its level. It returns store.ErrAccessDenied
// when the caller has no visibility level, so read routes that access ElasticSearch must respond with
// 403 Forbidden. A site that defines no visibility levels defaults to a single "all" level that is both
// floor and top, so every request resolves to it and this never denies there; the empty case only arises
// for a site that configures levels without a floor and a caller matching none.
func (s *Site) ReadIndex(ctx context.Context) (string, errors.E) {
	level := auth.Visibility(ctx)
	if level == "" {
		return "", errors.WithStack(store.ErrAccessDenied)
	}
	return internalSearch.LevelIndex(s.IndexPrefix, level), nil
}

func (s *Site) fetchDocumentIDs(ctx context.Context, classID identifier.Identifier) ([]identifier.Identifier, errors.E) {
	return internalSearch.FetchDocumentIDs(ctx, s.ESClient, s.TopIndex(), []identifier.Identifier{classID})
}

// FetchDocuments returns all documents that are instances of classID by loading their latest stored
// versions. It is used to load the property, class, and language documents that a site's converter
// needs at startup.
//
// It reads the raw stored documents directly and unfiltered, without the read-path document hooks
// (and thus any permission checks).
func (s *Site) FetchDocuments(ctx context.Context, classID identifier.Identifier) ([]base.StartDocument, errors.E) {
	allIDs, errE := s.fetchDocumentIDs(ctx, classID)
	if errE != nil {
		return nil, errE
	}

	documentsStore := s.Base.Documents()
	documents := make([]base.StartDocument, 0, len(allIDs))
	for _, id := range allIDs {
		data, metadata, version, parentChangesets, errE := documentsStore.GetLatest(ctx, id)
		if errE != nil {
			return nil, errE
		}
		doc := new(document.D)
		errE = x.UnmarshalWithoutUnknownFields(data, doc)
		if errE != nil {
			return nil, errE
		}
		documents = append(documents, base.StartDocument{
			Document:         doc,
			Metadata:         metadata,
			Version:          version,
			ParentChangesets: parentChangesets,
		})
	}

	return documents, nil
}

func (s *Site) validateDefaultLanguage() errors.E {
	if s.DefaultLanguage == "" {
		if len(s.LanguagePriority) < 1 {
			return nil
		}
		return errors.New("default language is required when language priority is set")
	}
	if _, ok := s.LanguagePriority[s.DefaultLanguage]; !ok {
		errE := errors.New("default language is not enabled")
		errors.Details(errE)["language"] = s.DefaultLanguage
		return errE
	}
	return nil
}

// This should be run before calling service.RouteWith because it freezes site's context.json
// as static file and updating language codes later means they are not included in context.json.
func (s *Site) updateLanguageCodes(_ context.Context) errors.E { //nolint:unparam
	s.LanguageCodes = s.Base.LanguageCodes()

	return nil
}

// Start starts the base for the site.
//
// You have to call this or PopulateAndStart for each site after Init.
func (s *Site) Start(ctx context.Context, documents []base.StartDocument) (func(), errors.E) {
	errE := s.validateDefaultLanguage()
	if errE != nil {
		return nil, errE
	}

	if s.Base.LanguagePriority == nil {
		s.Base.LanguagePriority = s.LanguagePriority
	}

	if !s.Base.IndexAncestorProperties {
		s.Base.IndexAncestorProperties = s.Features.IndexAncestorProperties
	}

	onShutdown, errE := s.Base.Start(ctx, documents)
	if errE != nil {
		return onShutdown, errE
	}

	errE = s.updateLanguageCodes(ctx)
	if errE != nil {
		return onShutdown, errE
	}

	return onShutdown, nil
}

// PopulateAndStart for the site: inserts the given documents into the store, starts the base,
// waits for Elasticsearch to catch up, and then refreshes ElasticSearch index.
//
// Optional count and size counters can be provided to track ES indexing progress.
//
// You have to call this or Start for each site after Init.
func (s *Site) PopulateAndStart(
	ctx context.Context, documents []*document.D, progress func(doc *document.D), beforeWait func(ctx context.Context) errors.E, count, size *x.Counter,
) (func(), errors.E) {
	errE := s.validateDefaultLanguage()
	if errE != nil {
		return nil, errE
	}

	if s.Base.LanguagePriority == nil {
		s.Base.LanguagePriority = s.LanguagePriority
	}

	if !s.Base.IndexAncestorProperties {
		s.Base.IndexAncestorProperties = s.Features.IndexAncestorProperties
	}

	onShutdown, errE := s.Base.PopulateAndStart(ctx, documents, progress, beforeWait, count, size)
	if errE != nil {
		return onShutdown, errE
	}

	errE = s.updateLanguageCodes(ctx)
	if errE != nil {
		return onShutdown, errE
	}

	return onShutdown, nil
}
