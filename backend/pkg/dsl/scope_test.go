package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withScope rewrites the kakeibo example so its expense entity gets the scope.
func withScope(t *testing.T, scope string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "kakeibo.json"))
	if err != nil {
		t.Skip("kakeibo example not found:", err)
	}
	const anchor = `"id": "expense",`
	if !strings.Contains(string(raw), anchor) {
		t.Fatal("kakeibo example changed: expense entity not found")
	}
	return []byte(strings.Replace(string(raw), anchor, anchor+` "scope": "`+scope+`",`, 1))
}

func TestEntityScopeDefaultsToPrivate(t *testing.T) {
	app := Initial()
	app.Entities = []Entity{{ID: "memo", Name: "メモ", Fields: []Field{{ID: "text", Name: "本文", Type: FieldString}}}}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
	e := &app.Entities[0]
	if e.EffectiveScope() != ScopePrivate || e.IsShared() || app.HasSharedEntity() {
		t.Fatalf("empty scope must be private: %+v", e)
	}
}

func TestEntityScopeParsesAndRoundTrips(t *testing.T) {
	for _, scope := range []string{"private", "shared", "public"} {
		app, err := Parse(withScope(t, scope))
		if err != nil {
			t.Fatalf("scope %s: %v", scope, err)
		}
		e := app.EntityByID("expense")
		if string(e.Scope) != scope {
			t.Fatalf("scope %s: parsed %q", scope, e.Scope)
		}
		if shared := scope != "private"; e.IsShared() != shared || app.HasSharedEntity() != shared {
			t.Fatalf("scope %s: IsShared=%v", scope, e.IsShared())
		}
	}
}

func TestEntityScopeRejectsUnknownValue(t *testing.T) {
	if _, err := Parse(withScope(t, "team")); err == nil {
		t.Fatal("schema must reject scope=team")
	}
	app := Initial()
	app.Entities = []Entity{{ID: "memo", Name: "メモ", Scope: "team", Fields: []Field{{ID: "text", Name: "本文", Type: FieldString}}}}
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("Validate must reject scope=team: %v", err)
	}
}

func TestSharedEntityRejectsReservedFieldIDs(t *testing.T) {
	for _, scope := range []EntityScope{ScopeShared, ScopePublic} {
		for _, reserved := range ReservedRecordFields {
			app := Initial()
			app.Entities = []Entity{{ID: "post", Name: "投稿", Scope: scope, Fields: []Field{
				{ID: "text", Name: "本文", Type: FieldString},
				{ID: reserved, Name: "x", Type: FieldString},
			}}}
			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), `field "`+reserved+`" is reserved`) {
				t.Fatalf("%s entity with field %q must fail: %v", scope, reserved, err)
			}
		}
	}
	// private entities may still use those names (no automatic author).
	app := Initial()
	app.Entities = []Entity{{ID: "book", Name: "本", Fields: []Field{{ID: "author", Name: "著者", Type: FieldString}}}}
	if err := app.Validate(); err != nil {
		t.Fatalf("private entity may define author: %v", err)
	}
}
