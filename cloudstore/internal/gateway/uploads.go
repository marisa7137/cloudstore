package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloudstore/gen/storagepb"
)

const maxChunkBody = 2 << 20 // keep in sync with storage service limit

// storageLimit is the per-user quota. Hardcoded for now; later this could
// live on the user row (plans, admin overrides, ...).
// Explicitly int64 so raising it past 2 GiB can never overflow a 32-bit int.
const storageLimit int64 = 1 << 30 // 1 GiB

// POST /api/uploads
func (s *Server) handleInitUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		MimeType   string `json:"mime_type"`
		ChunkCount int32  `json:"chunk_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Quota check: in-progress uploads already count at their declared size,
	// so parallel uploads can't collectively overshoot the limit (much).
	usage, err := s.storage.GetUsage(ctx, &storagepb.UsageRequest{
		UserId: currentUser(r).ID,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	if usage.GetUsedBytes()+req.Size > storageLimit {
		writeJSON(w, http.StatusInsufficientStorage, map[string]any{
			"error":     "storage limit exceeded",
			"limit":     storageLimit,
			"used":      usage.GetUsedBytes(),
			"remaining": max(storageLimit-usage.GetUsedBytes(), 0),
		})
		return
	}

	resp, err := s.storage.InitUpload(ctx, &storagepb.InitUploadRequest{
		UserId:     currentUser(r).ID,
		Name:       req.Name,
		MimeType:   req.MimeType,
		Size:       req.Size,
		ChunkCount: req.ChunkCount,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{
		"file_id": resp.GetFileId(),
		"task_id": resp.GetTaskId(),
	})
}

// PUT /api/uploads/{id}/chunks/{index}  (raw chunk bytes as body)
func (s *Server) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	fileID, err1 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	index, err2 := strconv.ParseInt(r.PathValue("index"), 10, 32)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid file id or chunk index")
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "chunk too large")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.storage.UploadChunk(ctx, &storagepb.UploadChunkRequest{
		UserId:     currentUser(r).ID,
		FileId:     fileID,
		ChunkIndex: int32(index),
		Data:       data,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int32{
		"chunks_received": resp.GetChunksReceived(),
		"chunks_total":    resp.GetChunksTotal(),
	})
}

// POST /api/uploads/{id}/complete
func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	file, err := s.storage.CompleteUpload(ctx, &storagepb.CompleteUploadRequest{
		UserId: currentUser(r).ID,
		FileId: fileID,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fileJSON{
		ID:        file.GetId(),
		Name:      file.GetName(),
		Size:      file.GetSize(),
		MD5:       file.GetMd5(),
		MimeType:  file.GetMimeType(),
		Status:    statusString(file.GetStatus()),
		CreatedAt: file.GetCreatedAt().AsTime(),
		UpdatedAt: file.GetUpdatedAt().AsTime(),
	})
}

// writeGRPCError maps gRPC status codes from the storage service onto
// sensible HTTP status codes.
func writeGRPCError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeError(w, http.StatusBadGateway, "storage service unavailable")
		return
	}
	httpCode := http.StatusBadGateway
	switch st.Code() {
	case codes.InvalidArgument:
		httpCode = http.StatusBadRequest
	case codes.NotFound:
		httpCode = http.StatusNotFound
	case codes.FailedPrecondition:
		httpCode = http.StatusConflict
	}
	writeError(w, httpCode, st.Message())
}
