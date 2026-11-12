package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestQuote(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"plain", "hello", 0, `"hello"`},
		{"newline and escape", "a\n\x1b[31m", 0, `"a\n\x1b[31m"`},
		{"invalid utf-8", "\xff", 0, `"\xff"`},
		{"exactly max", "あいう", 3, `"あいう"`},
		{"over max cuts by rune", "あいうえ", 3, `"あいう…"`},
		{"max 0 = no cut", strings.Repeat("x", 300), 0, `"` + strings.Repeat("x", 300) + `"`},
		{"empty", "", 5, `""`},
	}
	for _, c := range cases {
		if got := quote(c.in, c.max); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestUsage(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	if !strings.Contains(b.String(), "feedback list") || !strings.Contains(b.String(), "feedback show") {
		t.Fatalf("usage: %s", b.String())
	}
}

// fakeReader records the query and returns err.
type fakeReader struct {
	q   store.FeedbackQuery
	fb  *store.Feedback
	err error
}

func (f *fakeReader) ListFeedback(_ context.Context, q store.FeedbackQuery) ([]store.Feedback, error) {
	f.q = q
	return nil, f.err
}

func (f *fakeReader) GetFeedback(context.Context, string) (*store.Feedback, error) {
	return f.fb, f.err
}

func TestRunArgs(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		args  []string
		err   error
		ok    bool
		check func(*testing.T, *fakeReader, string)
	}{
		{"list defaults", []string{"list"}, nil, true, func(t *testing.T, f *fakeReader, out string) {
			if f.q.Since != now.Add(-7*24*time.Hour) || f.q.Limit != 50 || f.q.Status != "" || !strings.Contains(out, "0 item(s)") {
				t.Errorf("query %+v out %q", f.q, out)
			}
		}},
		{"list since 0 = all", []string{"list", "-since", "0", "-status", "done", "-limit", "5"}, nil, true, func(t *testing.T, f *fakeReader, _ string) {
			if !f.q.Since.IsZero() || f.q.Status != "done" || f.q.Limit != 5 {
				t.Errorf("query %+v", f.q)
			}
		}},
		{"list bad flag", []string{"list", "-since", "soon"}, nil, false, nil},
		{"list unknown flag", []string{"list", "-x"}, nil, false, nil},
		{"list store error", []string{"list"}, errors.New("down"), false, nil},
		{"show no id", []string{"show"}, nil, false, nil},
		{"show two ids", []string{"show", "a", "b"}, nil, false, nil},
		{"show bad flag", []string{"show", "-nope", "a"}, nil, false, nil},
		{"show store error", []string{"show", "a"}, errors.New("down"), false, nil},
		{"unknown command prints usage", []string{"purge"}, nil, false, func(t *testing.T, _ *fakeReader, out string) {
			if !strings.Contains(out, "usage:") {
				t.Errorf("out %q", out)
			}
		}},
	}
	for _, c := range cases {
		f := &fakeReader{err: c.err}
		var out bytes.Buffer
		err := run(context.Background(), f, c.args, &out, now)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
			continue
		}
		if c.check != nil {
			c.check(t, f, out.String())
		}
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

// a failed write of the output is an error (not a silent empty list)
func TestRunWriteError(t *testing.T) {
	if err := run(context.Background(), &fakeReader{}, []string{"list"}, failWriter{}, time.Now()); err == nil {
		t.Fatal("write error was dropped")
	}
	// the command's own error wins over the write error
	err := run(context.Background(), &fakeReader{err: errors.New("down")}, []string{"list"}, failWriter{}, time.Now())
	if err == nil || err.Error() != "down" {
		t.Fatalf("err %v", err)
	}
}

func TestRunShow(t *testing.T) {
	fb := &store.Feedback{
		ID: "fb1", UID: "u1", Kind: "bug", Status: "received", ConsentVersion: "v1",
		Message: "動かない\n\x1b[31m", AppID: "app1", DSLVersion: 3, DSL: []byte(`{"screens":[]}`),
		Screenshot: []byte{0xff, 0xd8}, Device: &store.FeedbackDevice{AppVersion: "1.0", Build: "7", OS: "iOS 18", Model: "iPhone"},
		CreatedAt: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
	}
	var out bytes.Buffer
	if err := run(context.Background(), &fakeReader{fb: fb}, []string{"show", "fb1"}, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id:        fb1", "createdAt: 2026-11-12T09:00:00+09:00", "device:    app \"1.0\" (\"7\")",
		"dsl:       app app1 v3 (14 bytes)", "screenshot: 2 bytes", `"動かない\n\x1b[31m"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}
