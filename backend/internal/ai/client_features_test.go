package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// featuresDoc uses every clientFeature once: aiButton long / inputPhoto /
// saveTo / inputFeeds / outputs, lookupBook (button and toolbar), text {{field}} + highlight, a
// warnings feed, addMinutes and aiChat inputEntity.
const featuresDoc = `{"schemaVersion":1,"app":{"name":"本","description":"d","icon":"book","voiceEntry":"fullscreen"},"initialScreen":"home",
"entities":[
 {"id":"player","name":"出場者","titleField":"name","fields":[{"id":"name","name":"名前","type":"string"},{"id":"wins","name":"勝ち上がり","type":"integer"}]},
 {"id":"book","name":"本","titleField":"title","fields":[
  {"id":"isbn","name":"ISBN","type":"string"},{"id":"title","name":"書名","type":"string"},
  {"id":"body","name":"本文","type":"text"},{"id":"keywords","name":"語","type":"string"},
  {"id":"photo","name":"写真","type":"photo"},{"id":"note","name":"メモ","type":"text"},
  {"id":"meet","name":"集合","type":"string"},{"id":"leave","name":"出発","type":"string","computed":{"op":"addMinutes","args":[{"ref":"meet"},-30]}}]},
 {"id":"card","name":"カード","titleField":"q","fields":[{"id":"q","name":"問題","type":"string","required":true}]},
 {"id":"turn","name":"会話","fields":[{"id":"text","name":"本文","type":"text","required":true},{"id":"role","name":"話した人","type":"enum","options":["user","assistant"]}]}],
"screens":[
 {"id":"home","title":"ホーム","components":[
  {"type":"bracket","entity":"player","titleField":"name","winsField":"wins"},
  {"type":"feed","provider":"warnings","region":"130000"},
  {"type":"feed","id":"wx","provider":"weather","region":"170000"},
  {"type":"aiButton","task":"generate","label":"問題を作る","prompt":"英単語の問題","shape":"list","count":5,"saveTo":{"entity":"card","field":"q"}},
  {"type":"aiButton","task":"generate","label":"服装","prompt":"今日の天気に合う服装","inputFeeds":["wx"]},
  {"type":"list","entity":"book","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"本","entity":"book","components":[
  {"type":"textField","field":"isbn"},
  {"type":"button","label":"ISBN から入力","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}},
  {"type":"textField","field":"title"},
  {"type":"textField","field":"body"},
  {"type":"aiButton","task":"generate","label":"書名を読む","input":"body","prompt":"本文から書名","shape":"fields","items":["書名"],"outputs":{"書名":"title"}},
  {"type":"aiButton","task":"summarize","input":"body","output":"note","length":"long","label":"要約"},
  {"type":"photoField","field":"photo","source":"library"},
  {"type":"aiButton","task":"extract","inputPhoto":"photo","output":"note","label":"写真を読む"},
  {"type":"text","text":"{{body}}","highlight":{"field":"keywords"}},
  {"type":"text","text":"本の記録"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"book"}},{"label":"ISBN","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}}]},
 {"id":"talk","title":"相談","components":[
  {"type":"aiChat","area":"A1:F12","entity":"turn","textField":"text","roleField":"role","persona":"本の記録を読んで話す相手","inputEntity":"book","limit":5}]}]}`

func TestClientFeaturesDocIsValid(t *testing.T) {
	if _, err := dsl.Parse([]byte(featuresDoc)); err != nil {
		t.Fatal(err)
	}
}

