package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// Open opens (creating if needed) the gateway sqlite database and runs migrations.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// sqlite allows only one writer; avoid "database is locked" errors.
	conn.SetMaxOpenConns(1)

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT    NOT NULL UNIQUE,
			password_hash TEXT    NOT NULL,
			created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT    PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TIMESTAMP NOT NULL
		);
	`)
	return err
}

func (d *DB) Close() error { return d.conn.Close() }

// ErrNotFound is re-exported so callers don't import database/sql.
var ErrNotFound = sql.ErrNoRows

func (d *DB) CreateUser(username, passwordHash string) (*User, error) {
	res, err := d.conn.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username, passwordHash,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetUserByID(id)
}

func (d *DB) GetUserByID(id int64) (*User, error) {
	u := &User{}
	err := d.conn.QueryRow(
		`SELECT id, username, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByUsername also returns the password hash for login verification.
func (d *DB) GetUserByUsername(username string) (*User, string, error) {
	u := &User{}
	var hash string
	err := d.conn.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &hash, &u.CreatedAt)
	if err != nil {
		return nil, "", err
	}
	return u, hash, nil
}

func (d *DB) CreateSession(token string, userID int64, expiresAt time.Time) error {
	_, err := d.conn.Exec(
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expiresAt,
	)
	return err
}

// GetSessionUser resolves a session token to its user, if the session is still valid.
func (d *DB) GetSessionUser(token string) (*User, error) {
	u := &User{}
	err := d.conn.QueryRow(`
		SELECT u.id, u.username, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > CURRENT_TIMESTAMP`,
		token,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) DeleteSession(token string) error {
	_, err := d.conn.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
