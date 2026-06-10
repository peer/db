package peerdb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

// TestCustomizerSiteDefaultsValidate verifies that Globals.Validate runs SiteDefaults on every configured
// site before the site is validated, so the hook sees the raw configured state and the values it sets are
// validated, and that a SiteDefaults error aborts validation.
func TestCustomizerSiteDefaultsValidate(t *testing.T) {
	t.Parallel()

	newGlobals := func() *peerdb.Globals {
		return &peerdb.Globals{ //nolint:exhaustruct
			Sites: []internalSite.Site{
				{ //nolint:exhaustruct
					Site: waf.Site{ //nolint:exhaustruct
						Domain: "localhost",
					},
				},
			},
		}
	}

	globals := newGlobals()
	globals.Customize = peerdb.Customizer{ //nolint:exhaustruct
		SiteDefaults: func(site *peerdb.Site) errors.E {
			// SiteDefaults runs before validation: visibility is still the raw (empty) configured
			// state, not yet defaulted by validation.
			assert.Empty(t, site.Visibility)
			site.Title = "Customized"
			return nil
		},
	}
	err := globals.Validate()
	require.NoError(t, err)
	assert.Equal(t, "Customized", globals.Sites[0].Title)
	// Validation ran after SiteDefaults and defaulted the empty visibility.
	require.Len(t, globals.Sites[0].Visibility, 1)
	assert.Equal(t, internalSite.AllVisibilityLevel, globals.Sites[0].Visibility[0].Name)

	globals = newGlobals()
	globals.Customize = peerdb.Customizer{ //nolint:exhaustruct
		SiteDefaults: func(_ *peerdb.Site) errors.E {
			return errors.New("test error")
		},
	}
	err = globals.Validate()
	assert.ErrorContains(t, err, "test error")
}

// TestCustomizerInitSites verifies that InitSites applies SiteDefaults to the default site synthesized when
// no sites are configured and that a SiteDefaults error propagates.
func TestCustomizerInitSites(t *testing.T) {
	t.Parallel()

	globals := &peerdb.Globals{ //nolint:exhaustruct
		Postgres: peerdb.PostgresConfig{ //nolint:exhaustruct
			Schema: "testschema",
		},
		Elastic: peerdb.ElasticConfig{ //nolint:exhaustruct
			IndexPrefix: "testindex",
		},
	}
	globals.Customize = peerdb.Customizer{ //nolint:exhaustruct
		SiteDefaults: func(site *peerdb.Site) errors.E {
			site.Title = "Customized"
			return nil
		},
	}
	errE := peerdb.InitSites(globals)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, globals.Sites, 1)
	assert.Equal(t, "Customized", globals.Sites[0].Title)
	assert.Equal(t, "testschema", globals.Sites[0].Schema)
	assert.Equal(t, "testindex", globals.Sites[0].IndexPrefix)
	// The synthesized site is validated as well: validation defaulted the empty visibility.
	require.Len(t, globals.Sites[0].Visibility, 1)
	assert.Equal(t, internalSite.AllVisibilityLevel, globals.Sites[0].Visibility[0].Name)

	globals.Sites = nil
	globals.Customize.SiteDefaults = func(_ *peerdb.Site) errors.E {
		return errors.New("test error")
	}
	errE = peerdb.InitSites(globals)
	assert.ErrorContains(t, errE, "test error")
}
