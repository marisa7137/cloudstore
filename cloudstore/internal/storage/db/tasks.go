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
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const taskColumns = `id, user_id, file_id, type, status, chunks_total, chunks_received, created_at, updated_at`

// CreateUploadTask records that a user started uploading a file.
func (d *DB) CreateUploadTask(userID, fileID int64, chunksTotal int) (*Task, error) {
	res, err := d.conn.Exec(
		`INSERT INTO tasks (user_id, file_id, type, chunks_total) VALUES (?, ?, 'upload', ?)`,
		userID, fileID, chunksTotal,
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
		&t.ChunksTotal, &t.ChunksReceived, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// IncrementTaskChunks counts one more received chunk and returns the new count.
func (d *DB) IncrementTaskChunks(id int64) (int, error) {
	_, err := d.conn.Exec(`
		UPDATE tasks
		SET chunks_received = chunks_received + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	var received int
	err = d.conn.QueryRow(`SELECT chunks_received FROM tasks WHERE id = ?`, id).Scan(&received)
	return received, err
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
