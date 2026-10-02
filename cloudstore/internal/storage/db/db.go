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

// FileStatus tracks the lifecycle of an upload.
type FileStatus string

const (
	StatusUploading FileStatus = "uploading" // row created, bytes still arriving
	StatusComplete  FileStatus = "complete"  // all bytes stored, md5 verified
	StatusFailed    FileStatus = "failed"    // upload aborted or checksum mismatch
)

// File is the metadata descriptor for one stored file (not its content).
type File struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	MD5       string     `json:"md5"`
	MimeType  string     `json:"mime_type"`
	Status    FileStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

var ErrNotFound = sql.ErrNoRows

// Open opens (creating if needed) the storage sqlite database and runs migrations.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL,
			name       TEXT    NOT NULL,
			size       INTEGER NOT NULL DEFAULT 0,
			md5        TEXT    NOT NULL DEFAULT '',
			mime_type  TEXT    NOT NULL DEFAULT '',
			status     TEXT    NOT NULL DEFAULT 'uploading'
			           CHECK (status IN ('uploading', 'complete', 'failed')),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_files_user ON files(user_id);

		CREATE TABLE IF NOT EXISTS tasks (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id         INTEGER NOT NULL,
			file_id         INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			type            TEXT    NOT NULL DEFAULT 'upload',
			status          TEXT    NOT NULL DEFAULT 'in_progress'
			                CHECK (status IN ('in_progress', 'complete', 'failed')),
			chunks_total    INTEGER NOT NULL,
			chunks_received INTEGER NOT NULL DEFAULT 0,
			source_hash     TEXT    NOT NULL DEFAULT '', -- sha256 of source file at init
			source_modified INTEGER NOT NULL DEFAULT 0,  -- client mtime, ms since epoch
			created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_file ON tasks(file_id);
	`)
	return err
}

func (d *DB) Close() error { return d.conn.Close() }

const fileColumns = `id, user_id, name, size, md5, mime_type, status, created_at, updated_at`

func scanFile(row *sql.Row) (*File, error) {
	f := &File{}
	err := row.Scan(&f.ID, &f.UserID, &f.Name, &f.Size, &f.MD5,
		&f.MimeType, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// CreateFile registers a new upload in 'uploading' state. Size and md5 may be
// zero/empty until the upload finishes.
func (d *DB) CreateFile(userID int64, name, mimeType string, size int64) (*File, error) {
	res, err := d.conn.Exec(
		`INSERT INTO files (user_id, name, mime_type, size) VALUES (?, ?, ?, ?)`,
		userID, name, mimeType, size,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetFile(id)
}

func (d *DB) GetFile(id int64) (*File, error) {
	return scanFile(d.conn.QueryRow(
		`SELECT `+fileColumns+` FROM files WHERE id = ?`, id))
}

// GetUserFile fetches a file only if it belongs to the given user,
// so one user can never address another user's files.
func (d *DB) GetUserFile(id, userID int64) (*File, error) {
	return scanFile(d.conn.QueryRow(
		`SELECT `+fileColumns+` FROM files WHERE id = ? AND user_id = ?`, id, userID))
}

// ListUserFiles returns all files of a user, newest first,
// including in-progress and failed uploads.
func (d *DB) ListUserFiles(userID int64) ([]*File, error) {
	rows, err := d.conn.Query(
		`SELECT `+fileColumns+` FROM files WHERE user_id = ? ORDER BY created_at DESC, id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []*File{} // non-nil so it encodes as [] instead of null
	for rows.Next() {
		f := &File{}
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.Size, &f.MD5,
			&f.MimeType, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// CompleteFile marks an upload as finished with its final size and checksum.
func (d *DB) CompleteFile(id, size int64, md5 string) error {
	return d.setStatus(id, StatusComplete, size, md5)
}

// FailFile marks an upload as failed (aborted stream, checksum mismatch, ...).
func (d *DB) FailFile(id int64) error {
	return d.setStatus(id, StatusFailed, 0, "")
}

func (d *DB) setStatus(id int64, status FileStatus, size int64, md5 string) error {
	res, err := d.conn.Exec(`
		UPDATE files
		SET status = ?, size = ?, md5 = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		status, size, md5, id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserUsage returns the bytes attributed to a user: completed files count at
// their final size, in-progress uploads at their declared size (reserving the
// space up front). Failed uploads don't count.
func (d *DB) UserUsage(userID int64) (int64, error) {
	var used int64
	err := d.conn.QueryRow(`
		SELECT COALESCE(SUM(size), 0) FROM files
		WHERE user_id = ? AND status IN ('complete', 'uploading')`,
		userID,
	).Scan(&used)
	return used, err
}

func (d *DB) DeleteFile(id, userID int64) error {
	res, err := d.conn.Exec(
		`DELETE FROM files WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
