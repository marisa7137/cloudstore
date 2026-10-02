package db

import "time"

// TaskStatus tracks the lifecycle of a background task (currently only uploads).
type TaskStatus string

const (
	TaskInProgress TaskStatus = "in_progress"
	TaskComplete   TaskStatus = "complete"
	TaskFailed     TaskStatus = "failed"
)

// Task records that a user started an operation on a file, e.g. an upload,
// and how far it has progressed.
type Task struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	FileID         int64      `json:"file_id"`
	Type           string     `json:"type"`
	Status         TaskStatus `json:"status"`
	ChunksTotal    int        `json:"chunks_total"`
	ChunksReceived int        `json:"chunks_received"`
	SourceHash     string     `json:"source_hash"`     // sha256 of source at init
	SourceModified int64      `json:"source_modified"` // client mtime, ms since epoch
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const taskColumns = `id, user_id, file_id, type, status, chunks_total, chunks_received, source_hash, source_modified, created_at, updated_at`

// CreateUploadTask records that a user started uploading a file, together
// with a fingerprint of the source so a changed file can be detected later.
func (d *DB) CreateUploadTask(userID, fileID int64, chunksTotal int, sourceHash string, sourceModified int64) (*Task, error) {
	res, err := d.conn.Exec(
		`INSERT INTO tasks (user_id, file_id, type, chunks_total, source_hash, source_modified)
		 VALUES (?, ?, 'upload', ?, ?, ?)`,
		userID, fileID, chunksTotal, sourceHash, sourceModified,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetTask(id)
}

func (d *DB) GetTask(id int64) (*Task, error) {
	return d.scanTask(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
}

// GetUploadTaskByFile returns the upload task tracking the given file.
func (d *DB) GetUploadTaskByFile(fileID int64) (*Task, error) {
	return d.scanTask(
		`SELECT `+taskColumns+` FROM tasks WHERE file_id = ? AND type = 'upload'`, fileID)
}

func (d *DB) scanTask(query string, args ...any) (*Task, error) {
	t := &Task{}
	err := d.conn.QueryRow(query, args...).Scan(
		&t.ID, &t.UserID, &t.FileID, &t.Type, &t.Status,
		&t.ChunksTotal, &t.ChunksReceived, &t.SourceHash, &t.SourceModified,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SetTaskChunksReceived records the current number of chunks on disk.
// The chunk files themselves are the source of truth; this column is a
// cached progress indicator for listings.
func (d *DB) SetTaskChunksReceived(id int64, received int) error {
	_, err := d.conn.Exec(`
		UPDATE tasks
		SET chunks_received = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, received, id)
	return err
}

// TaskListItem is a task enriched with its file's name and size for display.
type TaskListItem struct {
	Task
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

// ListUserTasks returns all tasks of a user, newest first.
func (d *DB) ListUserTasks(userID int64) ([]*TaskListItem, error) {
	rows, err := d.conn.Query(`
		SELECT t.id, t.user_id, t.file_id, t.type, t.status,
		       t.chunks_total, t.chunks_received, t.source_hash, t.source_modified,
		       t.created_at, t.updated_at,
		       f.name, f.size
		FROM tasks t JOIN files f ON f.id = t.file_id
		WHERE t.user_id = ?
		ORDER BY t.created_at DESC, t.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []*TaskListItem{}
	for rows.Next() {
		t := &TaskListItem{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.FileID, &t.Type, &t.Status,
			&t.ChunksTotal, &t.ChunksReceived, &t.SourceHash, &t.SourceModified,
			&t.CreatedAt, &t.UpdatedAt,
			&t.FileName, &t.FileSize,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (d *DB) SetTaskStatus(id int64, status TaskStatus) error {
	res, err := d.conn.Exec(`
		UPDATE tasks
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
