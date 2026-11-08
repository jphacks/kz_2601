package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// featuresDoc uses every clientFeature once: aiButton long / inputPhoto /
// saveTo, lookupBook (button and toolbar), text {{field}} + highlight and a
// warnings feed.
const featuresDoc = `{"schemaVersion":1,"app":{"name":"本","description":"d","icon":"book","voiceEntry":"fullscreen"},"initialScreen":"home",
"entities":[
 {"id":"book","name":"本","titleField":"title","fields":[
  {"id":"isbn","name":"ISBN","type":"string"},{"id":"title","name":"書名","type":"string"},
  {"id":"body","name":"本文","type":"text"},{"id":"keywords","name":"語","type":"string"},
  {"id":"photo","name":"写真","type":"photo"},{"id":"note","name":"メモ","type":"text"}]},
 {"id":"card","name":"カード","titleField":"q","fields":[{"id":"q","name":"問題","type":"string","required":true}]}],
"screens":[
 {"id":"home","title":"ホーム","components":[
  {"type":"feed","provider":"warnings","region":"130000"},
  {"type":"feed","provider":"weather","region":"170000"},
  {"type":"aiButton","task":"generate","label":"問題を作る","prompt":"英単語の問題","shape":"list","count":5,"saveTo":{"entity":"card","field":"q"}},
  {"type":"list","entity":"book","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"本","entity":"book","components":[
  {"type":"textField","field":"isbn"},
  {"type":"button","label":"ISBN から入力","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}},
  {"type":"textField","field":"title"},
  {"type":"textField","field":"body"},
  {"type":"aiButton","task":"summarize","input":"body","output":"note","length":"long","label":"要約"},
  {"type":"photoField","field":"photo","source":"library"},
  {"type":"aiButton","task":"extract","inputPhoto":"photo","output":"note","label":"写真を読む"},
  {"type":"text","text":"{{body}}","highlight":{"field":"keywords"}},
  {"type":"text","text":"本の記録"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"book"}},{"label":"ISBN","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}}]}]}`

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
		schema string // a schema fragment that must go
		places int    // uses in featuresDoc
		left   string // a doc fragment that must go
	}{
		FeatureAILong:        {"## アプリの中の AI の長い文章(aiButton length long)", `"length":`, 1, `"length"`},
		FeatureAIInputPhoto:  {"## アプリの中の AI に写真を見せる(aiButton inputPhoto)", `"inputPhoto":`, 1, `"inputPhoto"`},
		FeatureAISaveTo:      {"## AI が作った項目を 1 件ずつ記録に足す(aiButton saveTo)", `"saveTo":`, 1, `"saveTo"`},
		FeatureLookupBook:    {"## ISBN から本の情報を入れる(lookupBook)", `"bookLookup":`, 2, `"lookupBook"`},
		FeatureTextHighlight: {"## 文の大事な語に印をつける(text.highlight)", `"highlight":`, 1, `{{body}}`},
		FeatureFeedV2:        {"警報・注意報」「大雨警報が出ているか」", `"warnings"`, 1, `"warnings"`},
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
			if !strings.Contains(systemPrompt, g.rule) || !strings.Contains(string(generationSchema), g.schema) {
				t.Fatal("the full prompt / schema lack the part")
			}
			p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
			if strings.Contains(withoutNote(p), g.rule) || !strings.Contains(p, strings.TrimSpace(f.note)) {
				t.Fatal("restricted prompt: rules left / note missing")
			}
			if strings.Contains(string(r.assets().genSch), g.schema) {
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
