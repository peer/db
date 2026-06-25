package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/auth"
)

// TestSafeRedirectPath covers the same-site path validation used by
// SignIn to bound where the user can be sent after the callback.
// Anything that could let a malicious sign-in URL bounce the user to
// an external host must collapse to "/".
func TestSafeRedirectPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "/"},
		{"root", "/", "/"},
		{"absolute path", "/foo", "/foo"},
		{"nested path", "/foo/bar", "/foo/bar"},
		{"path with query", "/foo?q=v", "/foo?q=v"},
		{"path with fragment", "/foo#section", "/foo#section"},
		{"protocol-relative", "//evil.example", "/"},
		{"http absolute", "http://evil.example", "/"},
		{"https absolute", "https://evil.example", "/"},
		{"javascript scheme", "javascript:alert(1)", "/"},
		{"data scheme", "data:text/html,<script>", "/"},
		{"relative", "foo/bar", "/"},
		{"backslash trick", `\evil.example`, "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, auth.TestingSafeRedirectPath(tt.in))
		})
	}
}
