package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// A shared app's run writes one shared.usage notify line per recipient
// reached: hashes, kind and count only, never the text or the ids.
func TestDispatchCountsShared(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	was := metrics.SetSharedUsage(true)
	t.Cleanup(func() { slog.SetDefault(prev); metrics.SetSharedUsage(was) })

	tokens := MemoryTokens{"uid-a": {"ta"}, "uid-b": {"tb"}}
	c := Content{Key: "k", AppID: "APP-RAW", AppName: "家計簿", Title: "SECRET-TITLE", Body: "SECRET-BODY"}
	res := Resolution{To: []string{"uid-a", "uid-b", "uid-none"}}
	// without CountShared (notices to the creator): nothing
	if _, err := (&Dispatcher{Tokens: tokens, Sender: &FakeSender{}}).Dispatch(context.Background(), res, c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), metrics.SharedUsage) {
		t.Fatalf("counted without CountShared: %s", buf.String())
	}
	if _, err := (&Dispatcher{Tokens: tokens, Sender: &FakeSender{}, CountShared: true}).Dispatch(context.Background(), res, c); err != nil {
		t.Fatal(err)
	}
	var who []string
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil || m["event"] != metrics.SharedUsage {
			continue
		}
		if m["kind"] != metrics.SharedNotify || m["count"] != float64(1) || m["app_hash"] != metrics.AppHash("APP-RAW") {
			t.Fatalf("line %v", m)
		}
		who = append(who, m["uid_hash"].(string))
	}
	if len(who) != 2 || who[0] != metrics.UIDHash("uid-a") || who[1] != metrics.UIDHash("uid-b") { // no device: not reached
		t.Fatalf("recipients %v", who)
	}
	for _, leak := range []string{"SECRET", "APP-RAW", "uid-a", "家計簿"} {
		if strings.Contains(buf.String(), leak) {
			t.Fatalf("%q in the log: %s", leak, buf.String())
		}
	}
}