// A build without one of the features: its rules leave the prompt, its keys
// the schema, a new use is a violation and enforceRestriction removes it
// (the result stays valid). A current build stays unrestricted.
func TestClientFeaturesRestriction(t *testing.T) {
	all := allComponentTypes()
	if r := NewClientRestriction(all, nil, currentFeatures); r.Active() {
		t.Fatal("a current build must stay unrestricted")
	}
	gone := map[string]struct {
		rule   string // a line of the dropped rules
		schema string // Def.prop (or a fragment) that must go
		places int    // uses in featuresDoc
		left   string // a doc fragment that must go
	}{
		FeatureAILong:         {"## アプリの中の AI の長い文章(aiButton length long)", "AiButton.length", 1, `"length"`},
		FeatureAIInputPhoto:   {"## アプリの中の AI に写真を見せる(aiButton inputPhoto)", "AiButton.inputPhoto", 1, `"inputPhoto"`},
		FeatureAISaveTo:       {"## AI が作った項目を 1 件ずつ記録に足す(aiButton saveTo)", "AiButton.saveTo", 1, `"saveTo"`},
		FeatureAIInputFeeds:   {"inputFeeds: [その feed の id]", "AiButton.inputFeeds", 1, `"inputFeeds"`},
		FeatureAIOutputs:      {"## AI の答えを項目ごとに入力欄へ入れる(aiButton outputs)", "AiButton.outputs", 1, `"outputs"`},
		FeatureLookupBook:     {"## ISBN から本の情報を入れる(lookupBook)", "Action.bookLookup", 2, `"lookupBook"`},
		FeatureTextHighlight:  {"## 文の大事な語に印をつける(text.highlight)", "Text.highlight", 1, `{{body}}`},
		FeatureFeedV2:         {"警報・注意報」「大雨警報が出ているか」", `"warnings"`, 1, `"warnings"`},
		FeatureExprAddMinutes: {"## 時刻に分を足す・引く(addMinutes)", `"addMinutes"`, 1, `"addMinutes"`},
		FeatureAIChatInput:    {"## 自分の記録を読んで話す(aiChat inputEntity)", "AiChat.inputEntity", 1, `"inputEntity"`},
		FeatureBracket:        {"## トーナメント表(bracket)", `"#/$defs/Bracket"`, 1, `"bracket"`},
	}
	if len(gone) != len(clientFeatures) {
		t.Fatalf("test covers %d of %d features", len(gone), len(clientFeatures))
	}
	for _, f := range clientFeatures {
		t.Run(f.key, func(t *testing.T) {
			g := gone[f.key]
			r := NewClientRestriction(all, nil, featuresWithout(f.key))
			if !r.Active() || !r.NoFeature(f.key) || r.NoNear() {
				t.Fatalf("active=%v off=%v", r.Active(), r.off)
			}
			if !strings.Contains(systemPrompt, g.rule) || !schemaHas(t, dsl.SchemaJSON(), g.schema) {
				t.Fatal("the full prompt / schema lack the part")
			}
			p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
			if strings.Contains(withoutNote(p), g.rule) || !strings.Contains(p, strings.TrimSpace(f.note)) {
				t.Fatal("restricted prompt: rules left / note missing")
			}
			if schemaHas(t, restrictSchemaJSON(dsl.SchemaJSON(), r), g.schema) {
				t.Fatalf("restricted schema still has %s", g.schema)
			}
			v := restrictionViolations([]byte(featuresDoc), nil, r)
			if len(v) != g.places {
				t.Fatalf("violations = %v", v)
			}
			if v := restrictionViolations([]byte(featuresDoc), []byte(featuresDoc), r); len(v) != 0 {
				t.Fatalf("existing uses are not new: %v", v)
			}
			out, notes, changed := enforceRestriction([]byte(featuresDoc), r)
			if !changed || len(notes) != 1 || strings.Contains(string(out), g.left) {
				t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
			}
			if _, err := dsl.Parse(out); err != nil {
				t.Fatalf("enforced doc invalid: %v", err)
			}
		})
	}
}

