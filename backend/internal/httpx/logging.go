package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// Cloud Logging structured-log keys (#295).
// https://cloud.google.com/logging/docs/structured-logging
const (
	keySeverity     = "severity"
	keyMessage      = "message"
	keyTrace        = "logging.googleapis.com/trace"
	keySpanID       = "logging.googleapis.com/spanId"
	keyTraceSampled = "logging.googleapis.com/trace_sampled"
	keyType         = "@type"
	keyService      = "serviceContext"
	keyRequestID    = "requestId"
	keyUIDHash      = "uid_hash"

	// ReportedErrorEventType makes Error Reporting pick up an entry even
	// without a parsable stack trace.
	ReportedErrorEventType = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"
)

// requestMeta is what the log handler adds to every line logged with the
// request context. Set once by WithRequestID; uidHash is filled later by
// auth (NoteUID), so the outer access log sees it too.
type requestMeta struct {
	id      string
	traceID string // 32 hex
	spanID  string // 16 hex
	sampled bool
	uidHash atomic.Pointer[string]
}

type requestMetaKey struct{}

func metaFrom(ctx context.Context) *requestMeta {
	if ctx == nil {
		return nil
	}
	m, _ := ctx.Value(requestMetaKey{}).(*requestMeta)
	return m
}

// UIDHash is the pseudonymous user key in logs, never the raw uid. The
// "ai-usage:" prefix predates #295 (ai.usage, #298) and is kept so request
// logs and ai.usage lines share the same key. Empty uid gives "".
func UIDHash(uid string) string {
	if uid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("ai-usage:" + uid))
	return hex.EncodeToString(sum[:8])
}

// NoteUID records the verified caller (as UIDHash) on the request so every
// log line of the request carries uid_hash. No-op outside WithRequestID.
func NoteUID(ctx context.Context, uid string) {
	if m := metaFrom(ctx); m != nil {
		h := UIDHash(uid)
		m.uidHash.Store(&h)
	}
}

// parseTrace reads W3C traceparent first, then X-Cloud-Trace-Context
// ("TRACE/SPAN_DECIMAL;o=1"). Invalid values give "".
func parseTrace(traceparent, cloud string) (traceID, spanID string, sampled bool) {
	if p := strings.Split(strings.TrimSpace(traceparent), "-"); len(p) == 4 &&
		len(p[0]) == 2 && isHex(p[1], 32) && isHex(p[2], 16) && len(p[3]) == 2 {
		flags, err := strconv.ParseUint(p[3], 16, 8)
		if err == nil {
			return strings.ToLower(p[1]), strings.ToLower(p[2]), flags&1 == 1
		}
	}
	raw := strings.TrimSpace(cloud)
	if raw == "" {
		return "", "", false
	}
	rest := ""
	traceID, rest, _ = strings.Cut(raw, "/")
	if !isHex(traceID, 32) {
		return "", "", false
	}
	span, opts, _ := strings.Cut(rest, ";")
	if n, err := strconv.ParseUint(span, 10, 64); err == nil && n != 0 {
		spanID = strconv.FormatUint(n, 16)
		spanID = strings.Repeat("0", 16-len(spanID)) + spanID
	}
	return strings.ToLower(traceID), spanID, opts == "o=1"
}

func isHex(s string, n int) bool {
	if len(s) != n || strings.Trim(s, "0") == "" {
		return false
	}
	for _, c := range s {
		ok := c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		if !ok {
			return false
		}
	}
	return true
}

// CloudConfig names where the logs come from. Empty fields are left out.
type CloudConfig struct {
	ProjectID string // for logging.googleapis.com/trace
	Service   string // Error Reporting serviceContext.service
	Version   string // serviceContext.version
}

// CloudConfigFromEnv reads GOOGLE_CLOUD_PROJECT and the Cloud Run K_SERVICE /
// K_REVISION.
func CloudConfigFromEnv(getenv func(string) string) CloudConfig {
	return CloudConfig{
		ProjectID: getenv("GOOGLE_CLOUD_PROJECT"),
		Service:   getenv("K_SERVICE"),
		Version:   getenv("K_REVISION"),
	}
}

// NewCloudHandler writes Cloud Logging JSON: severity / message, the trace of
// the request, requestId and uid_hash from the context, and for ERROR and
// above the ReportedErrorEvent @type + serviceContext so Error Reporting
// collects them. Keys a record already has are not added twice.
func NewCloudHandler(w io.Writer, level slog.Leveler, cfg CloudConfig) slog.Handler {
	inner := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: cloudReplaceAttr})
	return &cloudHandler{inner: inner, cfg: cfg}
}

func cloudReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		lv, _ := a.Value.Any().(slog.Level)
		return slog.String(keySeverity, severity(lv))
	case slog.MessageKey:
		a.Key = keyMessage
	}
	return a
}

func severity(lv slog.Level) string {
	switch {
	case lv >= slog.LevelError+4:
		return "CRITICAL"
	case lv >= slog.LevelError:
		return "ERROR"
	case lv >= slog.LevelWarn:
		return "WARNING"
	case lv >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

type cloudHandler struct {
	inner slog.Handler
	cfg   CloudConfig
}

func (h *cloudHandler) Enabled(ctx context.Context, lv slog.Level) bool {
	return h.inner.Enabled(ctx, lv)
}

func (h *cloudHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &cloudHandler{inner: h.inner.WithAttrs(attrs), cfg: h.cfg}
}

func (h *cloudHandler) WithGroup(name string) slog.Handler {
	return &cloudHandler{inner: h.inner.WithGroup(name), cfg: h.cfg}
}

func (h *cloudHandler) Handle(ctx context.Context, r slog.Record) error {
	has := map[string]bool{}
	r.Attrs(func(a slog.Attr) bool {
		has[a.Key] = true
		return true
	})
	add := func(a slog.Attr) {
		if !has[a.Key] {
			has[a.Key] = true
			r.AddAttrs(a)
		}
	}
	if m := metaFrom(ctx); m != nil {
		if m.traceID != "" && h.cfg.ProjectID != "" {
			add(slog.String(keyTrace, "projects/"+h.cfg.ProjectID+"/traces/"+m.traceID))
			if m.spanID != "" {
				add(slog.String(keySpanID, m.spanID))
			}
			add(slog.Bool(keyTraceSampled, m.sampled))
		}
		if m.id != "" {
			add(slog.String(keyRequestID, m.id))
		}
		if p := m.uidHash.Load(); p != nil && *p != "" {
			add(slog.String(keyUIDHash, *p))
		}
	}
	if r.Level >= slog.LevelError {
		add(slog.String(keyType, ReportedErrorEventType))
		if h.cfg.Service != "" {
			sc := []any{slog.String("service", h.cfg.Service)}
			if h.cfg.Version != "" {
				sc = append(sc, slog.String("version", h.cfg.Version))
			}
			add(slog.Group(keyService, sc...))
		}
	}
	return h.inner.Handle(ctx, r)
}

// NewLogger is the service logger: Cloud Logging JSON on stdout, level from
// LOG_LEVEL.
func NewLogger() *slog.Logger {
	return newLogger(os.Stdout, os.Getenv)
}

func newLogger(w io.Writer, getenv func(string) string) *slog.Logger {
	return slog.New(NewCloudHandler(w, LogLevel(getenv("LOG_LEVEL")), CloudConfigFromEnv(getenv)))
}
