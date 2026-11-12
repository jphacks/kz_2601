package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage/eval"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// fakeModel classifies every text as one bug cluster (no live AI calls).
type fakeModel struct {
	category, cluster, summary string
	calls                      int
}

var reLocalID = regexp.MustCompile(`id="(i[0-9]+)"`)

func (f *fakeModel) Complete(_ context.Context, _, user string, _ json.RawMessage) (string, triage.Usage, error) {
	f.calls++
	rows := []map[string]any{}
	for _, m := range reLocalID.FindAllStringSubmatch(user, -1) {
		rows = append(rows, map[string]any{"id": m[1], "category": f.category, "area": "render", "impact": "high",
			"cluster": f.cluster, "summary": f.summary, "pii_suspected": false})
	}
	b, _ := json.Marshal(map[string]any{"items": rows})
	return string(b), triage.Usage{InputTokens: 10, OutputTokens: 5}, nil
}

func seedFeedback(t *testing.T, ta *testApp, msgs ...string) {
	t.Helper()
	for i, m := range msgs {
		if _, err := ta.fs.CreateFeedback(context.Background(), store.Feedback{UID: "u" + string(rune('a'+i)), Kind: "bug", Message: m,
			ConsentVersion: "feedback-2027-01", Device: &store.FeedbackDevice{AppVersion: "1.4.2", OS: "iOS 26.1"}}, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTriageCommand(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01"})
	fm := &fakeModel{category: "bug", cluster: "photo-add-crash", summary: "写真を追加するとアプリが終了する。"}
	ta.model = fm
	seedFeedback(t, ta, "写真を追加すると落ちる", "写真の追加で落ちます")
	ta.must(t, "ingest")
	out := ta.must(t, "triage")
	if !strings.Contains(out, "triaged 2") || fm.calls != 1 {
		t.Fatalf("out %s calls %d", out, fm.calls)
	}
	items, _ := ta.ls.ListItems(context.Background(), loop.ItemQuery{Status: loop.ItemTriaged})
	if len(items) != 2 {
		t.Fatalf("triaged %d", len(items))
	}

	labels := filepath.Join(t.TempDir(), "labels.csv")
	if err := os.WriteFile(labels, []byte(items[0].ID+",bug\n"+items[1].ID+",feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out = ta.must(t, "triage-score", "-labels", labels)
	if !strings.Contains(out, "1/2 agree") {
		t.Fatalf("score %s", out)
	}
}

func TestTriageNeedsLoopKey(t *testing.T) {
	ta := newTestApp(t, env{"ANTHROPIC_API_KEY": "app-key-is-not-used"})
	if err := ta.run(context.Background(), []string{"triage"}); err == nil || !strings.Contains(err.Error(), envTriageKey) {
		t.Fatalf("err %v", err)
	}
}

const labelledSet = "../../internal/loop/triage/eval/testdata/labeled.jsonl"

func TestTriageEvalCommand(t *testing.T) {
	ta := newTestApp(t, env{})
	out := ta.must(t, "triage-eval", "-set", labelledSet)
	if !strings.Contains(out, "28 cases") || !strings.Contains(out, "category: 26/28") {
		t.Fatalf("out %s", out)
	}
	// rules is offline: no tokens counted
	if used, _ := ta.ctl.Usage(context.Background(), loop.Month(ta.now())); used != 0 {
		t.Fatalf("usage %d", used)
	}
	// nothing reaches the loop's store
	if items, _ := ta.ls.ListItems(context.Background(), loop.ItemQuery{}); len(items) != 0 {
		t.Fatalf("items written: %d", len(items))
	}
	if strings.Contains(out, "embedding merge") {
		t.Fatalf("-embed is off by default: %s", out)
	}
	out = ta.must(t, "triage-eval", "-set", labelledSet, "-embed", "fake", "-embed-threshold", "0.9")
	if !strings.Contains(out, "embedding merge (cosine >= 0.90)") || !strings.Contains(out, "before ") || !strings.Contains(out, "after ") {
		t.Fatalf("-embed fake: %s", out)
	}
	for _, bad := range [][]string{{"-embed", "openai"}, {"-embed", "fake", "-embed-threshold", "0"}} {
		if err := ta.run(context.Background(), append([]string{"triage-eval", "-set", labelledSet}, bad...)); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}

	// -model ai uses the (fake) triage model and counts its tokens
	fm := &fakeModel{category: "bug", cluster: "all", summary: "落ちる。"}
	ta.model = fm
	out = ta.must(t, "triage-eval", "-set", labelledSet, "-model", "ai")
	if fm.calls == 0 || !strings.Contains(out, "category: 12/28") {
		t.Fatalf("calls %d out %s", fm.calls, out)
	}
	if used, _ := ta.ctl.Usage(context.Background(), loop.Month(ta.now())); used == 0 {
		t.Fatal("ai eval tokens not counted")
	}
}

// -model ai checks the monthly cap before every call and records each
// call's usage: the cap reached after the first call stops the second.
func TestTriageEvalAIStopsMidRun(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_MONTHLY_TOKEN_CAP": "15"})
	fm := &fakeModel{category: "bug", cluster: "all", summary: "落ちる。"} // 15 tokens a call
	ta.model = fm
	err := ta.run(context.Background(), []string{"triage-eval", "-set", labelledSet, "-model", "ai"})
	if !errors.Is(err, eval.ErrStopped) || fm.calls != 1 {
		t.Fatalf("err %v calls %d", err, fm.calls)
	}
	if used, _ := ta.ctl.Usage(context.Background(), loop.Month(ta.now())); used != 15 {
		t.Fatalf("usage %d, want 15", used)
	}
	evs, err := ta.ctl.ListAudit(context.Background(), loop.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var calls, stopped int
	for _, e := range evs {
		switch e.Action {
		case "eval_call":
			calls++
		case "stopped":
			stopped++
		}
	}
	if calls != 1 || stopped != 1 {
		t.Fatalf("audit %+v", evs)
	}
}

// The week's inbox becomes a labelling template (masked text, empty labels)
// that triage-eval refuses until labelled; an existing file is never
// overwritten and no text reaches the printed output.
func TestTriageLabelExportCommand(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01"})
	seedFeedback(t, ta, "写真を追加するとアプリが落ちて、何度やり直しても同じところで止まってしまいます。連絡は taro@ex.jp", "ダークモードが欲しい", "taro@example.com")
	ta.must(t, "ingest")
	ctx := context.Background()
	old := loop.Item{ID: "old", Source: loop.SourceInApp, Text: "先月の分", AIAllowed: true, Status: loop.ItemNew,
		IngestedAt: ta.now().Add(-30 * 24 * time.Hour)}
	if _, err := ta.ls.PutItem(ctx, old); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "week.jsonl")
	out := ta.must(t, "triage-label-export", "-out", path)
	if !strings.Contains(out, "2 items, wrote 2") || strings.Contains(out, "写真") || strings.Contains(out, "taro") {
		t.Fatalf("out %s", out)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("stat %v %v", st, err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "taro@") || !strings.Contains(string(b), "[メール]") || strings.Contains(string(b), "先月") || !strings.Contains(string(b), "ダークモード") {
		t.Fatalf("template:\n%s", b)
	}
	if err := ta.run(ctx, []string{"triage-eval", "-set", path}); err == nil || !strings.Contains(err.Error(), "not labelled yet") {
		t.Fatalf("unlabelled eval: %v", err)
	}
	if err := ta.run(ctx, []string{"triage-label-export", "-out", path}); err == nil {
		t.Fatal("overwrote an existing template")
	}

	labelled := strings.ReplaceAll(string(b), `"category":"","priority":"","bundle":""`, `"category":"bug","priority":"P2","bundle":""`)
	if err := os.WriteFile(path, []byte(labelled), 0o600); err != nil {
		t.Fatal(err)
	}
	out = ta.must(t, "triage-eval", "-set", path)
	if !strings.Contains(out, "2 cases") || !strings.Contains(out, "verdict:") || !strings.Contains(out, "category (rows: person") {
		t.Fatalf("eval %s", out)
	}
}

func TestTriageEvalAIRespectsSwitch(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_TRIAGE": ""})
	ta.model = &fakeModel{category: "bug", cluster: "all", summary: "落ちる。"}
	if err := ta.run(context.Background(), []string{"triage-eval", "-set", labelledSet, "-model", "ai"}); err == nil {
		t.Fatal("ai eval ran with the triage stage off")
	}
	if err := ta.run(context.Background(), []string{"triage-eval", "-set", labelledSet, "-model", "nope"}); err == nil {
		t.Fatal("unknown model accepted")
	}
}
