package storage

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cloudstore/gen/storagepb"
	"cloudstore/internal/storage/db"
)

// Server implements the StorageService gRPC interface.
type Server struct {
	storagepb.UnimplementedStorageServiceServer
	db      *db.DB
	baseDir string // root for uploads/ (temp chunks) and blobs/ (final files)
}

func NewServer(d *db.DB, baseDir string) *Server {
	return &Server{db: d, baseDir: baseDir}
}

func (s *Server) ListFiles(ctx context.Context, req *storagepb.ListFilesRequest) (*storagepb.ListFilesResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	files, err := s.db.ListUserFiles(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list files: %v", err)
	}

	resp := &storagepb.ListFilesResponse{}
	for _, f := range files {
		resp.Files = append(resp.Files, toProto(f))
		if f.Status == db.StatusComplete {
			resp.TotalSize += f.Size
		}
	}
	return resp, nil
}

func (s *Server) ListTasks(ctx context.Context, req *storagepb.ListTasksRequest) (*storagepb.ListTasksResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	tasks, err := s.db.ListUserTasks(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tasks: %v", err)
	}

	resp := &storagepb.ListTasksResponse{}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, &storagepb.TaskInfo{
			Id:             t.ID,
			FileId:         t.FileID,
			FileName:       t.FileName,
			FileSize:       t.FileSize,
			Type:           t.Type,
			Status:         toProtoTaskStatus(t.Status),
			ChunksTotal:    int32(t.ChunksTotal),
			ChunksReceived: int32(t.ChunksReceived),
			CreatedAt:      timestamppb.New(t.CreatedAt),
			UpdatedAt:      timestamppb.New(t.UpdatedAt),
		})
	}
	return resp, nil
}

func toProtoTaskStatus(s db.TaskStatus) storagepb.TaskStatus {
	switch s {
	case db.TaskInProgress:
		return storagepb.TaskStatus_TASK_STATUS_IN_PROGRESS
	case db.TaskComplete:
		return storagepb.TaskStatus_TASK_STATUS_COMPLETE
	case db.TaskFailed:
		return storagepb.TaskStatus_TASK_STATUS_FAILED
	default:
		return storagepb.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func toProto(f *db.File) *storagepb.FileInfo {
	return &storagepb.FileInfo{
		Id:        f.ID,
		UserId:    f.UserID,
		Name:      f.Name,
		Size:      f.Size,
		Md5:       f.MD5,
		MimeType:  f.MimeType,
		Status:    toProtoStatus(f.Status),
		CreatedAt: timestamppb.New(f.CreatedAt),
		UpdatedAt: timestamppb.New(f.UpdatedAt),
	}
}

func toProtoStatus(s db.FileStatus) storagepb.FileStatus {
	switch s {
	case db.StatusUploading:
		return storagepb.FileStatus_FILE_STATUS_UPLOADING
	case db.StatusComplete:
		return storagepb.FileStatus_FILE_STATUS_COMPLETE
	case db.StatusFailed:
		return storagepb.FileStatus_FILE_STATUS_FAILED
	default:
		return storagepb.FileStatus_FILE_STATUS_UNSPECIFIED
	}
}
