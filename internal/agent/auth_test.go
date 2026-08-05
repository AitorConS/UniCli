package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantCalled bool
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic secret", wantStatus: http.StatusUnauthorized},
		{name: "wrong same length", header: "Bearer secres", wantStatus: http.StatusUnauthorized},
		{name: "wrong shorter", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "ok", header: "Bearer secret", wantStatus: http.StatusOK, wantCalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := AuthMiddleware("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			require.Equal(t, tt.wantStatus, rr.Code)
			require.Equal(t, tt.wantCalled, called)
		})
	}
}

// TestAuthMiddleware_EmptyToken pins the (subtle, security-relevant) contract
// when the agent is configured with no token. This is NOT a fully open mode:
//   - a request with no Authorization header is still rejected (401), and
//   - a request must present an explicit empty bearer ("Authorization: Bearer ")
//     to be admitted.
//
// The apiserver disables auth outright on an empty token, and the daemon refuses
// tcp:// without a token unless --allow-insecure is set, so this path is only
// reached for local transports. Pinning it here guards against a refactor that
// would silently turn an empty token into "allow everything", including requests
// with no header at all.
func TestAuthMiddleware_EmptyToken(t *testing.T) {
	tests := []struct {
		name       string
		setHeader  bool
		header     string
		wantStatus int
		wantCalled bool
	}{
		{name: "no header rejected", setHeader: false, wantStatus: http.StatusUnauthorized},
		{name: "empty bearer admitted", setHeader: true, header: "Bearer ", wantStatus: http.StatusOK, wantCalled: true},
		{name: "non-empty bearer rejected", setHeader: true, header: "Bearer anything", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme rejected", setHeader: true, header: "Basic ", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := AuthMiddleware("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
			if tt.setHeader {
				req.Header.Set("Authorization", tt.header)
			}
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			require.Equal(t, tt.wantStatus, rr.Code)
			require.Equal(t, tt.wantCalled, called, "handler invocation must match the auth decision")
		})
	}
}
