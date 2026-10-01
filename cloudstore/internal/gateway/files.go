package gateway

import (
	"context"
	"net/http"
	"time"

	"cloudstore/gen/storagepb"
)

// fileJSON is the REST representation of a file descriptor. The gateway owns
// the REST contract, so proto messages are mapped instead of exposed directly.
type fileJSON struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MD5       string    `json:"md5"`
	MimeType  string    `json:"mime_type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.storage.ListFiles(ctx, &storagepb.ListFilesRequest{UserId: user.ID})
	if err != nil {
		writeError(w, http.StatusBadGateway, "storage service unavailable")
		return
	}

	files := make([]fileJSON, 0, len(resp.GetFiles()))
	for _, f := range resp.GetFiles() {
		files = append(files, fileJSON{
			ID:        f.GetId(),
			Name:      f.GetName(),
			Size:      f.GetSize(),
			MD5:       f.GetMd5(),
			MimeType:  f.GetMimeType(),
			Status:    statusString(f.GetStatus()),
			CreatedAt: f.GetCreatedAt().AsTime(),
			UpdatedAt: f.GetUpdatedAt().AsTime(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"files":      files,
		"total_size": resp.GetTotalSize(),
	})
}

func statusString(s storagepb.FileStatus) string {
	switch s {
	case storagepb.FileStatus_FILE_STATUS_UPLOADING:
		return "uploading"
	case storagepb.FileStatus_FILE_STATUS_COMPLETE:
		return "complete"
	case storagepb.FileStatus_FILE_STATUS_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}
