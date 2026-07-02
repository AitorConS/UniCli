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
