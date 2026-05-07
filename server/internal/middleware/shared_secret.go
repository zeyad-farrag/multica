package middleware

import (
	"crypto/subtle"
	"net/http"
)

// SharedSecretAuth protects machine-to-machine Team App system routes.
// It rejects before reading the request body so unauthenticated callers cannot
// force body parsing work.
func SharedSecretAuth(secret string) func(http.Handler) http.Handler {
	expected := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("X-Team-App-Secret"))
			if len(got) != len(expected) || subtle.ConstantTimeCompare(got, expected) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
