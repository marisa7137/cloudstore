package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cloudstore/internal/gateway/db"
)

const (
	sessionCookieName = "session"
	sessionDuration   = 7 * 24 * time.Hour
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	creds.Username = strings.TrimSpace(creds.Username)
	if len(creds.Username) < 3 || len(creds.Username) > 32 {
		writeError(w, http.StatusBadRequest, "username must be 3-32 characters")
		return
	}
	if len(creds.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := s.db.CreateUser(creds.Username, string(hash))
	if err != nil {
		// UNIQUE constraint violation -> username taken
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, hash, err := s.db.GetUserByUsername(strings.TrimSpace(creds.Username))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Same message as bad password so usernames can't be probed.
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(creds.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := newSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	expiresAt := time.Now().UTC().Add(sessionDuration)
	if err := s.db.CreateSession(token, user.ID, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true, // not readable from JS
		SameSite: http.SameSiteLaxMode,
		// Secure: true, // enable once serving over HTTPS
	})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.db.DeleteSession(cookie.Value)
	}
	// Expire the cookie in the browser.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
