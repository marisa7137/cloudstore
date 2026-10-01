package storage

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloudstore/gen/storagepb"
	"cloudstore/internal/storage/db"
)

const maxChunkSize = 2 << 20 // 2 MiB; frontend sends 1 MiB chunks

// Directory layout under baseDir:
//
//	uploads/{user_id}/{file_id}/chunk_000042   temporary chunks while uploading
//	blobs/{user_id}/{file_id}                  final assembled file
func (s *Server) uploadDir(userID, fileID int64) string {
	return filepath.Join(s.baseDir, "uploads", fmt.Sprint(userID), fmt.Sprint(fileID))
}

func (s *Server) blobPath(userID, fileID int64) string {
	return filepath.Join(s.baseDir, "blobs", fmt.Sprint(userID), fmt.Sprint(fileID))
}

func chunkName(index int32) string {
	return fmt.Sprintf("chunk_%06d", index)
}

func (s *Server) InitUpload(ctx context.Context, req *storagepb.InitUploadRequest) (*storagepb.InitUploadResponse, error) {
	switch {
	case req.GetUserId() <= 0:
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	case req.GetName() == "":
		return nil, status.Error(codes.InvalidArgument, "name is required")
	case req.GetChunkCount() <= 0:
		return nil, status.Error(codes.InvalidArgument, "chunk_count must be positive")
	case req.GetSize() < 0:
		return nil, status.Error(codes.InvalidArgument, "size must not be negative")
	}

	file, err := s.db.CreateFile(req.GetUserId(), req.GetName(), req.GetMimeType(), req.GetSize())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create file: %v", err)
	}
	task, err := s.db.CreateUploadTask(req.GetUserId(), file.ID, int(req.GetChunkCount()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
	}
	if err := os.MkdirAll(s.uploadDir(file.UserID, file.ID), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "create upload dir: %v", err)
	}

	return &storagepb.InitUploadResponse{FileId: file.ID, TaskId: task.ID}, nil
}

func (s *Server) UploadChunk(ctx context.Context, req *storagepb.UploadChunkRequest) (*storagepb.UploadChunkResponse, error) {
	if len(req.GetData()) > maxChunkSize {
		return nil, status.Errorf(codes.InvalidArgument, "chunk larger than %d bytes", maxChunkSize)
	}

	file, task, err := s.uploadState(req.GetUserId(), req.GetFileId())
	if err != nil {
		return nil, err
	}
	idx := req.GetChunkIndex()
	if idx < 0 || int(idx) >= task.ChunksTotal {
		return nil, status.Errorf(codes.InvalidArgument,
			"chunk_index %d out of range [0, %d)", idx, task.ChunksTotal)
	}

	path := filepath.Join(s.uploadDir(file.UserID, file.ID), chunkName(idx))

	// A retried chunk overwrites the old one but must not be counted twice.
	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, os.ErrNotExist)

	if err := os.WriteFile(path, req.GetData(), 0o644); err != nil {
		return nil, status.Errorf(codes.Internal, "write chunk: %v", err)
	}

	received := task.ChunksReceived
	if isNew {
		if received, err = s.db.IncrementTaskChunks(task.ID); err != nil {
			return nil, status.Errorf(codes.Internal, "update task: %v", err)
		}
	}

	return &storagepb.UploadChunkResponse{
		ChunksReceived: int32(received),
		ChunksTotal:    int32(task.ChunksTotal),
	}, nil
}

func (s *Server) CompleteUpload(ctx context.Context, req *storagepb.CompleteUploadRequest) (*storagepb.FileInfo, error) {
	file, task, err := s.uploadState(req.GetUserId(), req.GetFileId())
	if err != nil {
		return nil, err
	}
	if task.ChunksReceived != task.ChunksTotal {
		return nil, status.Errorf(codes.FailedPrecondition,
			"only %d of %d chunks received", task.ChunksReceived, task.ChunksTotal)
	}

	size, sum, err := s.assemble(file, task)
	if err != nil {
		// Leave chunks on disk for inspection; mark file + task failed.
		_ = s.db.FailFile(file.ID)
		_ = s.db.SetTaskStatus(task.ID, db.TaskFailed)
		return nil, status.Errorf(codes.Internal, "assemble: %v", err)
	}

	if err := s.db.CompleteFile(file.ID, size, sum); err != nil {
		return nil, status.Errorf(codes.Internal, "complete file: %v", err)
	}
	if err := s.db.SetTaskStatus(task.ID, db.TaskComplete); err != nil {
		return nil, status.Errorf(codes.Internal, "complete task: %v", err)
	}
	// Temp chunks are no longer needed.
	_ = os.RemoveAll(s.uploadDir(file.UserID, file.ID))

	done, err := s.db.GetFile(file.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "reload file: %v", err)
	}
	return toProto(done), nil
}

// assemble concatenates all chunks in order into the final blob,
// returning its size and md5 hex checksum.
func (s *Server) assemble(file *db.File, task *db.Task) (int64, string, error) {
	blobPath := s.blobPath(file.UserID, file.ID)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return 0, "", err
	}
	out, err := os.Create(blobPath)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()

	hash := md5.New()
	dst := io.MultiWriter(out, hash) // write once, hash alongside
	var size int64

	for i := 0; i < task.ChunksTotal; i++ {
		chunk, err := os.Open(filepath.Join(s.uploadDir(file.UserID, file.ID), chunkName(int32(i))))
		if err != nil {
			return 0, "", err
		}
		n, err := io.Copy(dst, chunk)
		chunk.Close()
		if err != nil {
			return 0, "", err
		}
		size += n
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

// uploadState loads file + task and checks the file is owned by the user
// and still accepting chunks.
func (s *Server) uploadState(userID, fileID int64) (*db.File, *db.Task, error) {
	if userID <= 0 || fileID <= 0 {
		return nil, nil, status.Error(codes.InvalidArgument, "user_id and file_id are required")
	}
	file, err := s.db.GetUserFile(fileID, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil, status.Error(codes.NotFound, "file not found")
		}
		return nil, nil, status.Errorf(codes.Internal, "load file: %v", err)
	}
	if file.Status != db.StatusUploading {
		return nil, nil, status.Errorf(codes.FailedPrecondition, "file is %s, not uploading", file.Status)
	}
	task, err := s.db.GetUploadTaskByFile(file.ID)
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "load task: %v", err)
	}
	return file, task, nil
}
