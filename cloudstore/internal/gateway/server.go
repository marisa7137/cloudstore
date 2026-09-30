package gateway

import (
	"net/http"

	"cloudstore/internal/gateway/db"
)

type Server struct {
	db  *db.DB
	mux *http.ServeMux
}

func NewServer(database *db.DB) *Server {
	s := &Server{db: database, mux: http.NewServeMux()}

	s.mux.HandleFunc("POST /api/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)
	s.mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(s.mux).ServeHTTP(w, r)
}
