package peerdb_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/peerdb/peerdb"
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
		{`valid credentials`, `testuser`, `testpass`, http.StatusOK,
			peerdb.DefaultTitle},
		{`invalid username`, `wronguser`, `testpass`, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid password`, `testuser`, `wrongpass`, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`invalid both`, `wronguser`, `wrongpass`, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`valid w/ username space`, `testuser `, `testpass`, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`valid w/ password space`, `testuser`, `testpass `, http.StatusUnauthorized,
			peerdb.DefaultTitle},
		{`no credentials`, ``, ``, http.StatusUnauthorized, ``},
	}

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	middleware := peerdb.BasicAuthHandler(
		peerdb.HasherSHA256("testuser"),
		peerdb.HasherSHA256("testpass"),
		peerdb.DefaultTitle,
	)
	handler := middleware(innerHandler)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
