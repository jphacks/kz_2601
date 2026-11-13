package metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

func TestEmitSharedUsage(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	ctx := context.Background()

	// off by default
	was := metrics.SetSharedUsage(false)
	t.Cleanup(func() { metrics.SetSharedUsage(was) })
	metrics.EmitSharedUsage(ctx, "RAW-APP", "RAW-UID", metrics.SharedRead, 3)
	if buf.Len() != 0 {
		t.Fatalf("off but wrote: %s", buf.String())
	}

	metrics.SetSharedUsage(true)
	metrics.EmitSharedUsage(ctx, "RAW-APP", "RAW-UID", metrics.SharedRead, 3)
	metrics.EmitSharedUsage(ctx, "RAW-APP", "RAW-UID", metrics.SharedWrite, 0) // nothing happened
	metrics.EmitSharedUsage(ctx, "", "RAW-UID", metrics.SharedWrite, 1)        // no app
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line: %s", buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	if m["event"] != metrics.SharedUsage || m["app_hash"] != metrics.AppHash("RAW-APP") || m["uid_hash"] != metrics.UIDHash("RAW-UID") ||
		m["kind"] != metrics.SharedRead || m["count"] != float64(3) {
		t.Fatalf("line: %v", m)
	}
	for k := range m {
		switch k {
		case "time", "level", "msg", "event", "app_hash", "uid_hash", "kind", "count":
		default:
			t.Fatalf("unexpected field %q: %v", k, m)
		}
	}
	if strings.Contains(buf.String(), "RAW-") {
		t.Fatalf("raw id leaked: %s", buf.String())
	}
}
