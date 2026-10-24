package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	// Cloud Run の Google Front End は /healthz をコンテナに届けない(HTML 404)ため、正式なヘルスパスは /health。
	// /healthz はローカル用に残す。
	health := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	log.Printf("ai listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
