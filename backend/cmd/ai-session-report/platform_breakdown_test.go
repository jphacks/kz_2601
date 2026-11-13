package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPlatformBreakdown(t *testing.T) {
	f, err := os.Open("testdata/platform_logs.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	evs, err := parseLogs(f, map[string]bool{"aa": true})
	if err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	r := platformBreakdowns(evs, since, since.Add(48*time.Hour))
	// restored is not a generation; the android line is outside the window
	want := map[string]platformRow{
		"ios":     {Generations: 2, Failed: 1},
		"windows": {Generations: 2, Failed: 1}, // one from http.request, one on the line itself (case folded)
		"linux":   {Generations: 1, Failed: 1}, // refused counts as failed
		"other":   {Generations: 1},            // a value outside the vocabulary
		"unknown": {Generations: 2, Failed: 1}, // no requestId / no http.request line
	}
	if len(r.Rows) != len(want) {
		t.Fatalf("rows: %v", r.Rows)
	}
	for k, w := range want {
		if got := r.Rows[k]; got == nil || *got != w {
			t.Fatalf("%s: %+v want %+v", k, got, w)
		}
	}
	var b strings.Builder
	renderPlatformBreakdown(&b, r)
	out := b.String()
	for _, s := range []string{"| ios | 2 | 1 | 50% |", "| linux | 1 | 1 | 100% |", "| other | 1 | 0 | 0% |", "| unknown | 2 | 1 | 50% |"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "PlayStation") {
		t.Fatalf("raw value leaked:\n%s", out)
	}
	b.Reset()
	renderPlatformBreakdown(&b, platformBreakdowns(nil, since, since))
	if !strings.Contains(b.String(), "ai.generate の行が無い") {
		t.Fatalf("empty: %s", b.String())
	}
}

func TestPlatformBreakdownOffByDefault(t *testing.T) {
	until := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	fx := newFixture(until, "u-alice", "u-other")
	logs, err := parseLogs(strings.NewReader(fx.logs()), map[string]bool{"": false, hashOf("u-alice"): true})
	if err != nil {
		t.Fatal(err)
	}
	render := func(cfg config) (string, string) {
		rep, err := buildReport(context.Background(), fakeReader{fx}, []string{"u-alice"}, logs, true, cfg)
		if err != nil {
			t.Fatal(err)
		}
		var md, js bytes.Buffer
		if err := renderMarkdown(&md, rep, until); err != nil {
			t.Fatal(err)
		}
		if err := renderJSON(&js, rep, until); err != nil {
			t.Fatal(err)
		}
		return md.String(), js.String()
	}
	md, js := render(testConfig(until))
	if strings.Contains(md, "OS ごとの生成") || strings.Contains(js, `"platforms"`) {
		t.Fatal("per-OS table shown while off")
	}
	cfg := testConfig(until)
	cfg.ByPlatform = true
	md, js = render(cfg)
	if !strings.Contains(md, "## OS ごとの生成") || !strings.Contains(md, "| unknown |") {
		t.Fatalf("on: %s", md)
	}
	var out struct {
		Platforms *platformBreakdown `json:"platforms"`
	}
	if err := json.Unmarshal([]byte(js), &out); err != nil || out.Platforms == nil || out.Platforms.Rows["unknown"] == nil {
		t.Fatalf("json: %v %s", err, js)
	}
}
