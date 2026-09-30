package gateway

import (
	"context"
	"net/http"

	"cloudstore/internal/gateway/db"
)

type ctxKey string

const userKey ctxKey = "user"

// corsMiddleware allows the Next.js dev server to call the API with cookies.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth validates the session cookie and puts the user on the request context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		user, err := s.db.GetSessionUser(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// currentUser retrieves the user placed on the context by requireAuth.
func currentUser(r *http.Request) *db.User {
	return r.Context().Value(userKey).(*db.User)
}
