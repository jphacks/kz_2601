package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

// emulatorClient opens a fresh demo project on the emulator (data from other
// tests cannot leak in).
func emulatorClient(t *testing.T) *firestore.Client {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	project := "demo-asr-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	c, err := fsenv.NewClient(t.Context(), fsenv.Target{Project: project, Database: fsenv.DefaultDatabase})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// seed writes the fixture in the layout of internal/store (test only: the
// tool itself never writes).
func seed(t *testing.T, c *firestore.Client, f *fixture) {
	t.Helper()
	ctx := t.Context()
	for _, a := range f.apps {
		last := a.versions[len(a.versions)-1]
		app := c.Collection("users").Doc(a.uid).Collection("apps").Doc(a.id)
		if _, err := app.Set(ctx, map[string]any{"name": a.name, "status": "active", "currentVersion": last.N,
			"createdAt": a.versions[0].CreatedAt, "updatedAt": last.CreatedAt, "dslJson": string(last.DSL)}); err != nil {
			t.Fatal(err)
		}
		for _, v := range a.versions {
			src := map[string]any{"kind": v.Kind, "mode": v.Mode}
			if v.Transcript != "" {
				src["transcript"] = v.Transcript
			}
			if v.Summary != "" {
				src["summary"] = v.Summary
			}
			if v.RestoredFrom > 0 {
				src["restoredFrom"] = v.RestoredFrom
			}
			if _, err := app.Collection("versions").Doc(strconv.Itoa(100000000 + v.N)[1:]).Set(ctx, map[string]any{
				"version": v.N, "parentVersion": v.N - 1, "dslJson": string(v.DSL), "source": src, "createdAt": v.CreatedAt}); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < a.records; i++ {
			if _, err := app.Collection("entities").Doc("expense").Collection("records").Doc("r"+strconv.Itoa(i)).Set(ctx,
				map[string]any{"entity": "expense", "data": map[string]any{"amount": 100 * i}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, fb := range f.feedback {
		if _, err := c.Collection("feedback").Doc("fb"+strconv.Itoa(i)).Set(ctx, map[string]any{"uid": fb.uid, "appId": fb.appID,
			"kind": fb.kind, "message": fb.msg, "untrusted": true, "status": "received", "createdAt": fb.at,
			"screenshotJpeg": []byte{0xff, 0xd8}}); err != nil {
			t.Fatal(err)
		}
	}
}

// snapshot lists every document path with its update time.
func snapshot(t *testing.T, c *firestore.Client) []string {
	t.Helper()
	ctx := t.Context()
	var out []string
	var walk func(cols *firestore.CollectionIterator)
	walk = func(cols *firestore.CollectionIterator) {
		for {
			col, err := cols.Next()
			if errors.Is(err, iterator.Done) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			refs := col.DocumentRefs(ctx)
			for {
				ref, err := refs.Next()
				if errors.Is(err, iterator.Done) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				line := ref.Path
				if snap, err := ref.Get(ctx); err == nil {
					line += " " + snap.UpdateTime.Format(time.RFC3339Nano)
				}
				out = append(out, line)
				walk(ref.Collections(ctx))
			}
		}
	}
	walk(c.Collections(ctx))
	sort.Strings(out)
	return out
}

func TestFirestoreReportMatchesFixtureAndWritesNothing(t *testing.T) {
	c := emulatorClient(t)
	until := time.Now().UTC().Truncate(time.Second)
	f := newFixture(until, "u-alice", "u-other")
	seed(t, c, f)
	before := snapshot(t, c)
	if len(before) < 10 {
		t.Fatalf("seed: %d docs", len(before))
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs.jsonl")
	if err := os.WriteFile(logPath, []byte(f.logs()), 0o600); err != nil {
		t.Fatal(err)
	}
	o := options{uids: []string{"u-alice"}, logs: []string{logPath}, outDir: filepath.Join(dir, "out"), format: "md",
		days: 14, until: until, cfg: testConfig(until), nowFunc: func() time.Time { return until }}
	var stdout bytes.Buffer
	if err := run(context.Background(), fsReader{c: c}, o, &stdout); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, c); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("the report changed Firestore:\nbefore %v\nafter  %v", before, after)
	}
	if strings.Contains(stdout.String(), fxTranscript1) {
		t.Errorf("stdout leaks the request text: %s", stdout.String())
	}
	files, _ := filepath.Glob(filepath.Join(dir, "out", "*.md"))
	if len(files) != 1 {
		t.Fatalf("report files: %v", files)
	}
	md, _ := os.ReadFile(files[0])
	if !strings.Contains(string(md), fxTranscript1) || strings.Contains(string(md), fxOtherText) {
		t.Error("report content: want alice's request, never other's")
	}

	// the same data read through fsReader gives the same sessions as the fake
	logs, err := parseLogs(strings.NewReader(f.logs()), map[string]bool{hashOf("u-alice"): true})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := buildReport(t.Context(), fsReader{c: c}, []string{"u-alice"}, logs, true, testConfig(until))
	if err != nil {
		t.Fatal(err)
	}
	checkFixtureReport(t, rep)
}

func TestFirestoreReaderSkipsOldApps(t *testing.T) {
	c := emulatorClient(t)
	until := time.Now().UTC().Truncate(time.Second)
	f := newFixture(until, "u-alice", "u-other")
	seed(t, c, f)
	r := fsReader{c: c}
	apps, err := r.Apps(t.Context(), "u-alice", until.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Errorf("apps not updated in the last hour must be skipped: %d", len(apps))
	}
	vs, err := r.Versions(t.Context(), "u-alice", "app1-u-alice", until.Add(-47*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].N != 5 || vs[0].Transcript != fxTranscript3 {
		t.Errorf("versions since: %+v", vs)
	}
	n, err := r.RecordCount(t.Context(), "u-alice", "app1-u-alice", []string{"expense", "none"}, 2)
	if err != nil || n != 2 {
		t.Errorf("record count capped at 2: %d %v", n, err)
	}
	fb, err := r.Feedback(t.Context(), "u-other", until.Add(-14*24*time.Hour))
	if err != nil || len(fb) != 1 || fb[0].Message != fxOtherText {
		t.Errorf("feedback: %+v %v", fb, err)
	}
}
