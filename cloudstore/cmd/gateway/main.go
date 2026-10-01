package main

import (
	"log"
	"net/http"

	"cloudstore/internal/gateway"
	"cloudstore/internal/gateway/db"
)

func main() {
	database, err := db.Open("data/gateway.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	storageClient, conn, err := gateway.DialStorage("localhost:9090")
	if err != nil {
		log.Fatalf("dial storage: %v", err)
	}
	defer conn.Close()

	srv := gateway.NewServer(database, storageClient)

	addr := ":8080"
	log.Printf("gateway listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}
