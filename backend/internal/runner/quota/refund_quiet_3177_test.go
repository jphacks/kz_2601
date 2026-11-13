package quota

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// logRecorder is a slog.Handler that keeps every record.
type logRecorder_3177 struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *logRecorder_3177) Enabled(context.Context, slog.Level) bool { return true }
func (h *logRecorder_3177) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *logRecorder_3177) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logRecorder_3177) WithGroup(string) slog.Handler      { return h }

func (h *logRecorder_3177) records() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.recs...)
}

func chargeOf_3177() Charge {
	return Charge{Parts: []ChargePart{{
		Counters: []ratelimit.Counter{{ID: "c1", Scope: "test", Limit: 10}},
		Weight:   1,
		ExpireAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	}}}
}

func TestRefundQuiet_3177(t *testing.T) {
	t.Parallel()

	t.Run("返すのが成功ならログは出ない", func(t *testing.T) {
		t.Parallel()
		h := &logRecorder_3177{}
		q := New(Config{}, nil, everyonePro{})
		q.Logger = slog.New(h)
		q.refundQuiet(context.Background(), chargeOf_3177())
		if got := h.records(); len(got) != 0 {
			t.Errorf("ログが %d 件出た: %v", len(got), got)
		}
	})

	t.Run("何も数えていなければログは出ない", func(t *testing.T) {
		t.Parallel()
		h := &logRecorder_3177{}
		q := New(Config{}, failing{}, everyonePro{})
		q.Logger = slog.New(h)
		q.refundQuiet(context.Background(), Charge{})
		if got := h.records(); len(got) != 0 {
			t.Errorf("ログが %d 件出た: %v", len(got), got)
		}
	})

	t.Run("失敗してもエラーを上げず警告のログを 1 行", func(t *testing.T) {
		t.Parallel()
		h := &logRecorder_3177{}
		q := New(Config{}, failing{}, everyonePro{})
		q.Logger = slog.New(h)
		// 戻り値は無い。パニックせず戻ること自体が「エラーを上げない」の確認。
		q.refundQuiet(context.Background(), chargeOf_3177())
		got := h.records()
		if len(got) != 1 {
			t.Fatalf("ログが %d 件, want 1", len(got))
		}
		if got[0].Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", got[0].Level)
		}
		if got[0].Message != "runner quota: refund after an allowance stop failed" {
			t.Errorf("message = %q", got[0].Message)
		}
		var errAttr string
		got[0].Attrs(func(a slog.Attr) bool {
			if a.Key == "error" {
				errAttr = a.Value.String()
			}
			return true
		})
		if errAttr != "unavailable" {
			t.Errorf("error 属性 = %q, want %q", errAttr, "unavailable")
		}
	})
}
