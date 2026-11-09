package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// onDemandEvalCases is the recipe each JPHACKS product eval case relies on
// ("" = none: the core rules cover it). Every case of the files must be here.
var onDemandEvalCases = map[string]string{
	"jp_slot_booking":    "slot_booking",
	"jp_priority_matrix": "priority_matrix",
	"jp_menu_complete":   "complete_rate",
	"jp_habit_hp":        "char_hp",
	"jp_stamp_rally":     "",
	"jp_notebook_lists":  "",

	"jp2_monster_todo":      "monster_hp",
	"jp2_class_assignments": "own_done",
	"jp2_task_order":        "prev_task",
	"jp2_same_feeling":      "same_feeling",
	"jp2_keep_in_touch":     "keep_in_touch",
	"jp2_coffee_quiz":       "diagnosis",

	"jp3_plant_streak":   "streak_grow",
	"jp3_lunch_vote":     "one_vote",
	"jp3_pomodoro_share": "timer_record",
	"jp3_cooking_level":  "level_up",
	"jp3_pdf_chars":      "char_count",
	"jp3_station_check":  "info_check",

	"jp4_level_rewrite":  "level_rewrite",
	"jp4_meeting_review": "meeting_review",
	"jp4_recycle_co2":    "kind_coef",
	"jp4_today_items":    "today_weekday",
	"jp4_need_kcal":      "need_kcal",
	"jp4_energy_drink":   "did_vs_didnt",

	"jp5_manual_photo_check": "manual_photo",
	"jp5_evac_badges":        "evac_badges",
	"jp5_kids_sns_practice":  "sns_practice",
	"jp5_task_urgency":       "urgency_color",
	"jp5_store_sales":        "store_mom",
	"jp5_message_emotion":    "emotion_ratio",

	// jphacks_coverage_cases.json: the one whose app is a recipe's
	"cov_shelter_guide": "evac_badges",
}

type evalCase struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Transcript string `json:"transcript"`
}

func readEvalCases(t *testing.T, pattern string) map[string][]evalCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("eval", pattern))
	if err != nil || len(files) == 0 {
		t.Fatalf("no eval files %s: %v", pattern, err)
	}
	out := map[string][]evalCase{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var cs []evalCase
		if json.Unmarshal(raw, &cs) != nil {
			continue // not a case list (base DSL files)
		}
		out[filepath.Base(f)] = cs
	}
	return out
}

func recipeIDs() map[string]bool {
	ids := map[string]bool{}
	for _, b := range onDemandBlocks {
		for _, r := range b.recipes {
			ids[r.id] = true
		}
	}
	return ids
}

func selects(transcript string, app []byte, id string) bool {
	_, ids := onDemandTail(transcript, app)
	return slices.Contains(ids, id)
}

// Every JPHACKS product recipe is on demand, ids are unique, and every block
// is in systemPrompt verbatim (no later init rewrote it), so the core drops it.
func TestOnDemandBlocks_FoundVerbatim(t *testing.T) {
	n := 0
	for _, b := range onDemandBlocks {
		n += len(b.recipes)
		if strings.Count(systemPrompt, "\n\n"+b.text) != 1 {
			t.Errorf("block %q…: not found exactly once in systemPrompt", b.text[:40])
		}
	}
	if len(recipeIDs()) != n || n != 28 {
		t.Fatalf("%d recipes, %d unique ids (want 28 unique)", n, len(recipeIDs()))
	}
	for _, s := range []string{jphacksProductsRules, jphacksProducts2Rules, jphacksProducts3Rules, jphacksProducts4Rules, jphacksProducts5Rules, jphacksProducts5PhotoRule} {
		if strings.Contains(onDemandCore(systemPrompt), s) {
			t.Errorf("the core still holds %q…", s[:40])
		}
	}
	want := len(systemPrompt)
	for _, b := range onDemandBlocks {
		want -= len("\n\n" + b.text)
	}
	if got := len(onDemandCore(systemPrompt)); got != want {
		t.Fatalf("core %d bytes, want %d", got, want)
	}
}

