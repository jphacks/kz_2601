package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// pausedAfter spends usd through the breaker newBudget built and reports
// whether free callers are paused (Spent first: it pins today's counter).
func pausedAfter(t *testing.T, usd float64) bool {
	t.Helper()
	b := newBudget(nil, nil)
	ctx := context.Background()
	b.Spent(ctx)
	b.Record(ctx, usd)
	paused, _ := b.Paused(ctx)
	return paused
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestNewBudget_3191(t *testing.T) {
	today := time.Now().In(ratelimit.JST).Format("2006-01-02")
	tests := []struct {
		name    string
		env     map[string]string
		spend   float64
		paused  bool
		wantLog string // "" = nothing logged
	}{
		{name: "unset: off", spend: 1000, paused: false},
		{name: "blank: off", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "  "}, spend: 1000, paused: false},
		{name: "zero: off", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "0"}, spend: 1000, paused: false},
		{name: "under the limit", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "5"}, spend: 4.99, paused: false, wantLog: "5.00 USD/day, store=memory"},
		{name: "at the limit pauses", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "5"}, spend: 5, paused: true, wantLog: "free plan budget on"},
		{name: "decimal limit", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": " 0.5 "}, spend: 0.6, paused: true, wantLog: "0.50 USD/day"},
		{name: "resume day today lifts the pause", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "1", "AI_FREE_BUDGET_RESUME_DAY": today}, spend: 10, paused: false, wantLog: `resumeDay="` + today + `"`},
		{name: "resume day another day keeps it", env: map[string]string{"AI_FREE_DAILY_BUDGET_USD": "1", "AI_FREE_BUDGET_RESUME_DAY": "2000-01-01"}, spend: 10, paused: true, wantLog: `resumeDay="2000-01-01"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t, tc.env)
			buf := captureLog(t)
			if got := pausedAfter(t, tc.spend); got != tc.paused {
				t.Fatalf("paused = %v, want %v", got, tc.paused)
			}
			switch out := buf.String(); {
			case tc.wantLog == "" && out != "":
				t.Fatalf("off must be silent, logged %q", out)
			case !strings.Contains(out, tc.wantLog):
				t.Fatalf("log %q lacks %q", out, tc.wantLog)
			}
		})
	}
}

// With a Firestore client the total goes there, unless RATE_LIMIT_STORE=memory.
// The emulator host makes the client without connecting (nothing is read).
func TestNewBudgetStoreChoice_3191(t *testing.T) {
	clearEnv(t, map[string]string{"AI_FREE_DAILY_BUDGET_USD": "1", "FIRESTORE_EMULATOR_HOST": "localhost:1"})
	client, err := firestore.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Skipf("no Firestore client: %v", err)
	}
	defer func() { _ = client.Close() }()

	buf := captureLog(t)
	newBudget(client, nil)
	if !strings.Contains(buf.String(), "store=firestore") {
		t.Fatalf("log: %s", buf)
	}

	t.Setenv("RATE_LIMIT_STORE", "memory")
	buf.Reset()
	newBudget(client, nil)
	if !strings.Contains(buf.String(), "store=memory") {
		t.Fatalf("log: %s", buf)
	}
}

// A bad value stops the start (log.Fatalf): checked in a child process.
func TestNewBudgetBadValueChild_3191(t *testing.T) {
	if os.Getenv("BUDGET_BAD_CHILD") == "" {
		t.Skip("child process only")
	}
	newBudget(nil, nil)
}

func TestNewBudgetBadValueExits_3191(t *testing.T) {
	for name, env := range map[string][]string{
		"not a number":   {"AI_FREE_DAILY_BUDGET_USD=abc"},
		"negative":       {"AI_FREE_DAILY_BUDGET_USD=-1"},
		"bad resume day": {"AI_FREE_DAILY_BUDGET_USD=1", "AI_FREE_BUDGET_RESUME_DAY=11/13"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNewBudgetBadValueChild_3191$")
			cmd.Env = append(append(os.Environ(), "BUDGET_BAD_CHILD=1", "AI_FREE_DAILY_BUDGET_USD=", "AI_FREE_BUDGET_RESUME_DAY="), env...)
			out, err := cmd.CombinedOutput()
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != 1 {
				t.Fatalf("want exit 1, got %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "ai: AI_FREE_") {
				t.Fatalf("output: %s", out)
			}
		})
	}
}
