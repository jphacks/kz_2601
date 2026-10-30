package ai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Compatibility with older iOS builds (#302, docs/dsl/README.md §5).
// iOS sends the component types it can render (allowedComponents) on every
// generate / chat. A type it does not list must never reach it, so a new
// backend can add parts while v1.0 is still installed.

// iosV1Components is the list the first App Store build (v1.0) sends: frozen
// here on purpose. Never edit it when adding a part; a part added later must
// show up in newerThanV1 and be withheld from this client.
var iosV1Components = func() []string {
	raw, err := os.ReadFile("testdata/ios_v1_components.json")
	if err != nil {
		panic(err)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}()

func componentTypes(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := map[string]bool{}
	var walk func([]dsl.Component)
	walk = func(cs []dsl.Component) {
		for _, c := range cs {
			out[string(c.Type)] = true
			walk(c.Children)
		}
	}
	for _, sc := range app.Screens {
		walk(sc.Components)
	}
	return out
}

func without(all map[string]bool, drop ...string) []string {
	gone := map[string]bool{}
	for _, d := range drop {
		gone[d] = true
	}
	var out []string
	for t := range all {
		if !gone[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// The frozen v1.0 list: every part the backend knows but v1.0 does not is
// disallowed for it; every part v1.0 lists stays allowed.
func TestCompat_V1ClientWithholdsNewerParts(t *testing.T) {
	r := NewRestriction(iosV1Components, nil)
	v1 := map[string]bool{}
	for _, c := range iosV1Components {
		v1[c] = true
	}
	var newer []string
	for k := range knownComponentTypes {
		if !v1[k] {
			newer = append(newer, k)
		}
	}
	sort.Strings(newer)
	if strings.Join(r.Disallowed(), ",") != strings.Join(newer, ",") {
		t.Fatalf("disallowed %v, want parts newer than v1.0 %v", r.Disallowed(), newer)
	}
	if len(newer) == 0 && r.Active() {
		t.Fatal("v1.0 lists every current part: no restriction (byte-identical prompt)")
	}
}

// A newer client may list parts this backend does not know yet: they are
// ignored, and a client listing every known part is unrestricted.
func TestCompat_NewerClientListIsHarmless(t *testing.T) {
	all := without(knownComponentTypes)
	r := NewRestriction(append(all, "hologram", "teleporter"), nil)
	if r.Active() {
		t.Fatalf("unknown future types must not restrict: %v", r.Disallowed())
	}
}

// One version back: the client lacks parts that the backend (and the AI)
// uses. Whatever the model returns, the generated app the old client gets
// has none of them and is still a valid DSL it can open.
func TestCompat_OlderClientNeverReceivesNewerComponents(t *testing.T) {
	files, err := filepath.Glob("../../../docs/dsl/examples/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("examples: %v", err)
	}
	checked, failed := 0, 0
	for _, path := range files {
		name := filepath.Base(path)
		if name == "initial.json" || strings.HasPrefix(name, "gallery-") {
			continue
		}
		raw := readDoc(t, path)
		used := componentTypes(t, raw)
		for part := range used {
			if part == string(dsl.CompVStack) || part == string(dsl.CompVoiceInput) {
				continue
			}
			t.Run(name+"/"+part, func(t *testing.T) {
				older := without(knownComponentTypes, part)
				// The model ignores the restriction every time; the last
				// attempt must strip the part rather than send it.
				f := &fakeCaller{replies: []string{string(raw)}}
				svc, _ := newSvc(f)
				svc.MaxRegenerations = -1
				res, err := svc.Generate(context.Background(), Request{AppID: "old1", Transcript: "記録を作って", AllowedComponents: older})
				if err != nil {
					// Stripping a structural part (a list others point at)
					// can leave nothing valid: then the client gets an error
					// it already handles, never the part.
					if ge := asError(err); ge.Code != "generation_failed" {
						t.Fatalf("unexpected error: %v", err)
					}
					failed++
					return
				}
				if componentTypes(t, res.DSL)[part] {
					t.Fatalf("%s sent to a client without it", part)
				}
				if _, err := dsl.Parse(res.DSL); err != nil {
					t.Fatalf("stripped DSL invalid: %v", err)
				}
				if !strings.Contains(f.calls[0].System, "## この依頼の制限") || !strings.Contains(f.calls[0].System, part) {
					t.Fatal("restricted prompt must name the missing part")
				}
			})
			checked++
		}
	}
	if checked < 20 {
		t.Fatalf("only %d cases: examples moved?", checked)
	}
	// Most apps must still open (the part is removed, the rest stays).
	if failed*10 > checked {
		t.Fatalf("%d of %d older-client cases produced no app", failed, checked)
	}
}

// Chat (create / edit proposals) uses the same restriction: the catalog the
// model sees for an older client does not list the missing part.
func TestCompat_OlderClientChatPromptDropsPart(t *testing.T) {
	ctx := withRestriction(context.Background(), NewRestriction(without(knownComponentTypes, "routeGuide"), nil))
	p := withoutNote(restrictPrompt(ctx, chatEditSystemPrompt))
	if strings.Contains(p, "routeGuide") {
		t.Fatal("chat prompt still offers routeGuide to a client without it")
	}
}
