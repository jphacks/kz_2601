package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iOS の手で動かす編集モード(#1442)が PATCH /dsl に送る形: 画面 id と元の area を test してから
// area を replace / 部品を remove する。重なりは Validate() が断る。
func TestApplyPatchGridEditShape(t *testing.T) {
	current, err := os.ReadFile(filepath.Join(docsDir, "examples", "kakeibo.json"))
	if err != nil {
		t.Skip(err)
	}
	op := func(o, path, value string) PatchOp {
		p := PatchOp{Op: o, Path: path}
		if value != "" {
			p.Value = json.RawMessage(`"` + value + `"`)
		}
		return p
	}
	screenTest := op("test", "/screens/0/id", "home")

	t.Run("move", func(t *testing.T) {
		_, app, err := ApplyPatch(current, []PatchOp{
			screenTest,
			op("test", "/screens/0/components/1/area", "D11:F12"),
			op("replace", "/screens/0/components/1/area", "A11:C12"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := app.Screens[0].Components[1].Area; got != "A11:C12" {
			t.Fatalf("area = %q", got)
		}
	})

	t.Run("overlap is refused", func(t *testing.T) {
		_, _, err := ApplyPatch(current, []PatchOp{
			screenTest,
			op("test", "/screens/0/components/1/area", "D11:F12"),
			op("replace", "/screens/0/components/1/area", "A10:C11"),
		})
		if err == nil || !strings.Contains(err.Error(), "must not share cells") {
			t.Fatalf("expected overlap error, got %v", err)
		}
	})

	t.Run("remove", func(t *testing.T) {
		_, app, err := ApplyPatch(current, []PatchOp{
			screenTest,
			op("test", "/screens/0/components/1/area", "D11:F12"),
			op("remove", "/screens/0/components/1", ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range app.Screens[0].Components {
			if c.ID == "add" {
				t.Fatal("component add was not removed")
			}
		}
	})

	t.Run("stale area is refused", func(t *testing.T) {
		_, _, err := ApplyPatch(current, []PatchOp{
			screenTest,
			op("test", "/screens/0/components/1/area", "A1:B2"),
			op("replace", "/screens/0/components/1/area", "A11:C12"),
		})
		if err == nil {
			t.Fatal("expected the test op to fail on a stale area")
		}
	})
}
