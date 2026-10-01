package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	"cloudstore/gen/storagepb"
	"cloudstore/internal/storage"
	"cloudstore/internal/storage/db"
)

func main() {
	database, err := db.Open("data/storage.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	storagepb.RegisterStorageServiceServer(grpcServer, storage.NewServer(database, "data"))

	log.Printf("storage service listening on %s", lis.Addr())
	log.Fatal(grpcServer.Serve(lis))
}
