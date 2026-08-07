package peerdb_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/auth"
)

// TestConfigDecode verifies that a configuration file decodes into the configuration structs the way
// the configuration flag decodes it: with github.com/goccy/go-yaml and unknown fields disallowed (see
// cli.ConfigFlag), which is also how a site given as a flag is decoded (see Site.Decode). The role
// grants are what this is mainly about: they are written as permission action codes and resolve to a
// map keyed by the actions' document IDs, which only happens when their unmarshaler is called.
func TestConfigDecode(t *testing.T) {
	t.Parallel()

	config := `
globals:
  sites:
    - domain: example.com
      title: Example
      roles:
        "":
          ACTION_READ: [all]
        admin:
          ACTION_READ: [all]
          ACTION_UPDATE: [documents]
      visibility:
        - name: all
`
	var c peerdb.Config
	err := yaml.NewDecoder(strings.NewReader(config), yaml.DisallowUnknownField()).Decode(&c)
	require.NoError(t, err)

	require.Len(t, c.Sites, 1)
	site := c.Sites[0]
	assert.Equal(t, "example.com", site.Domain)
	assert.Equal(t, "Example", site.Title)
	require.Len(t, site.Visibility, 1)
	assert.Equal(t, "all", site.Visibility[0].Name)

	require.Len(t, site.Roles[auth.RoleEveryone][auth.ActionRead], 1)
	assert.Equal(t, auth.ScopeAll, site.Roles[auth.RoleEveryone][auth.ActionRead][0].Literal)
	require.Len(t, site.Roles["admin"][auth.ActionUpdate], 1)
	assert.Equal(t, auth.ScopeDocuments, site.Roles["admin"][auth.ActionUpdate][0].Literal)
}
