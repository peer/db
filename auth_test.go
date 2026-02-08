package peerdb_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
)

const (
	testUsername      = "testuser"
	testWrongUsername = "wronguser"
	testPassword      = "testpass"
	testWrongPassword = "wrongpass"
)

func TestBasicAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		username       string
		password       string
		expectedStatus int
		expectedRealm  string
	}{
		{`valid credentials`, testUsername, testPassword, http.StatusOK, ""},
		{
			`invalid username`, testWrongUsername, testPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
		{
			`invalid password`, testUsername, testWrongPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
		{
			`invalid both`, testWrongUsername, testWrongPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
		{
			`invalid w/ username space`, `testuser `, testPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
		{
			`invalid w/ password space`, testUsername, `testpass `, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
		{
			`invalid no credentials`, ``, ``, http.StatusUnauthorized,
			peerdb.DefaultTitle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts, _ := startTestServer(t, func(_ *peerdb.Globals, serve *peerdb.ServeCommand) {
				serve.Username = testUsername
				serve.Password = []byte(testPassword)
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
			require.NoError(t, err)

			if tt.username != "" || tt.password != "" {
				// RFC 7617 construct username:password and base64 encode it - mimic browser behavior.
				auth := tt.username + ":" + tt.password
				encoded := base64.StdEncoding.EncodeToString([]byte(auth))
				req.Header.Set("Authorization", "Basic "+encoded)
			}

			resp, err := ts.Client().Do(req) //nolint:bodyclose
			require.NoError(t, err)
			t.Cleanup(func(r *http.Response) func() { return func() { r.Body.Close() } }(resp)) //nolint:errcheck,gosec

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == http.StatusUnauthorized {
				authHeader := resp.Header.Get("WWW-Authenticate")
				require.NotEmpty(t, authHeader)
				assert.Contains(t, authHeader, `Basic realm="`+tt.expectedRealm+`"`)
			}
		})
	}
}

func TestBasicAuthWithSiteContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		domain        string
		siteTitle     string
		expectedRealm string
	}{
		{`site with localhost domain`, `localhost`, `Example Site`, `Example Site`},
		{`site with custom title`, `example.com`, `Example Site`, `Example Site`},
		{`site with default title`, `test.com`, peerdb.DefaultTitle, peerdb.DefaultTitle},
		{`site with empty title`, `fallback.com`, ``, peerdb.DefaultTitle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts, _ := startTestServer(t, func(globals *peerdb.Globals, serve *peerdb.ServeCommand) {
				globals.Sites = []peerdb.Site{
					{
						Site: waf.Site{
							Domain:   tt.domain,
							CertFile: "",
							KeyFile:  "",
						},
						Build:       nil,
						Index:       "",
						Schema:      "",
						Title:       tt.siteTitle,
						Store:       nil,
						Coordinator: nil,
						Storage:     nil,
						ESProcessor: nil,
						ESClient:    nil,
						DBPool:      nil,
					},
				}
				serve.Username = testUsername
				serve.Password = []byte(testPassword)
			})
			// We only test unauthorized responses here to verify the realm.
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
			require.NoError(t, err)
			req.Host = tt.domain

			resp, err := ts.Client().Do(req) //nolint:bodyclose
			require.NoError(t, err)
			t.Cleanup(func(r *http.Response) func() { return func() { r.Body.Close() } }(resp)) //nolint:errcheck,gosec

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			authHeader := resp.Header.Get("WWW-Authenticate")
			require.NotEmpty(t, authHeader)
			assert.Contains(t, authHeader, `Basic realm="`+tt.expectedRealm+`"`)
		})
	}
}
