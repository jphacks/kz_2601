package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const RequestIDHeader = "X-Request-Id"

const maxRequestIDLen = 64

func RequestID(ctx context.Context) string {
	if m := metaFrom(ctx); m != nil {
		return m.id
	}
	return ""
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		m := &requestMeta{id: id}
		m.traceID, m.spanID, m.sampled = parseTrace(r.Header.Get("traceparent"), r.Header.Get("X-Cloud-Trace-Context"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestMetaKey{}, m)))
	})
}

func sanitizeRequestID(raw string) string {
	if len(raw) > maxRequestIDLen {
		return ""
	}
	for _, c := range raw {
		allowed := c == '-' || c == '_' || c == '.' ||
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !allowed {
			return ""
		}
	}
	return raw
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf[:])
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := asRecorder(w)
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				// Go panic format ("panic: ...\n\ngoroutine N [running]:") so
				// Error Reporting parses the stack (#295).
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.Any("panic", v),
					slog.String("requestId", RequestID(r.Context())),
					slog.String("stack_trace", fmt.Sprintf("panic: %v\n\n%s", v, debug.Stack())),
					errorContext(r, http.StatusInternalServerError),
				)
				if !recorder.wrote {
					Error(recorder, http.StatusInternalServerError, CodeInternal, "internal server error")
				}
			}()
			next.ServeHTTP(recorder, r)
		})
	}
}

func Log(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := asRecorder(w)
			defer func() {
				attrs := []any{
					slog.String("event", RequestEvent),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", recorder.status),
					slog.Duration("duration", time.Since(started)),
					slog.String("requestId", RequestID(r.Context())),
				}
				attrs = append(attrs, ClientAttrs(r)...)
				// 5xx is ERROR so it reaches Error Reporting (#295)
				if recorder.status >= http.StatusInternalServerError {
					attrs = append(attrs, errorContext(r, recorder.status))
					logger.ErrorContext(r.Context(), "request", attrs...)
					return
				}
				logger.InfoContext(r.Context(), "request", attrs...)
			}()
			next.ServeHTTP(recorder, r)
		})
	}
}

func asRecorder(w http.ResponseWriter) *responseRecorder {
	if existing, ok := w.(*responseRecorder); ok {
		return existing
	}
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rec *responseRecorder) WriteHeader(status int) {
	if rec.wrote {
		return
	}
	rec.status = status
	rec.wrote = true
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	if !rec.wrote {
		rec.WriteHeader(http.StatusOK)
	}
	return rec.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush (SSE) through the wrapper.
func (rec *responseRecorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// Flush keeps `w.(http.Flusher)` working for SSE handlers behind Log/Recover.
func (rec *responseRecorder) Flush() {
	if !rec.wrote {
		rec.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(rec.ResponseWriter).Flush()
}

// RequestEvent is the `event` attr of the access log line.
// Cloud Logging filter: jsonPayload.event="http.request".
const RequestEvent = "http.request"

// errorContext is the Error Reporting `context.httpRequest` (path only: the
// query may carry user input).
func errorContext(r *http.Request, status int) slog.Attr {
	return slog.Group("context", slog.Group("httpRequest",
		slog.String("method", r.Method),
		slog.String("url", r.URL.Path),
		slog.Int("responseStatusCode", status),
	))
}

func LogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func ErrorEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&envelopeWriter{ResponseWriter: w}, r)
	})
}

type envelopeWriter struct {
	http.ResponseWriter
	wrote     bool
	discarded bool
}

func (ew *envelopeWriter) WriteHeader(status int) {
	if ew.wrote {
		return
	}
	ew.wrote = true
	code, replace := envelopeCode(status, ew.Header().Get("Content-Type"))
	if !replace {
		ew.ResponseWriter.WriteHeader(status)
		return
	}
	ew.discarded = true
	Error(ew.ResponseWriter, status, code, http.StatusText(status))
}

func (ew *envelopeWriter) Write(b []byte) (int, error) {
	if !ew.wrote {
		ew.WriteHeader(http.StatusOK)
	}
	if ew.discarded {
		return len(b), nil
	}
	return ew.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush (SSE) through the wrapper.
func (ew *envelopeWriter) Unwrap() http.ResponseWriter { return ew.ResponseWriter }

// envelopeCode rewrites only the mux's default text/plain 404/405 bodies;
// a handler that chose its own Content-Type (JSON, HTML page) is left alone.
func envelopeCode(status int, contentType string) (string, bool) {
	if contentType != "" && !strings.HasPrefix(contentType, "text/plain") {
		return "", false
	}
	switch status {
	case http.StatusNotFound:
		return CodeNotFound, true
	case http.StatusMethodNotAllowed:
		return CodeBadRequest, true
	default:
		return "", false
	}
}
