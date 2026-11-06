package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testConfig(until time.Time) config {
	return config{Since: until.Add(-14 * 24 * time.Hour), Until: until, Gap: 30 * time.Minute,
		QuickUndo: 10 * time.Minute, QuickFix: 5 * time.Minute, Slow: 60 * time.Second, ManyClarify: 3, Top: 20, RecordLimit: 1000}
}

func signalCounts(s *session) map[string]int {
	m := map[string]int{}
	for _, h := range s.Signals {
		m[h.Code] = h.Count
	}
	return m
}

// checkFixtureReport is shared with the emulator test.
func checkFixtureReport(t *testing.T, rep *report) {
	t.Helper()
	if len(rep.Users) != 1 {
		t.Fatalf("users: %d", len(rep.Users))
	}
	if len(rep.Sessions) != 3 {
		for _, s := range rep.Sessions {
			t.Logf("session %+v", *s)
		}
		t.Fatalf("sessions: want 3, got %d", len(rep.Sessions))
	}
	s1, gone, s2 := rep.Sessions[0], rep.Sessions[1], rep.Sessions[2]

	// session 1: the one that went wrong
	var kinds []string
	for _, st := range s1.Steps {
		kinds = append(kinds, st.Kind)
	}
	if got := strings.Join(kinds, ","); got != "seed,ai_create,ai_edit,restore,gen_unchanged" {
		t.Errorf("session 1 steps: %s", got)
	}
	if s1.Steps[1].Clarify != 2 {
		t.Errorf("clarify turns before the create: want 2 (retry not counted), got %d", s1.Steps[1].Clarify)
	}
	want := map[string]int{"quick_undo": 1, "unchanged": 1, "abandoned": 1, "ai_errors": 1, "feedback": 1, "quick_fix": 1, "regenerated": 1, "slow": 1}
	got := signalCounts(s1)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("session 1 %s: want %d got %d", k, v, got[k])
		}
	}
	if len(got) != len(want) {
		t.Errorf("session 1 signals: %v", got)
	}
	if s1.Score != 17 {
		t.Errorf("session 1 score: want 17 got %d", s1.Score)
	}

	// the deleted app: log only
	if !gone.Deleted || gone.Score != 3 || len(gone.Steps) != 1 || gone.Steps[0].Kind != stepNoVersion {
		t.Errorf("deleted app session: %+v", *gone)
	}
	// session 2: a single edit 3 hours later, app still used
	if s2.Score != 0 || !s2.Last || s2.Records != 3 || len(s2.Steps) != 1 || s2.Steps[0].Kind != stepEdit {
		t.Errorf("session 2: %+v", *s2)
	}
	if s1.Last {
		t.Error("session 1 is not the app's last")
	}
	u := rep.Users[0]
	if len(u.ChatNoGen) != 1 || u.ChatNoGen[0].Turns != 2 {
		t.Errorf("chat without generation: %+v", u.ChatNoGen)
	}
	if len(u.Errors) != 0 || len(u.Feedback) != 0 {
		t.Errorf("errors / feedback should be in sessions: %+v %+v", u.Errors, u.Feedback)
	}
}

func TestBuildReport(t *testing.T) {
	until := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	f := newFixture(until, "u-alice", "u-other")
	logs, err := parseLogs(strings.NewReader(f.logs()), map[string]bool{"": false, hashOf("u-alice"): true})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := buildReport(context.Background(), fakeReader{f}, []string{"u-alice"}, logs, true, testConfig(until))
	if err != nil {
		t.Fatal(err)
	}
	checkFixtureReport(t, rep)
	tot := rep.totals()
	if tot.Sessions != 3 || tot.Flagged != 2 || tot.Requests != 5 || tot.ClarifyTurns != 2 || tot.ChatNoGenRun != 1 {
		t.Errorf("totals: %+v", tot)
	}
}

func TestSessionGapSplits(t *testing.T) {
	until := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	f := newFixture(until, "u-alice", "u-other")
	cfg := testConfig(until)
	cfg.Gap = 4 * time.Hour // the 3-hour pause no longer splits
	rep, err := buildReport(context.Background(), fakeReader{f}, []string{"u-alice"}, nil, false, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sessions) != 1 || len(rep.Sessions[0].Steps) != 5 {
		t.Fatalf("want one session of 5 versions, got %d", len(rep.Sessions))
	}
	// without logs: the quick undo is still seen from Firestore
	if c := signalCounts(rep.Sessions[0]); c["quick_undo"] != 1 || c["quick_fix"] != 1 {
		t.Errorf("signals: %v", c)
	}
}

func TestRenderKeepsContentInFileOnly(t *testing.T) {
	until := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	f := newFixture(until, "u-alice", "u-other")
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs.json")
	if err := os.WriteFile(logPath, []byte(f.logs()), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "reports")
	o := options{uids: []string{"u-alice"}, logs: []string{logPath}, outDir: out, format: "both", days: 14, until: until,
		cfg: testConfig(until), nowFunc: func() time.Time { return until }}
	var stdout bytes.Buffer
	if err := run(context.Background(), fakeReader{f}, o, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{fxTranscript1, fxTranscript2, fxFeedback, "家計簿", "u-alice"} {
		if strings.Contains(stdout.String(), secret) {
			t.Errorf("stdout leaks %q:\n%s", secret, stdout.String())
		}
	}
	md, err := os.ReadFile(filepath.Join(out, "ai-session-report-20261106-210000.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{fxTranscript1, fxTranscript2, fxFeedback, "画面 1: 「ホーム」: list・button「追加」", "すぐ元に戻した"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("report misses %q", want)
		}
	}
	if strings.Contains(string(md), fxOtherText) {
		t.Error("report contains another user's data")
	}
	js, err := os.ReadFile(filepath.Join(out, "ai-session-report-20261106-210000.json"))
	if err != nil || !strings.Contains(string(js), `"quick_undo"`) {
		t.Fatalf("json: %v", err)
	}
	st, err := os.Stat(filepath.Join(out, "ai-session-report-20261106-210000.md"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("report mode: %v %v", st.Mode(), err)
	}
	// never overwrite an earlier report
	if err := run(context.Background(), fakeReader{f}, o, &stdout); err == nil {
		t.Error("second run with the same timestamp must not overwrite")
	}
}

func TestUserTextFlattens(t *testing.T) {
	got := userText("a\nb | <script>`x`")
	if strings.ContainsAny(got, "\n|<>`") {
		t.Errorf("userText: %q", got)
	}
}
