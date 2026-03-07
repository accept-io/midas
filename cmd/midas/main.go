package main

import (
	"log"

	"github.com/accept-io/midas/internal/httpapi"
)

func main() {
	srv := httpapi.NewServer()

	log.Println("MIDAS listening on :8080")
	log.Fatal(srv.ListenAndServe(":8080"))
}
