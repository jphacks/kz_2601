package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// bookGateDoc: a book form with lookupBook on a button and in the toolbar.
const bookGateDoc = `{"schemaVersion":1,"app":{"name":"本","description":"d","icon":"book","voiceEntry":"fullscreen"},"initialScreen":"home",
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

func TestBookGateDocIsValid(t *testing.T) {
	if _, err := dsl.Parse([]byte(bookGateDoc)); err != nil {
		t.Fatal(err)
	}
}

// Server switch off: the prompt, schema, catalog and chat index lose
// lookupBook; on (the zero value) nothing changes.
func TestBookGate_PromptAndSchema(t *testing.T) {
	on := &Service{}
	if r := on.bookGate(Restriction{}); r.Active() || r.NoBookLookup() {
		t.Fatal("zero Service must not restrict")
	}
	r := (&Service{NoBookLookup: true}).bookGate(Restriction{})
	if !r.Active() || !r.NoBookLookup() {
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
	rb := (&Service{NoBookLookup: true}).bookGate(NewRestriction(a, d))
	if !rb.NoBookLookup() || !rb.NoSharedData() || rb.key == NewRestriction(a, d).key {
		t.Fatalf("combined restriction: %+v", rb)
	}
}

// Create with the switch off: a lookupBook answer is retried, and on the
// last attempt the buttons are removed.
func TestBookGate_GenerateRetriesAndStrips(t *testing.T) {
	f := &fakeCaller{replies: []string{bookGateDoc, bookGateDoc}}
	svc, _ := newSvc(f)
	svc.NoBookLookup = true
	svc.MaxRegenerations = 1
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Mode: ModeCreate, Transcript: "本の貸し借りアプリ。ISBN から書名を入れたい"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, "action lookupBook is not available now") {
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
	if !strings.Contains(strings.Join(res.Meta.Restricted, ","), "removed 2 lookupBook button") {
		t.Fatalf("restricted notes: %v", res.Meta.Restricted)
	}
}

// Switch on (zero value): lookupBook is generated as before.
func TestBookGate_EnabledKeepsLookupBook(t *testing.T) {
	f := &fakeCaller{replies: []string{bookGateDoc}}
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
func TestBookGate_ViolationsCountNewOnly(t *testing.T) {
	r := (&Service{NoBookLookup: true}).bookGate(Restriction{})
	if v := bookViolations([]byte(bookGateDoc), []byte(bookGateDoc), r); len(v) != 0 {
		t.Fatalf("unchanged doc: %v", v)
	}
	if v := bookViolations([]byte(bookGateDoc), nil, r); len(v) != 2 {
		t.Fatalf("create: %v", v)
	}
	if v := bookViolations([]byte(bookGateDoc), nil, Restriction{}); v != nil {
		t.Fatalf("unrestricted: %v", v)
	}
}

// Chat (clarify) sends the restricted prompt too.
func TestBookGate_Chat(t *testing.T) {
	svc, f := newChatSvc(questionReply)
	svc.NoBookLookup = true
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "c1", Messages: user("本の記録")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[0].System, "ボタンの lookupBook(ISBN から本の情報を入れる)は使わない") {
		t.Fatal("restricted chat prompt not sent")
	}
}
