package gateway

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"cloudstore/gen/storagepb"
)

// DialStorage connects to the storage service. The returned conn must be
// closed by the caller on shutdown.
func DialStorage(addr string) (storagepb.StorageServiceClient, *grpc.ClientConn, error) {
	// insecure: both services run on localhost; add TLS when deploying for real.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return storagepb.NewStorageServiceClient(conn), conn, nil
}
