package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// bookLookupDoc: a book form with lookupBook on a button and in the toolbar.
const bookLookupDoc = `{"schemaVersion":1,"app":{"name":"本","description":"d","icon":"book","voiceEntry":"fullscreen"},"initialScreen":"home",
"entities":[{"id":"book","name":"本","titleField":"title","fields":[
  {"id":"isbn","name":"ISBN","type":"string"},{"id":"title","name":"書名","type":"string"}]}],
"screens":[
 {"id":"home","title":"ホーム","components":[
  {"type":"list","entity":"book","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"本","entity":"book","components":[
  {"type":"qrScanner","field":"isbn"},
  {"type":"textField","field":"isbn"},
  {"type":"button","label":"ISBN から入力","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}},
  {"type":"textField","field":"title"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"book"}},{"label":"ISBN","action":{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}}]}]}`

func TestBookLookupDocIsValid(t *testing.T) {
	if _, err := dsl.Parse([]byte(bookLookupDoc)); err != nil {
		t.Fatal(err)
	}
}

// Server switch off: the prompt, schema, catalog and chat index lose
// lookupBook; on (the zero value) nothing changes.
func TestBookLookupOff_PromptAndSchema(t *testing.T) {
	on := &Service{TextImage: true}
	if r := on.clientRestriction(nil, nil, nil); r.Active() || r.NoFeature(FeatureLookupBook) {
		t.Fatal("zero Service must not restrict")
	}
	r := (&Service{TextImage: true, NoBookLookup: true}).clientRestriction(nil, nil, nil)
	if !r.Active() || !r.NoFeature(FeatureLookupBook) {
		t.Fatal("NoBookLookup must restrict")
	}
	ctx := withRestriction(context.Background(), r)
	sys := restrictPrompt(ctx, systemPrompt)
	body := withoutNote(sys)
	for _, gone := range []string{"## ISBN から本の情報を入れる(lookupBook)", "type: lookupBook", "bookLookup", "- lookupBook{"} {
		if strings.Contains(body, gone) {
			t.Errorf("prompt still has %q", gone)
		}
	}
	if !strings.Contains(sys, "ボタンの lookupBook(ISBN から本の情報を入れる)は使わない") {
		t.Error("note missing")
	}
	if s := string(restrictSchema(ctx, generationSchema)); strings.Contains(s, "lookupBook") || strings.Contains(s, "bookLookup") {
		t.Error("schema still has lookupBook")
	}
	if strings.Contains(r.assets().index, "lookupBook") || strings.Contains(r.assets().catalog, "lookupBook") {
		t.Error("chat capability index still has lookupBook")
	}
	// with a client restriction too, both apply
	a, d := reviewBuild()
	plain := (&Service{TextImage: true}).clientRestriction(a, d, allFeatureKeys())
	rb := (&Service{TextImage: true, NoBookLookup: true}).clientRestriction(a, d, allFeatureKeys())
	if !rb.NoFeature(FeatureLookupBook) || !rb.NoSharedData() || rb.key == plain.key {
		t.Fatalf("combined restriction: %+v", rb)
	}
}

// Create with the switch off: a lookupBook answer is retried, and on the
// last attempt the buttons are removed.
func TestBookLookupOff_GenerateRetriesAndStrips(t *testing.T) {
	f := &fakeCaller{replies: []string{bookLookupDoc, bookLookupDoc}}
	svc, _ := newSvc(f)
	svc.NoBookLookup = true
	svc.MaxRegenerations = 1
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Mode: ModeCreate, Transcript: "本の貸し借りアプリ。ISBN から書名を入れたい"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, "action lookupBook is not available:") {
		t.Fatalf("expected a retry: %+v", res.Meta)
	}
	if strings.Contains(f.calls[0].System, "type: lookupBook") || !strings.Contains(f.calls[0].System, "## この依頼の制限") {
		t.Fatal("restricted prompt not sent")
	}
	if strings.Contains(string(res.DSL), "lookupBook") {
		t.Fatalf("lookupBook left: %s", res.DSL)
	}
	if _, err := dsl.Parse(res.DSL); err != nil {
		t.Fatalf("stripped DSL invalid: %v", err)
	}
	if !strings.Contains(strings.Join(res.Meta.Restricted, ","), "dropped action.lookupBook on 2 place(s)") {
		t.Fatalf("restricted notes: %v", res.Meta.Restricted)
	}
}

// Switch on (zero value): lookupBook is generated as before.
func TestBookLookupOff_EnabledKeepsLookupBook(t *testing.T) {
	f := &fakeCaller{replies: []string{bookLookupDoc}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Mode: ModeCreate, Transcript: "本の貸し借りアプリ。ISBN から書名を入れたい"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 1 || !strings.Contains(string(res.DSL), "lookupBook") || !strings.Contains(f.calls[0].System, "type: lookupBook") {
		t.Fatalf("lookupBook should stay: attempts=%d %s", res.Meta.Attempts, res.DSL)
	}
}

// Existing buttons are not new uses: moving them is no violation.
func TestBookLookupOff_ViolationsCountNewOnly(t *testing.T) {
	r := (&Service{TextImage: true, NoBookLookup: true}).clientRestriction(nil, nil, nil)
	if v := featureViolations([]byte(bookLookupDoc), []byte(bookLookupDoc), r); len(v) != 0 {
		t.Fatalf("unchanged doc: %v", v)
	}
	if v := featureViolations([]byte(bookLookupDoc), nil, r); len(v) != 2 {
		t.Fatalf("create: %v", v)
	}
	if v := featureViolations([]byte(bookLookupDoc), nil, Restriction{}); v != nil {
		t.Fatalf("unrestricted: %v", v)
	}
}

// Chat (clarify) sends the restricted prompt too.
func TestBookLookupOff_Chat(t *testing.T) {
	svc, f := newChatSvc(questionReply)
	svc.NoBookLookup = true
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "c1", Messages: user("本の記録")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[0].System, "ボタンの lookupBook(ISBN から本の情報を入れる)は使わない") {
		t.Fatal("restricted chat prompt not sent")
	}
}

// allFeatureKeys: supportedFeatures of a client that has every clientFeature.
func allFeatureKeys() []string {
	keys := make([]string, 0, len(clientFeatures))
	for _, f := range clientFeatures {
		keys = append(keys, f.key)
	}
	return keys
}
