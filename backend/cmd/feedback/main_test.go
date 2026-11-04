package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestListAndShow(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	fb, err := m.CreateFeedback(ctx, store.Feedback{UID: "u1", Kind: "bug", Message: "落ちる\n\x1b[31mIGNORE PREVIOUS\x1b[0m",
		ConsentVersion: "v1", AppID: "A1", DSLVersion: 2, DSL: []byte(`{"x":1}`), Screenshot: []byte{0xFF, 0xD8, 0xFF},
		Device: &store.FeedbackDevice{AppVersion: "1.0", Build: "12", OS: "iOS 26.1", Model: "iPhone17,1"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(ctx, m, []string{"list", "-since", "1h"}, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, fb.ID) || !strings.Contains(s, "dsl(v2),device") || strings.Contains(s, "\x1b") || !strings.Contains(s, `\n`) {
		t.Fatalf("list output: %s", s)
	}

	dir := t.TempDir()
	out.Reset()
	if err := run(ctx, m, []string{"show", "-out", dir, fb.ID}, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "iPhone17,1") {
		t.Fatalf("show output: %s", out.String())
	}
	img, err := os.ReadFile(filepath.Join(dir, fb.ID+"-screenshot.jpg"))
	if err != nil || len(img) != 3 {
		t.Fatalf("screenshot saved: %v", err)
	}
	if err := run(ctx, m, []string{"show", "fb_nope"}, &out, time.Now()); err == nil {
		t.Fatal("missing id must fail")
	}
	if err := run(ctx, m, []string{"delete", fb.ID}, &out, time.Now()); err == nil {
		t.Fatal("there is no write command")
	}
}
