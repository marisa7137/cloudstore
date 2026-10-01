package gateway

import (
	"net/http"

	"cloudstore/gen/storagepb"
	"cloudstore/internal/gateway/db"
)

type Server struct {
	db      *db.DB
	storage storagepb.StorageServiceClient
	mux     *http.ServeMux
}

func NewServer(database *db.DB, storage storagepb.StorageServiceClient) *Server {
	s := &Server{db: database, storage: storage, mux: http.NewServeMux()}

	s.mux.HandleFunc("POST /api/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)
	s.mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("GET /api/files", s.requireAuth(http.HandlerFunc(s.handleListFiles)))
	s.mux.Handle("POST /api/uploads", s.requireAuth(http.HandlerFunc(s.handleInitUpload)))
	s.mux.Handle("PUT /api/uploads/{id}/chunks/{index}", s.requireAuth(http.HandlerFunc(s.handleUploadChunk)))
	s.mux.Handle("POST /api/uploads/{id}/complete", s.requireAuth(http.HandlerFunc(s.handleCompleteUpload)))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(s.mux).ServeHTTP(w, r)
}
