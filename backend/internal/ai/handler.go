package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// NewHandler exposes POST /v1/generate, POST /v1/chat, DELETE /v1/chat/{appId},
// POST /v1/text and GET /health.
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
			svc.writeErr(w, r, &Error{Status: http.StatusBadRequest, Code: httpx.CodeBadRequest, Message: "cannot read body"})
			return
		}
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			svc.writeErr(w, r, &Error{Status: http.StatusBadRequest, Code: httpx.CodeBadRequest, Message: "invalid JSON: " + err.Error()})
			return
		}
		ctx := callerContext(r)
		if q := r.URL.Query().Get("stream"); q == "1" || q == "true" {
			serveSSE(w, func(emit func(string, any)) {
				res, err := svc.GenerateStream(ctx, req, func(ev Event) { emit(ev.Type, ev) })
				if err != nil {
					ge := asError(err)
					// the stream already answered 200: tell the limiter (#548)
					ratelimit.ReportStatus(ctx, ge.Status)
					emit("error", svc.errBody(ctx, ge))
					return
				}
				emit("result", res)
			})
			return
		}
		res, err := svc.Generate(ctx, req)
		if err != nil {
			svc.writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// /v1/chat: the caller (if the token verifies) owns the conversation (#286)
	caller := auth.Optional(svc.Verifier, svc.AuthMode)
	mux.Handle("POST /v1/chat", caller(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			svc.writeErr(w, r, &Error{Status: http.StatusBadRequest, Code: httpx.CodeBadRequest, Message: "cannot read body"})
			return
		}
		var req ChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			svc.writeErr(w, r, &Error{Status: http.StatusBadRequest, Code: httpx.CodeBadRequest, Message: "invalid JSON: " + err.Error()})
			return
		}
		ctx := callerContext(r) // edit mode reads the current DSL
		res, err := svc.Chat(ctx, req)
		if err != nil {
			svc.writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})))
	mux.Handle("POST /v1/text", caller(textHandler(svc))) // #417 aiButton generate (text.go)
	// Only the caller's own conversation can be cleared: no token 401,
	// another user's conversation 403, otherwise 204 (#286).
	mux.Handle("DELETE /v1/chat/{appId}", caller(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UID(r.Context())
		if uid == "" {
			svc.writeErr(w, r, &Error{Status: http.StatusUnauthorized, Code: httpx.CodeUnauthenticated, Message: "missing or invalid bearer token"})
			return
		}
		if err := svc.ClearChat(uid, r.PathValue("appId")); errors.Is(err, ErrNotOwner) {
			svc.writeErr(w, r, &Error{Status: http.StatusForbidden, Code: httpx.CodeForbidden, Message: err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	return mux
}

// callerContext carries the caller's Authorization and App Check token (#318)
// to the appconfig calls made on its behalf.
func callerContext(r *http.Request) context.Context {
	ctx := r.Context()
	if auth := r.Header.Get("Authorization"); auth != "" {
		ctx = appconfigclient.WithBearer(ctx, auth)
	}
	if token := r.Header.Get(appconfigclient.AppCheckHeader); token != "" {
		ctx = appconfigclient.WithAppCheck(ctx, token)
	}
	return ctx
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
		ge = &Error{Status: http.StatusInternalServerError, Code: httpx.CodeInternal, Message: err.Error()}
	}
	return ge
}

// logger is s.Logger, or slog.Default() when unset.
func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// errBody builds the error body. A 5xx is logged at ERROR with ctx, so the
// line carries the request's trace and requestId (#295).
func (s *Service) errBody(ctx context.Context, ge *Error) map[string]any {
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
		s.logger().LogAttrs(ctx, slog.LevelError, "ai: error response",
			slog.Int("status", ge.Status),
			slog.String("code", ge.Code),
			slog.String("error", ge.Message),
		)
	}
	return map[string]any{"error": e}
}

func (s *Service) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	ge := asError(err)
	body := s.errBody(r.Context(), ge)
	delete(body["error"].(map[string]any), "status")
	writeJSON(w, ge.Status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
