package gateway

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"cloudstore/gen/storagepb"
)

type taskJSON struct {
	ID             int64     `json:"id"`
	FileID         int64     `json:"file_id"`
	FileName       string    `json:"file_name"`
	FileSize       int64     `json:"file_size"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	ChunksTotal    int32     `json:"chunks_total"`
	ChunksReceived int32     `json:"chunks_received"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GET /api/tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.storage.ListTasks(ctx, &storagepb.ListTasksRequest{
		UserId: currentUser(r).ID,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}

	tasks := make([]taskJSON, 0, len(resp.GetTasks()))
	for _, t := range resp.GetTasks() {
		tasks = append(tasks, taskJSON{
			ID:             t.GetId(),
			FileID:         t.GetFileId(),
			FileName:       t.GetFileName(),
			FileSize:       t.GetFileSize(),
			Type:           t.GetType(),
			Status:         taskStatusString(t.GetStatus()),
			ChunksTotal:    t.GetChunksTotal(),
			ChunksReceived: t.GetChunksReceived(),
			CreatedAt:      t.GetCreatedAt().AsTime(),
			UpdatedAt:      t.GetUpdatedAt().AsTime(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// GET /api/uploads/{id}/status
func (s *Server) handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.storage.GetUploadStatus(ctx, &storagepb.UploadStatusRequest{
		UserId: currentUser(r).ID,
		FileId: fileID,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}

	missing := resp.GetMissingChunks()
	if missing == nil {
		missing = []int32{} // encode as [] instead of null
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chunks_total":   resp.GetChunksTotal(),
		"missing_chunks": missing,
	})
}

func taskStatusString(s storagepb.TaskStatus) string {
	switch s {
	case storagepb.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return "in_progress"
	case storagepb.TaskStatus_TASK_STATUS_COMPLETE:
		return "complete"
	case storagepb.TaskStatus_TASK_STATUS_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}
