package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const RequestIDHeader = "X-Request-Id"

const maxRequestIDLen = 64

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey{}).(string)
	return s
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
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
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.Any("panic", v),
					slog.String("requestId", RequestID(r.Context())),
					slog.String("stack", string(debug.Stack())),
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
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", recorder.status),
					slog.Duration("duration", time.Since(started)),
					slog.String("requestId", RequestID(r.Context())),
				}
				logger.InfoContext(r.Context(), "request", append(attrs, clientAttrs(r)...)...)
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

func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: LogLevel(os.Getenv("LOG_LEVEL"))}))
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

func envelopeCode(status int, contentType string) (string, bool) {
	if strings.HasPrefix(contentType, "application/json") {
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
