package peerdb_test

import (
	"context"
	"crypto/tls"
	"embed"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	z "gitlab.com/tozd/go/zerolog"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/es"
)

//go:embed public
var publicFiles embed.FS

//nolint:exhaustruct
var testFiles = fstest.MapFS{ //nolint:gochecknoglobals
	"index.html": &fstest.MapFile{
		Data: []byte("<html><body>dummy test content</body></html>"),
	},
	// Symlinks are not included in publicFiles.
	"LICENSE.txt": &fstest.MapFile{
		Data: []byte("test license file"),
	},
	"NOTICE.txt": &fstest.MapFile{
		Data: []byte("test notice file"),
	},
}

func init() { //nolint:gochecknoinits
	f, err := fs.Sub(publicFiles, "public")
	if err != nil {
		panic(err)
	}

	err = fs.WalkDir(f, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := f.(fs.ReadFileFS).ReadFile(path) //nolint:forcetypeassert,errcheck
		if err != nil {
			return err //nolint:wrapcheck
		}

		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck
		}

		testFiles[filepath.Join("dist", path)] = &fstest.MapFile{
			Data:    data,
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Sys:     info.Sys(),
		}

		return nil
	})
	if err != nil {
		panic(err)
	}
}

func testStaticFile(t *testing.T, route, filePath, contentType string) {
	t.Helper()

	ts, service := startTestServer(t, nil)

	path, errE := service.Reverse(route, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	expected, err := testFiles.ReadFile(filePath)
	require.NoError(t, err)

	resp, err := ts.Client().Get(ts.URL + path) //nolint:noctx,bodyclose
	if assert.NoError(t, err) {
		t.Cleanup(func(r *http.Response) func() { return func() { r.Body.Close() } }(resp)) //nolint:errcheck,gosec
		out, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 2, resp.ProtoMajor)
		assert.Equal(t, contentType, resp.Header.Get("Content-Type"))
		assert.Equal(t, string(expected), string(out))
	}
}

func TestRouteHome(t *testing.T) {
	t.Parallel()

	// Regular GET should just return the SPA index page.
	testStaticFile(t, "Home", "index.html", "text/html; charset=utf-8")
}

func startTestServer(t *testing.T, setupFunc func(globals *peerdb.Globals, serve *peerdb.ServeCommand)) (*httptest.Server, *peerdb.Service) {
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
			URL:   os.Getenv("ELASTIC"),
			Index: strings.ToLower(identifier.New().String()),
		},
	}

	serve := &peerdb.ServeCommand{ //nolint:exhaustruct
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
		Title: peerdb.DefaultTitle,
	}

	if setupFunc != nil {
		setupFunc(globals, serve)
	}

	for i := range globals.Sites {
		site := &globals.Sites[i]
		require.Empty(t, site.Schema)
		site.Schema = identifier.New().String()
		require.Empty(t, site.Index)
		site.Index = strings.ToLower(identifier.New().String())
	}

	err := globals.Validate()
	require.NoError(t, err)

	err = serve.Validate()
	require.NoError(t, err)

	populate := peerdb.PopulateCommand{}

	errE := populate.Run(globals)
	require.NoError(t, errE, "% -+#.1v", errE)

	domains := []string{"localhost"}
	if len(globals.Sites) > 0 {
		domains = []string{}
		for _, site := range globals.Sites {
			domains = append(domains, site.Domain)
		}
	}
	errE = x.CreateTempCertificateFiles(certPath, keyPath, domains)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := range globals.Sites {
		site := &globals.Sites[i]
		site.CertFile = certPath
		site.KeyFile = keyPath
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

	certDomain := "localhost"
	if len(globals.Sites) > 0 {
		// We can pick any domain to obtain the cert because we have made one cert for all domains.
		certDomain = globals.Sites[0].Domain
	}

	// We have to call GetCertificate ourselves.
	// See: https://github.com/golang/go/issues/63812
	cert, err := ts.TLS.GetCertificate(&tls.ClientHelloInfo{ //nolint:exhaustruct
		ServerName: certDomain,
	})
	require.NotNil(t, cert)
	require.NoError(t, err)
	// By setting Certificates, we force testing server and testing client to use our certificate.
	ts.TLS.Certificates = []tls.Certificate{*cert}

	// This does not start server's managers, but that is OK for this test.
	ts.StartTLS()

	// Our certificate is not for 127.0.0.1 IP. So we set it to certDomain.
	// Caller can generate its own URLs for other sites if needed.
	ts.URL = strings.ReplaceAll(ts.URL, "127.0.0.1", certDomain)

	dialerContext := cleanhttp.DefaultTransport().DialContext
	ts.Client().Transport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) { //nolint:errcheck,forcetypeassert
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// We map any connection to domains used in sites to localhost.
		for _, site := range globals.Sites {
			if site.Domain == host {
				addr = net.JoinHostPort("localhost", port)
				break
			}
		}
		return dialerContext(ctx, network, addr)
	}

	cleanupESClient, errE := es.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		ctx := context.Background()
		if len(globals.Sites) == 0 {
			_, err = cleanupESClient.DeleteIndex(globals.Elastic.Index).Do(ctx)
			if err != nil {
				require.NoError(t, err)
			}
		} else {
			for _, site := range globals.Sites {
				_, err = cleanupESClient.DeleteIndex(site.Index).Do(ctx)
				if err != nil {
					require.NoError(t, err)
				}
			}
		}
	})

	return ts, service
}
