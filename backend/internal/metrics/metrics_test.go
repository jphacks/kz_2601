package metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

func TestUIDHashMatchesAIUsage(t *testing.T) {
	for _, uid := range []string{"", "u1", "firebase-uid-XYZ"} {
		if got, want := metrics.UIDHash(uid), ai.UIDHash(uid); got != want {
			t.Errorf("UIDHash(%q) = %q, ai.UIDHash = %q", uid, got, want)
		}
	}
}

func TestAppHash(t *testing.T) {
	a := metrics.AppHash("app_1")
	if len(a) != 16 || a == metrics.AppHash("app_2") || a == metrics.UIDHash("app_1") {
		t.Fatalf("AppHash: %q", a)
	}
	if metrics.AppHash("") != "" {
		t.Fatal("empty appId must give empty hash")
	}
}

func TestEmit(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	metrics.Emit(context.Background(), metrics.AppShared, "RAW-UID", slog.String("app_hash", metrics.AppHash("RAW-APP")))
	metrics.Emit(context.Background(), metrics.SessionStart, "")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines: %s", buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	if m["event"] != metrics.AppShared || m["msg"] != metrics.AppShared || m["uid_hash"] != metrics.UIDHash("RAW-UID") ||
		m["app_hash"] != metrics.AppHash("RAW-APP") {
		t.Fatalf("line: %v", m)
	}
	if strings.Contains(buf.String(), "RAW-UID") || strings.Contains(buf.String(), "RAW-APP") {
		t.Fatalf("raw id leaked: %s", buf.String())
	}
	if strings.Contains(lines[1], "uid_hash") {
		t.Fatalf("anonymous caller must not get uid_hash: %s", lines[1])
	}
}