// Each recipe is selected by the phrases its own bullet quotes (「…」).
func TestOnDemandRecall_OwnPhrases(t *testing.T) {
	quoted := regexp.MustCompile(`「([^」]+)」`)
	for _, b := range onDemandBlocks {
		for _, r := range b.recipes {
			// the first quote is the request; later ones are prompt texts
			m := quoted.FindStringSubmatch(r.text)
			if m == nil {
				t.Fatalf("%s: no quoted phrase", r.id)
			}
			if !selects(m[1], nil, r.id) {
				t.Errorf("%s: misses its own phrase %q", r.id, m[1])
			}
		}
	}
}

// Every eval case of the JPHACKS product files selects its recipe.
func TestOnDemandRecall_EvalCases(t *testing.T) {
	ids := recipeIDs()
	seen := 0
	for f, cs := range readEvalCases(t, "jphacks_*cases.json") {
		for _, c := range cs {
			want, ok := onDemandEvalCases[c.ID]
			if !ok {
				if strings.HasPrefix(f, "jphacks_products") {
					t.Errorf("%s %s: not in onDemandEvalCases", f, c.ID)
				}
				continue
			}
			seen++
			if want == "" {
				continue
			}
			if !ids[want] {
				t.Fatalf("%s: unknown recipe %s", c.ID, want)
			}
			if !selects(c.Transcript, nil, want) {
				t.Errorf("%s: %s not selected for %q", c.ID, want, c.Transcript)
			}
		}
	}
	if seen != len(onDemandEvalCases) {
		t.Fatalf("%d of %d mapped cases found", seen, len(onDemandEvalCases))
	}
}

type phrasings struct {
	Train   map[string][]string `json:"train"`
	Tuned   map[string][]string `json:"tuned"`
	Holdout map[string][]string `json:"holdout"`
}

func readPhrasings(t *testing.T) phrasings {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "on_demand_phrasings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p phrasings
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	ids := recipeIDs()
	for _, set := range []map[string][]string{p.Train, p.Tuned, p.Holdout} {
		if len(set) != len(ids) {
			t.Fatalf("phrasings for %d recipes, want %d", len(set), len(ids))
		}
		for id := range set {
			if !ids[id] {
				t.Fatalf("phrasings for unknown recipe %s", id)
			}
		}
	}
	return p
}

// The paraphrases the triggers were written from all select their recipe.
func TestOnDemandRecall_Train(t *testing.T) {
	p := readPhrasings(t)
	for _, set := range []map[string][]string{p.Train, p.Tuned} {
		for id, ss := range set {
			for _, s := range ss {
				if !selects(s, nil, id) {
					t.Errorf("%s: misses %q", id, s)
				}
			}
		}
	}
}

// onDemandHoldoutFloor guards the keyword-only held-out recall against
// regressions (the selector call covers the rest: recipe_selector.go).
const onDemandHoldoutFloor = 0.65

// Paraphrases not used to write the triggers: recall is reported (go test
// -v -run Holdout) and must stay at or above onDemandHoldoutFloor.
func TestOnDemandRecall_Holdout(t *testing.T) {
	hit, total := 0, 0
	var misses []string
	for id, ss := range readPhrasings(t).Holdout {
		for _, s := range ss {
			total++
			if selects(s, nil, id) {
				hit++
			} else {
				misses = append(misses, id+": "+s)
			}
		}
	}
	sort.Strings(misses)
	recall := float64(hit) / float64(total)
	t.Logf("holdout recall %d/%d = %.1f%%", hit, total, 100*recall)
	for _, m := range misses {
		t.Logf("  miss %s", m)
	}
	if recall < onDemandHoldoutFloor {
		t.Fatalf("holdout recall %.1f%% under %.0f%%", 100*recall, 100*onDemandHoldoutFloor)
	}
}

// An edit brings a recipe when the existing app already uses its parts or
// names, whatever the utterance says.
func TestOnDemand_EditAppSelectsItsRecipes(t *testing.T) {
	for _, c := range []struct{ id, app string }{
		{"priority_matrix", `{"screens":[{"components":[{"type":"gridTable","entity":"task","columnField":"urgency","rowField":"importance"}]}]}`},
		{"timer_record", `{"screens":[{"components":[{"type":"timerDisplay","mode":"countdown","seconds":1500}]}]}`},
		{"evac_badges", `{"screens":[{"components":[{"type":"routeGuide","entity":"shelter"}]}]}`},
		{"streak_grow", `{"entities":[{"id":"habit","fields":[{"id":"streak","type":"integer","computed":{"op":"streak","args":[{"ref":"done_on"}]}}]}]}`},
		{"slot_booking", `{"entities":[{"id":"slot","name":"面談の枠"}]}`},
		{"store_mom", `{"app":{"name":"支店の売上"}}`},
	} {
		if !selects("ボタンの色を青にして", []byte(c.app), c.id) {
			t.Errorf("%s: not selected for an app with %s", c.id, c.app)
		}
	}
	if _, ids := onDemandTail("ボタンの色を青にして", []byte(`{"app":{"name":"家計簿"}}`)); len(ids) != 0 {
		t.Errorf("a plain kakeibo edit selected %v", ids)
	}
}

