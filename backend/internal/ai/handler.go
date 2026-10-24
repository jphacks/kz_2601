package ai

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// NewHandler exposes POST /v1/generate and GET /health.
// Cloud Run's front end swallows /healthz, so /health is the one to probe.
func NewHandler(svc *Service) http.Handler {
	mux := http.NewServeMux()
	health := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("POST /v1/generate", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "cannot read body"})
			return
		}
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "invalid JSON: " + err.Error()})
			return
		}
		ctx := r.Context()
		if auth := r.Header.Get("Authorization"); auth != "" {
			ctx = appconfigclient.WithBearer(ctx, auth)
		}
		res, err := svc.Generate(ctx, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return mux
}

func writeErr(w http.ResponseWriter, err error) {
	var ge *Error
	if !errors.As(err, &ge) {
		ge = &Error{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
	}
	details := map[string]any{}
	if len(ge.Problems) > 0 {
		details["problems"] = ge.Problems
	}
	if ge.Attempts > 0 {
		details["attempts"] = ge.Attempts
	}
	body := map[string]any{"error": map[string]any{"code": ge.Code, "message": ge.Message}}
	if len(details) > 0 {
		body["error"].(map[string]any)["details"] = details
	}
	if ge.Status >= 500 {
		log.Printf("generate: %d %s: %s", ge.Status, ge.Code, ge.Message)
	}
	writeJSON(w, ge.Status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
