package peerdb

import (
	"context"
	"io"
	"net/http"
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

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/search"
)

// Build contains version and build metadata.
type Build struct {
	Version        string `json:"version,omitempty"`
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
	Revision       string `json:"revision,omitempty"`
}

// SiteFeatures contains enabled feature flags.
type SiteFeatures struct {
	SearchResultsTable bool `json:"searchResultsTable,omitempty" yaml:"searchResultsTable,omitempty"`
	DownloadButtons    bool `json:"downloadButtons,omitempty"    yaml:"downloadButtons,omitempty"`

	// IndexAncestorProperties enables claim propagation to transitive super-properties
	// when indexing: a claim for property X is also indexed for every ancestor of X
	// via SUBPROPERTY_OF. Disabled by default. Backend-only; not exposed to the frontend.
	IndexAncestorProperties bool `json:"-" yaml:"indexAncestorProperties,omitempty"`
}

// SiteOIDC carries the OIDC configuration exposed to the frontend so it can
// initiate a sign-in flow. The fields are populated by the serve command from
// the global --auth.issuer and --auth.client-id flags, with the redirect URI
// derived from the site's own domain.
type SiteOIDC struct {
	Issuer      string `json:"issuer"`
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
}

// Site represents a single site in the PeerDB application with its configuration and state.
type Site struct {
	waf.Site `yaml:",inline"`

	Build *Build `json:"build,omitempty" yaml:"-"`

	Index  string `json:"-"               yaml:"index,omitempty"`
	Schema string `json:"-"               yaml:"schema,omitempty"`
	Title  string `json:"title,omitempty" yaml:"title,omitempty"`
	Logo   string `json:"logo,omitempty"  yaml:"logo,omitempty"`

	LanguagePriority map[string][]string `json:"languagePriority,omitempty" yaml:"languagePriority,omitempty"`
	DefaultLanguage  string              `json:"defaultLanguage,omitempty"  yaml:"defaultLanguage,omitempty"`

	// TODO: How to keep LanguageCodes in sync, if they are added or removed after initialization?
	LanguageCodes map[identifier.Identifier]string `json:"languageCodes,omitempty" yaml:"-"`

	Features SiteFeatures `json:"features" yaml:"features"`

	// Roles is a map of role names to permissions.
	Roles map[string][]string `json:"roles,omitempty" yaml:"roles,omitempty"`

	// OIDC is populated at startup when the server is launched with
	// --auth.issuer and --auth.client-id. It is exposed in the site context
	// so the frontend can start an OIDC sign-in flow against the issuer.
	OIDC *SiteOIDC `json:"oidc,omitempty" yaml:"-"`

	Base        *base.B                    `json:"-" yaml:"-"`
	DBPool      *pgxpool.Pool              `json:"-" yaml:"-"`
	ESClient    *elasticsearch.TypedClient `json:"-" yaml:"-"`
	RiverClient *river.Client[pgx.Tx]      `json:"-" yaml:"-"`

	// debugRiverHandler is the River UI handler mounted at /debug/river.
	// Populated only in development mode.
	debugRiverHandler http.Handler

	initialized bool

	// TODO: How to keep propertiesTotal and unitsTotal in sync with the number of properties and units available, if they are added or removed after initialization?
	propertiesTotal int64
	unitsTotal      int64
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
	err = decoder.Decode(s)
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

func (s *Site) fetchDocumentIDs(ctx context.Context, classID identifier.Identifier) ([]identifier.Identifier, errors.E) {
	return search.FetchDocumentIDs(ctx, s.ESClient, s.Index, []identifier.Identifier{classID})
}

func (s *Site) fetchDocuments(ctx context.Context, classID identifier.Identifier) ([]*document.D, errors.E) {
	allIDs, errE := s.fetchDocumentIDs(ctx, classID)
	if errE != nil {
		return nil, errE
	}

	documents := make([]*document.D, 0, len(allIDs))
	for _, id := range allIDs {
		doc, _, _, _, errE := s.Base.GetDocumentLatestDoc(ctx, id)
		if errE != nil {
			return nil, errE
		}
		documents = append(documents, doc)
	}

	return documents, nil
}

func (s *Site) updatePropertiesTotal(_ context.Context, documents []*document.D) errors.E { //nolint:unparam
	// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?
	// TODO: Limit really only to properties.
	s.propertiesTotal = int64(len(documents))
	return nil
}

func (s *Site) updateUnitsTotal(_ context.Context, documents []*document.D) errors.E { //nolint:unparam
	// TODO: Limit really only to units.
	s.unitsTotal = int64(len(documents))
	return nil
}

func (s *Site) validateDefaultLanguage() errors.E {
	if s.DefaultLanguage == "" {
		if len(s.LanguagePriority) < 1 {
			return nil
		}
		return errors.New("default language is required when more than one language is enabled")
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
func (s *Site) Start(ctx context.Context, documents []*document.D) (func(), errors.E) {
	errE := s.updatePropertiesTotal(ctx, documents)
	if errE != nil {
		return nil, errE
	}

	errE = s.updateUnitsTotal(ctx, documents)
	if errE != nil {
		return nil, errE
	}

	errE = s.validateDefaultLanguage()
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
	errE := s.updatePropertiesTotal(ctx, documents)
	if errE != nil {
		return nil, errE
	}

	errE = s.updateUnitsTotal(ctx, documents)
	if errE != nil {
		return nil, errE
	}

	errE = s.validateDefaultLanguage()
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