// selectorFake answers the recipe selector call (Model = the recipe model)
// with reply (or blocks until its ctx ends when block is set, or fails with
// err) and passes every other call to gen.
type selectorFake struct {
	mu       sync.Mutex
	gen      *fakeCaller
	reply    string
	block    bool
	err      error
	selector []Completion
	ops      []string
}

func (f *selectorFake) Complete(ctx context.Context, req Completion) (*Response, error) {
	if req.Model == DefaultRecipeModel {
		f.mu.Lock()
		f.selector = append(f.selector, req)
		f.ops = append(f.ops, operationOf(ctx))
		f.mu.Unlock()
		if f.block {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if f.err != nil {
			return nil, f.err
		}
		return &Response{Text: f.reply, StopReason: "end_turn", Usage: Usage{InputTokens: 900, OutputTokens: 12}}, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gen.Complete(ctx, req)
}

// onDemandRun is one create and one edit through Generate.
type onDemandRun struct {
	create, edit         Completion
	createMeta, editMeta Meta
	sel                  *selectorFake
}

// onDemandCalls runs one create and one edit; sel (nil = it picks nothing)
// answers their selector calls.
func onDemandCalls(t *testing.T, ctx context.Context, on bool, create, edit string, sel *selectorFake) onDemandRun {
	t.Helper()
	if sel == nil {
		sel = &selectorFake{reply: `{"ids":[]}`}
	}
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(kb)}}
	sel.gen = f
	svc := &Service{Claude: sel, Store: appconfigclient.NewMemory(), Model: "m", OnDemandRules: on, RecipeTimeout: 200 * time.Millisecond}
	cr, err := svc.Generate(ctx, Request{AppID: "a1", Transcript: create})
	if err != nil {
		t.Fatal(err)
	}
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	g := &fakeCaller{replies: []string{`{"patch":[{"op":"replace","path":"/app/name","value":"家計簿2"}],"summary":"x"}`}}
	sel.gen = g
	svc = &Service{Claude: sel, Store: store, Model: "m", OnDemandRules: on, RecipeTimeout: 200 * time.Millisecond}
	er, err := svc.Generate(ctx, Request{AppID: "a1", Transcript: edit})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) == 0 || len(g.calls) == 0 {
		t.Fatalf("calls: create %d edit %d", len(f.calls), len(g.calls))
	}
	return onDemandRun{create: f.calls[0], edit: g.calls[0], createMeta: cr.Meta, editMeta: er.Meta, sel: sel}
}

// The cached blocks are the same for every request; only the tail differs.
func TestOnDemand_CoreCachedRecipesInTail(t *testing.T) {
	h := onDemandCalls(t, context.Background(), true, "大事か急ぎかの4象限で見るToDo", "カテゴリにカフェを足して。あと何%制覇したかも", nil)
	m := onDemandCalls(t, context.Background(), true, "家計簿を作って", "カテゴリにカフェを足して", nil)
	hit, hitEdit, miss, missEdit := h.create, h.edit, m.create, m.edit
	// the default Service (AI_TEXT_IMAGE off) restricts the core: compare with that
	core := restrictPrompt(withRestriction(context.Background(), (&Service{}).clientRestriction(nil, nil, nil)), onDemandCore(systemPrompt))
	if hit.System != core || miss.System != core || hitEdit.System != core || missEdit.System != core {
		t.Fatal("the first block must be the core for every create / edit (cache-stable)")
	}
	if hitEdit.SystemSuffix != editSystemSuffix() || missEdit.SystemSuffix != editSystemSuffix() || hit.SystemSuffix != "" {
		t.Fatal("edit keeps its cached editRules block")
	}
	if !strings.HasPrefix(hit.SystemTail, "\n\n"+onDemandHeading) || !strings.Contains(hit.SystemTail, "gridTable{entity: task") {
		t.Fatalf("create tail: %q", hit.SystemTail)
	}
	if !strings.Contains(hitEdit.SystemTail, "制覇率") {
		t.Fatalf("edit tail: %q", hitEdit.SystemTail)
	}
	if miss.SystemTail != "" || strings.Contains(missEdit.SystemTail, "制覇率") {
		t.Fatalf("unmatched requests: create %q edit %q", miss.SystemTail, missEdit.SystemTail)
	}
}

