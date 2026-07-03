package agent

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func AuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		tokenMatch := subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
		if !ok || !tokenMatch {
			WriteError(w, http.StatusUnauthorized, KindUnauthorized, "missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
