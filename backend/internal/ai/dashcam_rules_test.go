package ai

import (
	"context"
	"strings"
	"testing"
)

// #410: the dashcam rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe dashcam.
func TestDashcamRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ドラレコ・運転中の録画(#410)", "autoStart [landscape, still]", "videoField・list で代替しない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- dashcam:") {
		t.Error("catalog lacks the dashcam line")
	}
	if !strings.Contains(chatCapabilityIndex, "dashcam") {
		t.Error("chat capability index lacks dashcam (chat would answer 「できません」)")
	}
}

// A build without dashcam (App Store review, #245) must not be steered to it:
// every rule line naming the part is dropped by the 「…」は <type> filter.
func TestDashcamRulesDroppedWhenRestricted(t *testing.T) {
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != "dashcam" {
			allowed = append(allowed, typ)
		}
	}
	ctx := withRestriction(context.Background(), NewRestriction(allowed, nil))
	for name, p := range map[string]string{"create": systemPrompt, "chatCreate": chatCreateSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if !strings.Contains(got, "使えない部品: dashcam") {
			t.Fatalf("%s: restriction note missing", name)
		}
		body := withoutNote(got)
		if i := strings.Index(body, "dashcam"); i >= 0 {
			t.Fatalf("%s: dashcam still recommended: …%s…", name, body[max(0, i-80):min(len(body), i+40)])
		}
	}
}
