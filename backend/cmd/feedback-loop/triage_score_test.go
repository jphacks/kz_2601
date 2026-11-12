package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func TestTriageScoreBreakdown(t *testing.T) {
	ta := newTestApp(t, env{})
	ctx := context.Background()
	// id, source, model's category, human label
	rows := [][4]string{
		{"a1", "in_app", "bug", "bug"},
		{"a2", "in_app", "bug", "bug"},
		{"a3", "in_app", "bug", "feature"},
		{"a4", "in_app", "question", "feature"},
		{"s1", "app_store_review", "bug", "feature"},
		{"s2", "app_store_review", "praise", "praise"},
		{"t1", "testflight", "question", "bug"},
		{"t2", "testflight", "", "bug"}, // not triaged: not counted
	}
	var labels strings.Builder
	for _, r := range rows {
		if _, err := ta.ls.PutItem(ctx, loop.Item{ID: r[0], Source: loop.Source(r[1]), Category: r[2], Status: loop.ItemTriaged}); err != nil {
			t.Fatal(err)
		}
		labels.WriteString(r[0] + "," + r[3] + "\n")
	}
	path := filepath.Join(t.TempDir(), "labels.csv")
	if err := os.WriteFile(path, []byte(labels.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	out := ta.must(t, "triage-score", "-labels", path)
	for _, want := range []string{
		"triage-score: 3/7 agree (43%)",
		"app_store_review       2     1    50%",
		"in_app                 4     2    50%",
		"testflight             1     0     0%",
		"bug                    3     2    67%",
		"feature                3     0     0%",
		"praise                 1     1   100%",
		"feature -> bug               2",
		"bug -> question              1",
		"feature -> question          1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// mix-ups are listed most frequent first
	if strings.Index(out, "feature -> bug") > strings.Index(out, "bug -> question") {
		t.Fatalf("order:\n%s", out)
	}
}