// Kill switch (AI_ON_DEMAND_RULES=0 = OnDemandRules off): the whole
// systemPrompt in the first block, no tail, no selector call, no recipe meta.
func TestOnDemand_OffSendsWholePrompt(t *testing.T) {
	r := onDemandCalls(t, context.Background(), false, "大事か急ぎかの4象限で見るToDo", "何%制覇したかも見たい", &selectorFake{reply: `{"ids":["slot_booking"]}`})
	full := restrictPrompt(withRestriction(context.Background(), (&Service{}).clientRestriction(nil, nil, nil)), systemPrompt)
	if r.create.System != full || r.edit.System != full || r.create.SystemTail != "" || r.edit.SystemTail != "" {
		t.Fatal("off: the request must be today's, byte for byte")
	}
	if r.edit.SystemSuffix != editSystemSuffix() || r.create.SystemSuffix != "" {
		t.Fatal("off: edit keeps its editRules block")
	}
	if len(r.sel.selector) != 0 || r.createMeta.Recipes != nil || r.createMeta.RecipeSelector != "" {
		t.Fatalf("off: no selector call (%d) and no recipe meta", len(r.sel.selector))
	}
}

// Keyword hits and selector picks are unioned in the tail, with their source.
func TestOnDemand_SelectorUnionedWithKeywords(t *testing.T) {
	sel := &selectorFake{reply: `{"ids":["one_vote","priority_matrix"]}`}
	r := onDemandCalls(t, context.Background(), true, "大事か急ぎかの4象限で見るToDo", "ボタンの色を青にして", sel)
	m := r.createMeta
	if !slices.Equal(m.Recipes, []string{"one_vote", "priority_matrix"}) {
		t.Fatalf("recipes %v", m.Recipes)
	}
	if m.RecipeSources["priority_matrix"] != recipeByBoth || m.RecipeSources["one_vote"] != recipeBySelector || m.RecipeSelector != recipeSelOK {
		t.Fatalf("sources %v selector %q", m.RecipeSources, m.RecipeSelector)
	}
	if !strings.Contains(r.create.SystemTail, "1 人 1 票") || !strings.Contains(r.create.SystemTail, "gridTable{entity: task") {
		t.Fatalf("tail: %q", r.create.SystemTail)
	}
	// the edit: the selector alone brings recipes the keywords miss
	if r.editMeta.RecipeSources["one_vote"] != recipeBySelector || !strings.Contains(r.edit.SystemTail, "1 人 1 票") {
		t.Fatalf("edit meta %v", r.editMeta.RecipeSources)
	}
	if len(sel.selector) != 2 || sel.ops[0] != OpRecipes {
		t.Fatalf("selector calls %d ops %v", len(sel.selector), sel.ops)
	}
	c := sel.selector[0]
	sys, schema := recipeIndexOnce()
	if c.System != sys || !c.Structured || string(c.Schema) != string(schema) || c.Effort != "low" || c.Thinking != "disabled" || c.SystemTail != "" {
		t.Fatalf("selector request: %+v", c)
	}
	// edit: the app's name, entities and parts go with the transcript
	if u := sel.selector[1].Messages[0].Content; !strings.Contains(u, "今のアプリ:") || !strings.Contains(u, "部品:") {
		t.Fatalf("edit selector message: %q", u)
	}
}

// Unknown or repeated ids are dropped; a non-JSON reply counts as invalid.
func TestRecipeSelector_DropsUnknownIDs(t *testing.T) {
	ids, st := parseRecipeIDs(`{"ids":["one_vote","no_such","one_vote","SLOT_BOOKING"]}`)
	if !slices.Equal(ids, []string{"one_vote"}) || st != recipeSelOK {
		t.Fatalf("ids %v status %s", ids, st)
	}
	if ids, st := parseRecipeIDs("たぶん one_vote"); ids != nil || st != recipeSelInvalid {
		t.Fatalf("invalid reply: %v %s", ids, st)
	}
	r := onDemandCalls(t, context.Background(), true, "家計簿を作って", "カテゴリにカフェを足して", &selectorFake{reply: `{"ids":["made_up"]}`})
	if r.createMeta.Recipes != nil || r.create.SystemTail != "" {
		t.Fatalf("unknown ids must not reach the tail: %v", r.createMeta.Recipes)
	}
}

