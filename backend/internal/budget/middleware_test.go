package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestTapWithGroup(t *testing.T) {
	ctx := context.Background()
	b, st, _ := newTestBreaker(100, "")
	var buf bytes.Buffer
	lg := slog.New(b.Tap(slog.NewJSONHandler(&buf, nil), "ai.usage")).WithGroup("g").With("svc", "ai")
	lg.LogAttrs(ctx, slog.LevelInfo, "ai.usage", slog.Float64("est_cost_usd", 0.25))
	<-st.added
	lg.LogAttrs(ctx, slog.LevelInfo, "ai.generate", slog.Float64("est_cost_usd", 9))

	if got := b.Spent(ctx); got != 0.25 {
		t.Fatalf("spent = %v, want 0.25", got)
	}

	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		lines = append(lines, rec)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2: %s", len(lines), buf.String())
	}
	for _, rec := range lines {
		g, ok := rec["g"].(map[string]any)
		if !ok {
			t.Fatalf("the group is gone: %v", rec)
		}
		if g["svc"] != "ai" || g["est_cost_usd"] == nil {
			t.Fatalf("the attrs are gone: %v", rec)
		}
	}
}
