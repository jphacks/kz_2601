package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #451 G2: the review build sends disabledFeatures sharedData; every record
// stays private and chatThread is not offered.

// withPublicEntity marks kakeibo's first entity public with a like mark and
// edit:members (what a share request would produce).
func withPublicEntity(t *testing.T) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(kakeibo(t), &doc); err != nil {
		t.Fatal(err)
	}
	ents, _ := doc["entities"].([]any)
	e, _ := ents[0].(map[string]any)
	e["scope"] = "public"
	e["edit"] = "members"
	e["marks"] = []any{map[string]any{"id": "like", "icon": "heart", "label": "いいね"}}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRestriction_NoSharedDataKeepsRecordsPrivate(t *testing.T) {
	ctx := reviewCtx()
	for name, p := range map[string]string{
		"create": systemPrompt, "edit": systemPrompt + "\n" + editRules,
		"chat": chatSystemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt,
	} {
		body := withoutNote(restrictPrompt(ctx, p))
		for _, bad := range []string{`scope:"public"`, `scope:"shared"`, "## 共有の範囲", "## いいね・保存・既読は entity.marks", "## チャット・メッセージのアプリ", "- 投稿者と「自分の」", "- entity.edit(#252)"} {
			if strings.Contains(body, bad) {
				t.Fatalf("%s: shared-data rule %q still in the restricted prompt", name, bad)
			}
		}
		if !strings.Contains(restrictPrompt(ctx, p), "記録はすべて自分だけ(private)") {
			t.Fatalf("%s: shared-data note missing", name)
		}
	}
	// Drift: every rewritten prefix still exists in some unrestricted prompt.
	all := systemPrompt + "\n" + chatSystemPrompt + "\n" + chatCreateSystemPrompt + "\n" + chatEditSystemPrompt
	for _, rw := range sharingLineRewrites {
		if !strings.Contains(all, "\n"+rw.prefix) {
			t.Fatalf("rule line %q no longer in the prompts: update sharingLineRewrites", rw.prefix)
		}
	}
	if !strings.Contains(systemPrompt, "\n\n"+marksRules+"\n"+marksCatalog) || !strings.Contains(systemPrompt, "\n\n"+chatThreadRules) ||
		!strings.Contains(systemPrompt, shareRules) || !strings.Contains(chatEditSystemPrompt, shareChatRule) {
		t.Fatal("marks / chatThread / share rules are no longer in the prompts as dropSharingRules expects")
	}
	if !strings.Contains(restrictPrompt(ctx, chatEditSystemPrompt), "記録を他の人と共有できません") {
		t.Fatal("chat capabilities still offer shared data")
	}

	a, d := reviewBuild()
	as := NewRestriction(a, d).assets()
	if strings.Contains(as.catalog, "scope: public") || strings.Contains(as.catalog, "scope: shared") || !strings.Contains(as.catalog, "scope: private") {
		t.Fatal("restricted catalog still offers shared scopes")
	}
	if !strings.Contains(catalog, "scope: public") {
		t.Fatal("full catalog lost scope public")
	}
	var root map[string]any
	if err := json.Unmarshal(restrictSchemaJSON(dsl.SchemaJSON(), NewRestriction(a, d)), &root); err != nil {
		t.Fatal(err)
	}
	props := root["$defs"].(map[string]any)["Entity"].(map[string]any)["properties"].(map[string]any)
	if en := props["scope"].(map[string]any)["enum"].([]any); len(en) != 1 || en[0] != "private" {
		t.Fatalf("scope enum: %v", en)
	}
	if _, ok := props["marks"]; ok {
		t.Fatal("marks still in the restricted schema")
	}
}

func TestRestriction_NoSharedDataEnforcesPrivate(t *testing.T) {
	r := NewRestriction(nil, []string{FeatureSharedData}) // kakeibo's camera source is not under test here
	if !r.Active() || !r.NoSharedData() {
		t.Fatal("sharedData alone is a restriction")
	}
	raw := withPublicEntity(t)
	if v := restrictionViolations(raw, nil, r); len(v) != 1 || !strings.Contains(v[0], "shared / public") {
		t.Fatalf("create with a public entity: %v", v)
	}
	if v := restrictionViolations(raw, raw, r); len(v) != 0 {
		t.Fatalf("an entity already shared is not a new violation: %v", v)
	}
	if v := restrictionViolations(raw, nil, NewRestriction(nil, nil)); len(v) != 0 {
		t.Fatalf("normal builds keep shared data: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, r)
	if !changed || len(notes) != 1 || notes[0] != "made 1 entities private" {
		t.Fatalf("enforce: changed=%v notes=%v", changed, notes)
	}
	for _, bad := range []string{`"scope"`, `"edit"`, `"marks"`} {
		if strings.Contains(string(out), bad) {
			t.Fatalf("%s left after enforce", bad)
		}
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatal(err)
	}
	if _, _, changed := enforceRestriction(kakeibo(t), r); changed {
		t.Fatal("a private app must pass unchanged")
	}
}

// The review build has no purchase UI (#450 G1): chat does not mark a share
// request as paid. Normal builds still do (TestChat_ReadyRequiresProForSharedData).
func TestRestriction_NoSharedDataSkipsPaidFeatures(t *testing.T) {
	a, d := reviewBuild()
	svc, _ := newSvc(&fakeCaller{replies: []string{createReadyShared}})
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "n1", Mode: "create", Messages: user("みんなで使う掲示板"), AllowedComponents: a, DisabledFeatures: d})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.RequiresPro || len(r.PaidFeatures) != 0 || r.FreeTranscript != "" {
		t.Fatalf("review build got paid features: %+v", r)
	}
}
