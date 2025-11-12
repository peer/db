package peerdb_test

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/tozd/go/x"
	z "gitlab.com/tozd/go/zerolog"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
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
		{`invalid username`, testWrongUsername, testPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid password`, testUsername, testWrongPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid both`, testWrongUsername, testWrongPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid w/ username space`, `testuser `, testPassword, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid w/ password space`, testUsername, `testpass `, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid no credentials`, ``, ``, http.StatusUnauthorized,
			peerdb.DefaultTitle},
	}

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	middleware := peerdb.BasicAuthHandler(
		peerdb.HasherSHA256(testUsername),
		peerdb.HasherSHA256(testPassword),
		peerdb.DefaultTitle,
	)
	handler := middleware(innerHandler)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			req := httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.username != "" || tt.password != "" {
				// RFC 7617 construct username:password and base64 encode it - mimic browser behavior.
				auth := tt.username + ":" + tt.password
				encoded := base64.StdEncoding.EncodeToString([]byte(auth))
				req.Header.Set("Authorization", "Basic "+encoded)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusUnauthorized {
				authHeader := w.Header().Get("WWW-Authenticate")
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

			sites := []peerdb.Site{ //nolint:exhaustruct
				{
					Site: waf.Site{
						Domain: tt.domain,
					},
					Index:  strings.ToLower(identifier.New().String()),
					Schema: identifier.New().String(),
					Title:  tt.siteTitle,
				},
			}

			ts, _ := startTestServerWithConfig(t, sites, testUsername, testPassword)
			// We only test unauthorized responses here to verify the realm.
			req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
			require.NoError(t, err)
			req.Host = tt.domain

			resp, err := ts.Client().Do(req) //nolint:bodyclose
			if assert.NoError(t, err) {
				t.Cleanup(func(r *http.Response) func() { return func() { r.Body.Close() } }(resp))

				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				authHeader := resp.Header.Get("WWW-Authenticate")
				require.NotEmpty(t, authHeader)
				assert.Contains(t, authHeader, `Basic realm="`+tt.expectedRealm+`"`)
			}
		})
	}
}

func startTestServerWithConfig(t *testing.T, sites []peerdb.Site, username string, password string) (*httptest.Server, *peerdb.Service) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}
	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "test_cert.pem")
	keyPath := filepath.Join(tempDir, "test_key.pem")

	domains := []string{"localhost"}
	for _, site := range sites {
		domains = append(domains, site.Domain)
	}

	errE := x.CreateTempCertificateFiles(certPath, keyPath, domains)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := range sites {
		sites[i].Site.CertFile = certPath
		sites[i].Site.KeyFile = keyPath
	}

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	globals := &peerdb.Globals{ //nolint:exhaustruct
		LoggingConfig: z.LoggingConfig{ //nolint:exhaustruct
			Logger: logger,
		},
		Postgres: peerdb.PostgresConfig{
			URL:    []byte(os.Getenv("POSTGRES")),
			Schema: identifier.New().String(),
		},
		Elastic: peerdb.ElasticConfig{
			URL:       os.Getenv("ELASTIC"),
			Index:     strings.ToLower(identifier.New().String()),
			SizeField: false,
		},
		Sites: sites,
	}

	populate := peerdb.PopulateCommand{}

	errE = populate.Run(globals)
	require.NoError(t, errE, "% -+#.1v", errE)

	serve := peerdb.ServeCommand{ //nolint:exhaustruct
		Server: waf.Server[*peerdb.Site]{ //nolint:exhaustruct
			TLS: waf.TLS{ //nolint:exhaustruct
				CertFile: certPath,
				KeyFile:  keyPath,
			},
			Development: true,
			// httptest.Server allocates a random port for its listener (but does not use serve.Server.Addr to do so).
			// Having 0 for port here makes the rest of the codebase expect a random port and wait for its assignment.
			Addr: "localhost:0",
		},
		Username: username,
		Password: []byte(password),
		Title:    peerdb.DefaultTitle,
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	handler, service, errE := serve.Init(ctx, globals, testFiles)
	require.NoError(t, errE, "% -+#.1v", errE)

	ts := httptest.NewUnstartedServer(nil)
	ts.EnableHTTP2 = true
	t.Cleanup(ts.Close)

	ts.Config = serve.Server.HTTPServer
	ts.Config.Handler = handler
	ts.TLS = serve.Server.HTTPServer.TLSConfig.Clone()

	// We have to call GetCertificate ourselves.
	// See: https://github.com/golang/go/issues/63812
	cert, err := ts.TLS.GetCertificate(&tls.ClientHelloInfo{ //nolint:exhaustruct
		ServerName: sites[0].Domain,
	})
	require.NoError(t, err, "% -+#.1v", err)

	// By setting Certificates, we force testing server and testing client to use our certificate.
	ts.TLS.Certificates = []tls.Certificate{*cert}

	// This does not start server's managers, but that is OK for this test.
	ts.StartTLS()

	// Our certificate is for localhost domain and not 127.0.0.1 IP.
	ts.URL = strings.ReplaceAll(ts.URL, "127.0.0.1", "localhost")

	cleanupESClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		ctx := context.Background()
		for _, site := range sites {
			if site.Index != "" {
				_, err = cleanupESClient.DeleteIndex(site.Index).Do(ctx)
				if err != nil {
					require.NoError(t, err, "% -+#.1v", err)
				}
			}
		}
	})

	return ts, service
}
