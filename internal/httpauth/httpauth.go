// Package httpauth provides a small, constant-time bearer-token middleware for
// the daemon's optional-auth HTTP surfaces (Prometheus metrics, web dashboard).
// Centralizing the check keeps every endpoint's authentication behavior
// identical instead of re-implementing it per server.
package httpauth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Bearer wraps next so that, when token is non-empty, every request must carry a
// matching "Authorization: Bearer <token>" header, compared in constant time.
//
// An empty token disables the check and returns next unchanged, so a caller can
// pass an operator-supplied token through unconditionally: no token configured
// means the endpoint stays open exactly as it was before.
func Bearer(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