// schemaHas: "Def.prop" is that property of the def in the canonical-shaped
// schema (the generation schema flattens leaves); anything else a fragment.
func schemaHas(t *testing.T, sch []byte, what string) bool {
	t.Helper()
	def, prop, ok := strings.Cut(what, ".")
	if !ok || strings.ContainsAny(what, `"`) {
		return strings.Contains(string(sch), what)
	}
	var root struct {
		Defs map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(sch, &root); err != nil {
		t.Fatal(err)
	}
	return root.Defs[def].Properties[prop] != nil
}

// AI_TEXT_IMAGE off (the default): every request, restricted or not, is
// told not to use inputPhoto, so a generated photo button never meets the 403.
func TestTextImageOffRestrictsPrompt(t *testing.T) {
	off := (&Service{}).clientRestriction(nil, nil, nil)
	if !off.Active() || !off.NoFeature(FeatureAIInputPhoto) || off.NoFeature(FeatureAILong) {
		t.Fatalf("switch off: active=%v off=%v", off.Active(), off.off)
	}
	if on := (&Service{TextImage: true}).clientRestriction(nil, nil, nil); on.Active() {
		t.Fatal("switch on and no client restriction: unrestricted")
	}
	// on top of a client restriction too
	old := (&Service{}).clientRestriction(allComponentTypes(), nil, featuresWithout(FeatureAILong))
	if !old.NoFeature(FeatureAILong) || !old.NoFeature(FeatureAIInputPhoto) {
		t.Fatalf("combined: %v", old.off)
	}

	f := &fakeCaller{replies: []string{featuresDoc, featuresDoc, featuresDoc, featuresDoc, featuresDoc}}
	svc, _ := newSvc(f)
	svc.TextImage = false
	res, err := svc.Generate(context.Background(), Request{AppID: "p1", Transcript: "本の記録"})
	if err != nil {
		t.Fatal(err)
	}
	c := f.calls[len(f.calls)-1]
	if strings.Contains(withoutNote(c.System), "aiButton に inputPhoto") || strings.Contains(c.System, "inputPhoto の写真 1 枚") || strings.Contains(string(c.Schema), `"inputPhoto"`) {
		t.Fatal("prompt / schema still teach inputPhoto")
	}
	if strings.Contains(string(res.DSL), `"inputPhoto"`) || !strings.Contains(string(res.DSL), `"length"`) {
		t.Fatalf("output: %s", res.DSL)
	}
}

// A feed removed by the restriction (a build without feed) takes the
// inputFeeds ids that name it along, so the enforced document stays valid.
func TestPruneInputFeedsOfRemovedFeed(t *testing.T) {
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != string(dsl.CompFeed) {
			allowed = append(allowed, typ)
		}
	}
	r := NewClientRestriction(allowed, nil, currentFeatures)
	out, notes, changed := enforceRestriction([]byte(featuresDoc), r)
	if !changed || strings.Contains(string(out), `"inputFeeds"`) || !strings.Contains(strings.Join(notes, ","), "dropped inputFeeds of removed feeds on 1 button(s)") {
		t.Fatalf("notes=%v out=%s", notes, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatalf("enforced doc invalid: %v", err)
	}
	root := map[string]any{"screens": []any{map[string]any{"id": "s", "components": []any{
		map[string]any{"type": "feed", "id": "a"},
		map[string]any{"type": "aiButton", "inputFeeds": []any{"a", "gone", "typo"}},
	}}}}
	if n := pruneInputFeeds(root, map[string]map[string]bool{"s": {"gone": true}}); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	b, _ := json.Marshal(root)
	if !strings.Contains(string(b), `"inputFeeds":["a","typo"]`) {
		t.Fatalf("kept ids: %s", b)
	}
}

// An inputFeeds id that names no feed for another reason (a typo) is not
// pruned by an active restriction: validation still reports it for repair.
func TestPruneInputFeedsKeepsTypo(t *testing.T) {
	doc := strings.Replace(featuresDoc, `"inputFeeds":["wx"]`, `"inputFeeds":["wx","wz"]`, 1)
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != string(dsl.CompPhotoField) {
			allowed = append(allowed, typ)
		}
	}
	r := NewClientRestriction(allowed, nil, currentFeatures)
	out, notes, changed := enforceRestriction([]byte(doc), r)
	if !changed || !strings.Contains(string(out), `"inputFeeds":["wx","wz"]`) || strings.Contains(strings.Join(notes, ","), "inputFeeds") {
		t.Fatalf("notes=%v out=%s", notes, out)
	}
	if _, err := dsl.Parse(out); err == nil || !strings.Contains(err.Error(), "wz") {
		t.Fatalf("typo not reported: %v", err)
	}
}
