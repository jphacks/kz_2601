package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// The Memory paths of newStore: STORE=memory forces it, and an empty project
// falls back to it. The Firestore path is not touched (it needs an emulator).
func TestNewStoreMemory_3271(t *testing.T) {
	cases := []struct {
		name   string
		store  string
		target fsenv.Target
	}{
		{"STORE=memory with a project", "memory", fsenv.Target{Project: "demo-kz2601", Database: "(default)"}},
		{"empty project", "", fsenv.Target{}},
		{"empty project with another STORE value", "firestore", fsenv.Target{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STORE", tc.store)
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))

			st, closeFn := newStore(context.Background(), tc.target, logger)
			if _, ok := st.(*store.Memory); !ok {
				t.Fatalf("store = %T, want *store.Memory", st)
			}
			if closeFn == nil {
				t.Fatal("close function is nil")
			}
			closeFn() // must be callable (and harmless)
			closeFn()

			if !strings.Contains(buf.String(), "in-memory store") {
				t.Errorf("no in-memory warning logged: %q", buf.String())
			}
		})
	}
}
