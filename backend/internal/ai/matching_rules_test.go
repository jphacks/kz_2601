package ai

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #507: the dating decline reaches every prompt; the matching recipe reaches
// the generation and chat prompts of builds that render matching.
func TestMatchingRulesReachPrompts(t *testing.T) {
	for _, want := range []string{datingRules, matchingRules, datingNoteJA, datingNoteEN} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %.40q", want)
		}
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		for _, want := range []string{datingChatRule, matchingChatRule} {
			if !strings.Contains(p, want) {
				t.Errorf("%s chat prompt lacks %.40q", name, want)
			}
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	if got := plainTerms("match を profile にします", LocaleJA); strings.Contains(got, "match ") {
		t.Fatalf("chat leaks the DSL name: %q", got)
	}
}

// Builds without the matching UI (every build before #507 phase 2) get no
// match, but keep the dating decline.
func TestMatchingRestriction(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	if r := NewClientRestriction(nil, nil, nil); r.NoMatching() {
		t.Fatal("no allowedComponents = no restriction (ai-eval, dev)")
	}
	old := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen})
	if !old.Active() || !old.NoMatching() || old.NoRetention() {
		t.Fatalf("a build without matching: active=%v noMatching=%v", old.Active(), old.NoMatching())
	}
	if !NewRestriction(nil, []string{FeatureSharedData}).NoMatching() {
		t.Fatal("no shared data = no matching (match needs a shared entity)")
	}
	ctx := withRestriction(context.Background(), old)
	for name, p := range map[string]string{"create": systemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if strings.Contains(got, matchingRules) || strings.Contains(got, matchingChatRule) {
			t.Errorf("%s: matching rules left for a build without the UI", name)
		}
		if !strings.Contains(got, datingNoteJA) {
			t.Errorf("%s: the dating decline must stay", name)
		}
	}
	if strings.Contains(string(old.assets().genSch), `"match":`) {
		t.Fatal("restricted schema still offers match")
	}
	if !strings.Contains(string(NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed, FeatureNotificationNear}).assets().genSch), `"match":`) &&
		!strings.Contains(string(generationSchema), `"match":`) {
		t.Fatal("the full schema must offer match")
	}

	raw := []byte(`{"entities":[{"id":"profile","name":"プロフィール","scope":"public","match":"profile","fields":[]},{"id":"memo","name":"メモ","fields":[]}],"screens":[]}`)
	if v := restrictionViolations(raw, nil, old); len(v) != 1 || !strings.Contains(v[0], "match") {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(raw, raw, old); len(v) != 0 {
		t.Fatalf("an existing match is not a new violation: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, old)
	if !changed || strings.Contains(string(out), `"match"`) || len(notes) != 1 {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
}

func TestDatingAsked(t *testing.T) {
	for _, s := range []string{
		"恋人を探せるマッチングアプリ",
		"婚活アプリを作って",
		"出会い系のアプリ",
		"異性と出会えるアプリ",
		"デート相手を探したい",
		"彼女が欲しいので探せるアプリ",
		"Make a dating app",
		"an app to find a girlfriend",
	} {
		if !datingAsked(s) {
			t.Errorf("must decline: %q", s)
		}
	}
	for _, s := range []string{
		"同じ趣味の人を探して、お互いに OK したら連絡できるアプリ",
		"恋人とのデートの記録アプリ",
		"彼女の誕生日を忘れないアプリ",
		"恋愛小説の読書記録",
		"恋愛目的じゃない、ボドゲ仲間を探すアプリ",
		"a study buddy finder for my university",
		"match results of my tennis club",
	} {
		if datingAsked(s) {
			t.Errorf("must not decline: %q", s)
		}
	}
}

// The decline sentence is added unless the model already said it.
func TestDatingOmission(t *testing.T) {
	app := &dsl.App{}
	ids := func(tr, honest string) string {
		return strings.Join(omissionIDs(omissions(tr, app, honest, true)), ",")
	}
	if got := ids("恋人を探せるマッチングアプリ", ""); !strings.Contains(got, "dating") {
		t.Fatalf("dating omission: %q", got)
	}
	if got := ids("恋人を探せるマッチングアプリ", datingNoteJA); strings.Contains(got, "dating") {
		t.Fatalf("already said: %q", got)
	}
	if got := ids("ボドゲ仲間を探すアプリ", ""); strings.Contains(got, "dating") {
		t.Fatalf("peer finding is not dating: %q", got)
	}
}

// #507 phase 2: the recipe names the components, and a build that sends
// matching (the current iOS, not the review build) gets them.
func TestMatchingComponentsInRecipe(t *testing.T) {
	for _, want := range []string{"item.interest: true", "matchList", `show: "incoming"`, `show: "matches"`} {
		if !strings.Contains(matchingRules, want) {
			t.Errorf("matchingRules lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "matchList") {
		t.Fatal("the catalog must list matchList (from the schema)")
	}
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	current := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed, FeatureNotificationNear})
	if current.Active() {
		t.Fatalf("a build with every part and matching is unrestricted: %v", current.Disallowed())
	}

	// Sends matchList but not matching (or no shared data): matchList and
	// item.interest are withheld along with match.
	for name, r := range map[string]Restriction{
		"no matching": NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen}),
		"no shared":   NewClientRestriction(all, []string{FeatureSharedData}, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed, FeatureNotificationNear}),
	} {
		if !slices.Contains(r.Disallowed(), string(dsl.CompMatchList)) {
			t.Errorf("%s: matchList allowed: %v", name, r.Disallowed())
		}
		sch := string(r.assets().genSch)
		if strings.Contains(sch, `"interest":`) || strings.Contains(sch, `"matchList"`) {
			t.Errorf("%s: generation schema still offers interest / matchList", name)
		}
		if strings.Contains(r.assets().catalog, "matchList") {
			t.Errorf("%s: catalog still lists matchList", name)
		}
	}
	if !strings.Contains(string(generationSchema), `"interest":`) {
		t.Fatal("the full generation schema must offer item.interest")
	}

	old := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen})
	raw := []byte(`{"entities":[{"id":"profile","name":"プロフィール","scope":"public","match":"profile","fields":[{"id":"hobby","name":"趣味","type":"string"}]}],` +
		`"screens":[{"id":"home","components":[{"type":"list","entity":"profile","item":{"title":"hobby","interest":true}},` +
		`{"type":"matchList","entity":"profile","show":"matches"}]}]}`)
	out, notes, changed := enforceRestriction(raw, old)
	s := string(out)
	if !changed || strings.Contains(s, "interest") || strings.Contains(s, "matchList") || strings.Contains(s, `"match"`) {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, s)
	}
	if got := plainTerms("matchList で届いた人を出します", LocaleJA); strings.Contains(got, "matchList") {
		t.Fatalf("chat leaks the component name: %q", got)
	}
}