// A selector slower than RecipeTimeout (or failing) leaves the keywords alone;
// the generation still runs.
func TestRecipeSelector_TimeoutAndErrorFallBackToKeywords(t *testing.T) {
	for _, c := range []struct {
		sel  *selectorFake
		want string
	}{
		{&selectorFake{block: true}, recipeSelTimeout},
		{&selectorFake{err: &APIError{Status: 500, Type: "api_error"}}, recipeSelError},
	} {
		start := time.Now()
		r := onDemandCalls(t, context.Background(), true, "大事か急ぎかの4象限で見るToDo", "何%制覇したかも見たい", c.sel)
		if r.createMeta.RecipeSelector != c.want || r.editMeta.RecipeSelector != c.want {
			t.Fatalf("status %q / %q, want %s", r.createMeta.RecipeSelector, r.editMeta.RecipeSelector, c.want)
		}
		if !slices.Equal(r.createMeta.Recipes, []string{"priority_matrix"}) || r.createMeta.RecipeSources["priority_matrix"] != recipeByKeyword {
			t.Fatalf("keywords only: %v", r.createMeta.Recipes)
		}
		if c.want == recipeSelTimeout && (time.Since(start) > 2*time.Second || r.createMeta.RecipeSelectorMs < 150) {
			t.Fatalf("timeout wait %v (selector %d ms)", time.Since(start), r.createMeta.RecipeSelectorMs)
		}
	}
}

// With a selector that answers right, every eval case and every phrasing
// gets its recipe (the real model's recall is for the live quality tests).
func TestOnDemandRecall_WithRightSelectorIs100(t *testing.T) {
	type want struct{ text, id string }
	var all []want
	for _, cs := range readEvalCases(t, "jphacks_*cases.json") {
		for _, c := range cs {
			if id := onDemandEvalCases[c.ID]; id != "" {
				all = append(all, want{c.Transcript, id})
			}
		}
	}
	p := readPhrasings(t)
	for _, set := range []map[string][]string{p.Train, p.Tuned, p.Holdout} {
		for id, ss := range set {
			for _, s := range ss {
				all = append(all, want{s, id})
			}
		}
	}
	for _, w := range all {
		f := &selectorFake{reply: `{"ids":["` + w.id + `"]}`}
		s := &Service{Claude: f, OnDemandRules: true}
		var meta Meta
		_, tail := s.onDemandPrompt(context.Background(), w.text, nil, &meta)
		if !slices.Contains(meta.Recipes, w.id) || !strings.Contains(strings.Join(tail, ""), recipeText(w.id)) {
			t.Errorf("%s: not sent for %q", w.id, w.text)
		}
	}
	t.Logf("with a right selector: %d/%d phrasings get their recipe", len(all), len(all))
}

func recipeText(id string) string {
	for _, b := range onDemandBlocks {
		for _, r := range b.recipes {
			if r.id == id {
				return r.text
			}
		}
	}
	return "\x00missing"
}

// The selector's index: one short line per recipe, and its size.
func TestRecipeSelector_Index(t *testing.T) {
	sys, schema := recipeIndexOnce()
	for _, b := range onDemandBlocks {
		for _, r := range b.recipes {
			d := recipeDesc(r.text)
			if d == "" || len([]rune(d)) > 120 || !strings.Contains(sys, r.id+": "+d+"\n") {
				t.Errorf("%s: index line %q", r.id, d)
			}
		}
	}
	if !json.Valid(schema) {
		t.Fatal("schema")
	}
	u := recipeUserMessage("ボタンを青に", kakeibo(t))
	t.Logf("selector system %d bytes (~%d tokens), schema %d bytes, kakeibo edit message %d bytes: %q", len(sys), len(sys)*2/5, len(schema), len(u), u)
}

