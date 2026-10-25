package ai

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// NewHandler exposes POST /v1/generate, POST /v1/chat, DELETE /v1/chat/{appId}
// and GET /health.
// Cloud Run's front end swallows /healthz, so /health is the one to probe.
//
// POST /v1/generate?stream=1 answers with Server-Sent Events instead of one
// JSON body: `event: app` / `event: summary` (an Event, as soon as the first
// block of the model output is complete), then `event: result` (the same
// GenerateResponse as the JSON mode) or `event: error` (the same error body).
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
		if q := r.URL.Query().Get("stream"); q == "1" || q == "true" {
			serveSSE(w, func(emit func(string, any)) {
				res, err := svc.GenerateStream(ctx, req, func(ev Event) { emit(ev.Type, ev) })
				if err != nil {
					ge := asError(err)
					emit("error", errBody(ge))
					return
				}
				emit("result", res)
			})
			return
		}
		res, err := svc.Generate(ctx, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /v1/chat", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "cannot read body"})
			return
		}
		var req ChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "invalid JSON: " + err.Error()})
			return
		}
		ctx := r.Context()
		if auth := r.Header.Get("Authorization"); auth != "" {
			ctx = appconfigclient.WithBearer(ctx, auth) // edit mode reads the current DSL
		}
		res, err := svc.Chat(ctx, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("DELETE /v1/chat/{appId}", func(w http.ResponseWriter, r *http.Request) {
		svc.ClearChat(r.PathValue("appId"))
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// serveSSE runs fn with an emitter that writes `event:`/`data:` frames and
// flushes after each one (Cloud Run streams chunked HTTP/1.1 responses).
func serveSSE(w http.ResponseWriter, fn func(emit func(event string, v any))) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	emit := func(event string, v any) {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = w.Write([]byte("event: " + event + "\ndata: "))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	fn(emit)
}

func asError(err error) *Error {
	var ge *Error
	if !errors.As(err, &ge) {
		ge = &Error{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
	}
	return ge
}

func errBody(ge *Error) map[string]any {
	details := map[string]any{}
	if len(ge.Problems) > 0 {
		details["problems"] = ge.Problems
	}
	if ge.Attempts > 0 {
		details["attempts"] = ge.Attempts
	}
	e := map[string]any{"code": ge.Code, "message": ge.Message, "status": ge.Status}
	if len(details) > 0 {
		e["details"] = details
	}
	if ge.Status >= 500 {
		log.Printf("generate: %d %s: %s", ge.Status, ge.Code, ge.Message)
	}
	return map[string]any{"error": e}
}

func writeErr(w http.ResponseWriter, err error) {
	ge := asError(err)
	body := errBody(ge)
	delete(body["error"].(map[string]any), "status")
	writeJSON(w, ge.Status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
