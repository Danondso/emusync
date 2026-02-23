package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// AuthMiddleware returns middleware that checks for a valid Bearer token.
func AuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) != 1 {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
