package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShape(t *testing.T) {
	gen := `[{"id":"a","kind":"create","group":"g","transcript":"t"}]`
	chat := `[{"id":"a","group":"g","steps":[{"say":"x"}]}]`
	app := `{"schemaVersion":1,"app":{"name":"x"}}`
	tests := []struct {
		name, file, body string
		want             fileShape
		wantErr          string
	}{
		{"generate", "feed_cases.json", gen, shapeGenerate, ""},
		{"chat", "feed_chat_cases.json", chat, shapeChat, ""},
		{"chat default file", "chat_cases.json", chat, shapeChat, ""},
		{"dsl base", "base_warikan_v1.json", app, shapeDSL, ""},
		{"empty generate", "jphacks_coverage_pending_cases.json", " [] ", shapeGenerate, ""},
		{"empty chat", "x_chat_cases.json", "[]", shapeChat, ""},
		{"empty not cases", "x.json", "[]", "", "empty array"},
		{"chat misnamed", "feed_cases.json", chat, "", "not named *chat_cases.json"},
		{"generate misnamed chat", "feed_chat_cases.json", gen, "", "without chat"},
		{"generate not cases", "feed.json", gen, "", "without chat"},
		{"dsl named cases", "x_cases.json", app, "", "DSL app"},
		{"unknown object", "x.json", `{"foo":1}`, "", "unknown shape"},
		{"unknown items", "x_cases.json", `[{"foo":1}]`, "", "unknown shape"},
		{"both keys", "x_cases.json", `[{"steps":[],"kind":"edit"}]`, "", "unknown shape"},
		{"mixed", "x_cases.json", `[{"kind":"create"},{"steps":[]}]`, "", "mixed shapes"},
		{"scalar", "x.json", `"hi"`, "", "unknown shape"},
		{"empty file", "x.json", "  ", "", "empty file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectShape(tt.file, []byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// checkAll fails on an unknown file and on an edit base that doesn't exist.
func TestCheckAllFailsOnUnknownAndMissingBase(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "ai", "eval")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok_cases.json", `[{"id":"a","kind":"create","group":"g","transcript":"t"}]`)
	counts, err := checkAll(dir, root)
	if err != nil || counts[shapeGenerate] != 1 {
		t.Fatalf("clean dir: %v %v", counts, err)
	}

	write("sub/notes.json", `{"foo":1}`)
	write("edit_cases.json", `[{"id":"e","kind":"edit","group":"g","base":"example:nope","transcript":"t"}]`)
	_, err = checkAll(dir, root)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"notes.json: unknown shape", "base example:nope"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}