// Haiku 5.5 is priced on its short-prompt card, and the long one above 100K.
func TestEstimateCostUSD_HaikuTiers(t *testing.T) {
	short, ok := EstimateCostUSD("claude-haiku-5-5", Usage{InputTokens: 50_000, OutputTokens: 50_000})
	if !ok || short < 0.0299 || short > 0.0301 {
		t.Fatalf("short card: %v %v", short, ok)
	}
	long, _ := EstimateCostUSD("claude-haiku-5-5", Usage{InputTokens: 200_000})
	if long < 0.0999 || long > 0.1001 {
		t.Fatalf("long card: %v", long)
	}
	if _, ok := EstimateCostUSD(DefaultRecipeModel, Usage{InputTokens: 1}); !ok {
		t.Fatal("the selector's default model must be priced")
	}
}

// A restricted build restricts the tail like the edit block: without
// AI_TEXT_IMAGE the inputPhoto recipe is dropped, and the restriction note
// stays only at the end of the first block.
func TestOnDemand_RestrictedTail(t *testing.T) {
	ctx := withRestriction(context.Background(), (&Service{}).clientRestriction(nil, nil, nil))
	pieces, ids := onDemandTail("作業の手順書をAIで作って、写真でできているか判定したい。避難訓練も", nil)
	if !slices.Contains(ids, "manual_photo") || !slices.Contains(ids, "evac_badges") {
		t.Fatalf("ids %v", ids)
	}
	got := restrictTail(ctx, pieces)
	if strings.Contains(got, "inputPhoto") || !strings.Contains(got, "routeGuide") || strings.Contains(got, "この依頼の制限") {
		t.Fatalf("restricted tail: %q", got)
	}
	if restrictTail(ctx, []string{pieces[0], "\n\n" + jphacksProducts5PhotoRule}) != "" {
		t.Fatal("a tail left with only its heading is dropped")
	}
	if restrictTail(context.Background(), pieces) != strings.Join(pieces, "") {
		t.Fatal("unrestricted tail changed")
	}
}

// callBlocks rewrites the first block only for server push (#760): recipes
// must hold none of its anchors.
func TestOnDemand_NoServerPushAnchors(t *testing.T) {
	for _, b := range onDemandBlocks {
		if rewriteServerPush(b.text) != b.text {
			t.Fatalf("an on-demand block holds a server-push anchor: %q…", b.text[:40])
		}
	}
}

// The tail goes on the wire as a third, uncached block.
func TestClientSendsSystemTailUncached(t *testing.T) {
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okBody("claude-test"))
	}))
	defer srv.Close()
	c, _ := testClient(srv, "claude-test")
	if _, err := c.Complete(context.Background(), Completion{System: "base", SystemSuffix: "\nedit", SystemTail: "\n\ntail", Messages: []Message{{Role: "user", Content: "u"}}}); err != nil {
		t.Fatal(err)
	}
	var wr wireRequest
	if err := json.Unmarshal(sent, &wr); err != nil {
		t.Fatal(err)
	}
	if len(wr.System) != 3 || wr.System[2].Text != "\n\ntail" || wr.System[2].CacheControl != nil || wr.System[1].CacheControl == nil {
		t.Fatalf("system blocks: %+v", wr.System)
	}
}

// TestOnDemand_Sizes logs the always-sent bytes and the tail added per eval
// transcript (go test -v -run OnDemand_Sizes).
func TestOnDemand_Sizes(t *testing.T) {
	core := onDemandCore(systemPrompt)
	t.Logf("systemPrompt with every recipe %d bytes, sent core %d bytes (%d on demand)", len(systemPrompt), len(core), len(systemPrompt)-len(core))
	var sizes []int
	none := 0
	for _, cs := range readEvalCases(t, "*cases.json") {
		for _, c := range cs {
			if c.Transcript == "" {
				continue
			}
			pieces, _ := onDemandTail(c.Transcript, nil)
			n := len(strings.Join(pieces, ""))
			if n == 0 {
				none++
			}
			sizes = append(sizes, n)
		}
	}
	sort.Ints(sizes)
	sum := 0
	for _, n := range sizes {
		sum += n
	}
	t.Logf("eval transcripts %d: tail none %d, mean %d, median %d, p90 %d, max %d bytes",
		len(sizes), none, sum/len(sizes), sizes[len(sizes)/2], sizes[len(sizes)*9/10], sizes[len(sizes)-1])
}
