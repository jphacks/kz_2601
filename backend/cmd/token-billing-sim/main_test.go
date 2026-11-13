package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runSim(t *testing.T, args ...string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := run(args, &out, &errOut); err != nil {
		t.Fatalf("run %v: %v (%s)", args, err, errOut.String())
	}
	return out.String()
}

func TestFlat(t *testing.T) {
	got := runSim(t, "-logs", "testdata/usage.json", "-weights", "testdata/weights_flat.json")
	want := `## flat: 1 per call

ai.usage lines 11, counted 9, left out by the weights (errors / retries) 2, people 3

| per person | people × periods | p50 | p90 | max |
| --- | ---: | ---: | ---: | ---: |
| day (JST) | 5 | 1 | 3 | 3 |
| month (JST) | 4 | 1 | 3 | 3 |

| operation | calls | tokens | share |
| --- | ---: | ---: | ---: |
| generate | 2 | 2 | 22% |
| text | 2 | 2 | 22% |
| chat | 1 | 1 | 11% |
| chat_first | 1 | 1 | 11% |
| logic | 1 | 1 | 11% |
| patch | 1 | 1 | 11% |
| recipes | 1 | 1 | 11% |

- without a uid (not signed in): 1 calls, 1 tokens, not in the per-person rows
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, h := range []string{"p1", "p2", "p3"} {
		if strings.Contains(got, h) {
			t.Fatalf("output carries a hash %s", h)
		}
	}
}

func TestByKindAndTokens(t *testing.T) {
	got := runSim(t, "-logs", "testdata/usage.json",
		"-weights", "testdata/weights_by_kind.json", "-weights", "testdata/weights_tokens.json", "-top", "2")
	for _, s := range []string{
		"## by kind: a create weighs more than an edit or a chat turn",
		// p1 11-01: generate 10 + logic 0 + patch 5; p2 11-02: chat_first 1 + the participant's generate 10 (payer)
		"| day (JST) | 4 | 2 | 15 | 15 |",
		"| generate | 2 | 20 | 65% |",
		"- no weight (not counted; add it or \"*\"): recipes 1",
		"## tokens: by input / output tokens",
		"| day (JST) | 5 | 0.9 | 23 | 23 |",
		"| generate | 2 | 29 | ",
		"| (5 more) | | | |",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
}

func TestCountErrorsAndRetries(t *testing.T) {
	w := filepath.Join(t.TempDir(), "w.json")
	if err := os.WriteFile(w, []byte(`{"name":"all","count_errors":true,"count_retries":true,"operations":{"*":{"call":1}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runSim(t, "-logs", "testdata/usage.json", "-weights", w)
	if !strings.Contains(got, "counted 11, left out by the weights (errors / retries) 0") ||
		!strings.Contains(got, "| patch | 3 | 3 |") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestJSONLines(t *testing.T) {
	lines := `{"time":"2026-11-01T10:00:00+09:00","event":"ai.usage","operation":"chat","attempt":1,"outcome":"ok","uid_hash":"a"}
not json
{"time":"2026-11-01T10:01:00+09:00","event":"ai.generate","uid_hash":"a"}
{"timestamp":"2026-11-01T01:02:00Z","jsonPayload":{"event":"ai.usage","attempt":1,"uid_hash":"a"}}`
	us, err := parseUsage(strings.NewReader(lines))
	if err != nil {
		t.Fatal(err)
	}
	if len(us) != 2 || us[0].Operation != "chat" || us[1].Operation != "unknown" {
		t.Fatalf("%+v", us)
	}
}

func TestBadInput(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, args := range map[string][]string{
		"no logs":          {"-weights", "testdata/weights_flat.json"},
		"no weights":       {"-logs", "testdata/usage.json"},
		"missing log":      {"-logs", "testdata/nope.json", "-weights", "testdata/weights_flat.json"},
		"broken log":       {"-logs", write("bad.json", "[{"), "-weights", "testdata/weights_flat.json"},
		"negative weight":  {"-logs", "testdata/usage.json", "-weights", write("neg.json", `{"operations":{"*":{"call":-1}}}`)},
		"empty operations": {"-logs", "testdata/usage.json", "-weights", write("empty.json", `{"name":"x"}`)},
		"unknown field":    {"-logs", "testdata/usage.json", "-weights", write("typo.json", `{"operations":{"*":{"cal":1}}}`)},
		"extra arg":        {"-logs", "testdata/usage.json", "-weights", "testdata/weights_flat.json", "x"},
		"top 0":            {"-logs", "testdata/usage.json", "-weights", "testdata/weights_flat.json", "-top", "0"},
	} {
		if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
