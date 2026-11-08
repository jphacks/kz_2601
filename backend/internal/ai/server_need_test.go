package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"unsafe"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// withNeed appends the root serverNeed (as the model writes it, last).
func withNeed(t *testing.T, app, need string) string {
	t.Helper()
	s := strings.TrimSpace(app)
	if !strings.HasSuffix(s, "}") {
		t.Fatal("app is not an object")
	}
	return s[:len(s)-1] + `,"serverNeed":"` + need + `"}`
}

// Transcripts the regex misses / matches.
const (
	noAskTr = "サークルの出欠を作って"
	askTr   = "サークルの出欠。出欠がついたら予定を作った人に通知して"
)

func TestServerNeed_Trigger(t *testing.T) {
	for _, c := range []struct{ need, tr, want string }{
		{"push", noAskTr, "model"},
		{"timed", askTr, "both"},
		{"none", askTr, "regex"},
		{"", askTr, "regex"}, // missing field: the regex is the safety net
		{"none", noAskTr, ""},
		{"", noAskTr, ""},
	} {
		if got := serverRulesTrigger(c.need, c.tr); got != c.want {
			t.Errorf("need=%q tr=%q: %q, want %q", c.need, c.tr, got, c.want)
		}
	}
	if mergeServerNeed("none", "push") != "push" || mergeServerNeed("record", "none") != "record" || mergeServerNeed("", "none") != "none" {
		t.Error("mergeServerNeed must OR the lint retry")
	}
}

func TestServerNeed_TakeStrips(t *testing.T) {
	raw := []byte(withNeed(t, rsvpApp, "push"))
	out, need, seen := takeServerNeed(raw)
	if need != "push" || !seen || bytes.Contains(out, []byte("serverNeed")) {
		t.Fatalf("need=%q seen=%t out=%s", need, seen, out)
	}
	if _, err := dsl.Parse(fillDefaults(out)); err != nil {
		t.Fatalf("stripped DSL must parse: %v", err)
	}
	if _, err := dsl.Parse(fillDefaults(raw)); err == nil {
		t.Fatal("dsl.Parse must reject the unknown root key (why it is stripped)")
	}
	// absent: the same bytes (flag off is byte-identical)
	if out, _, seen := takeServerNeed([]byte(rsvpApp)); seen || string(out) != rsvpApp {
		t.Fatal("no serverNeed: bytes must pass unchanged")
	}
	// an unknown value is seen but not trusted
	if _, need, seen := takeServerNeed([]byte(withNeed(t, rsvpApp, "bogus"))); need != "" || !seen {
		t.Fatalf("bogus: need=%q seen=%t", need, seen)
	}
	if need, seen := editServerNeed([]byte(`{"summary":"x","patch":[],"serverNeed":"record"}`)); need != "record" || !seen {
		t.Fatalf("edit: need=%q seen=%t", need, seen)
	}
	if _, seen := editServerNeed([]byte(`{"summary":"x","patch":[]}`)); seen {
		t.Fatal("edit without the key must be missing")
	}
}

// Create: the model asks with serverNeed although the regex misses; the key
// never reaches the stored DSL.
func TestServerNeed_CreateModelOnly(t *testing.T) {
	f := &fakeCaller{replies: []string{withNeed(t, rsvpApp, "push"), rsvpRules}}
	svc, store := newSvc(f)
	svc.ServerRulesPass = true
	res, err := svc.Generate(context.Background(), Request{AppID: "need-create", Transcript: noAskTr})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[1].System != serverRulesPrompt {
		t.Fatalf("the pass must run on serverNeed alone (%d calls)", len(f.calls))
	}
	sm := res.Meta.ServerRules
	if sm == nil || !sm.Applied || sm.Need != "push" || sm.Trigger != "model" {
		t.Fatalf("meta: %+v", sm)
	}
	saved, _ := store.Get(context.Background(), "need-create")
	if bytes.Contains(saved.DSL, []byte("serverNeed")) || bytes.Contains(res.DSL, []byte("serverNeed")) {
		t.Fatal("serverNeed leaked into the DSL")
	}
	// the main call got the field in its schema, last in required
	var sch struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(f.calls[0].Schema, &sch); err != nil {
		t.Fatal(err)
	}
	if sch.Properties["serverNeed"] == nil || sch.Required[len(sch.Required)-1] != "serverNeed" {
		t.Fatalf("schema: required=%v", sch.Required)
	}
	if len(f.calls[0].Schema) > 21000 {
		t.Fatalf("generation schema with serverNeed too large: %d", len(f.calls[0].Schema))
	}
}

// Flag off: a leaked key is still stripped (never stored) and the pass is off.
func TestServerNeed_LeakedKeyFlagOff(t *testing.T) {
	f := &fakeCaller{replies: []string{withNeed(t, rsvpApp, "push")}}
	svc, store := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "need-leak", Transcript: askTr})
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Get(context.Background(), "need-leak")
	if len(f.calls) != 1 || res.Meta.ServerRules != nil || bytes.Contains(saved.DSL, []byte("serverNeed")) {
		t.Fatalf("calls=%d meta=%+v", len(f.calls), res.Meta.ServerRules)
	}
}

func TestServerNeed_EditDecodes(t *testing.T) {
	patch := `{"summary":"名前を変えました。","patch":[{"op":"replace","path":"/app/name","value":"部の出欠"}],"serverNeed":"push"}`
	f := &fakeCaller{replies: []string{patch, rsvpRules}}
	svc, store := newSvc(f)
	svc.ServerRulesPass = true
	if _, err := store.Put(context.Background(), "need-edit", 0, []byte(rsvpApp), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Generate(context.Background(), Request{AppID: "need-edit", Transcript: "名前を部の出欠にして"})
	if err != nil {
		t.Fatal(err)
	}
	if sm := res.Meta.ServerRules; sm == nil || !sm.Applied || sm.Trigger != "model" || sm.Need != "push" {
		t.Fatalf("meta: %+v", sm)
	}
	if !bytes.Contains(f.calls[0].Schema, []byte(`"serverNeed"`)) {
		t.Fatal("edit schema must carry serverNeed")
	}
}

// Gate truth table through Generate: neither / regex only / both / missing.
func TestServerNeed_GateTable(t *testing.T) {
	unchanged := `{"unchanged":true,"declined":"","serverRules":[]}`
	for _, c := range []struct {
		name, app, tr, trigger string
		ran, logged            bool
	}{
		{"neither", withNeed(t, rsvpApp, "none"), noAskTr, "", false, false},
		{"regex only", withNeed(t, rsvpApp, "none"), askTr, "regex", true, false},
		{"both", withNeed(t, rsvpApp, "record"), askTr, "both", true, false},
		{"missing, regex", rsvpApp, askTr, "regex", true, true},
		{"missing, no regex", rsvpApp, noAskTr, "", false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeCaller{replies: []string{c.app, unchanged}}
			svc, _ := newSvc(f)
			svc.ServerRulesPass = true
			var logs bytes.Buffer
			svc.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			res, err := svc.Generate(context.Background(), Request{AppID: "gate-" + c.name, Transcript: c.tr})
			if err != nil {
				t.Fatal(err)
			}
			if ran := len(f.calls) == 2; ran != c.ran {
				t.Fatalf("ran=%t want %t", ran, c.ran)
			}
			if c.ran && res.Meta.ServerRules.Trigger != c.trigger {
				t.Fatalf("trigger=%q want %q", res.Meta.ServerRules.Trigger, c.trigger)
			}
			if got := strings.Contains(logs.String(), "ai.server_need missing"); got != c.logged {
				t.Fatalf("missing log=%t want %t", got, c.logged)
			}
		})
	}
}

// The model asked but the pass made nothing: the summary says so.
func TestServerNeed_UnchangedWithNeedSaysFailed(t *testing.T) {
	for _, c := range []struct {
		need string
		want bool
	}{{"push", true}, {"none", false}} {
		f := &fakeCaller{replies: []string{withNeed(t, rsvpApp, c.need), `{"unchanged":true,"declined":"","serverRules":[]}`}}
		svc, _ := newSvc(f)
		svc.ServerRulesPass = true
		res, err := svc.Generate(context.Background(), Request{AppID: "fail-" + c.need, Transcript: askTr})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(res.Summary, LocaleJA.serverRulesFailed()); got != c.want {
			t.Fatalf("need=%s: failed sentence=%t want %t (%q)", c.need, got, c.want, res.Summary)
		}
	}
}

// Flag off: the create / edit prompt and schema are today's bytes.
func TestServerNeed_FlagOffByteIdentical(t *testing.T) {
	f := &fakeCaller{replies: []string{string(kakeibo(t))}}
	svc, _ := newSvc(f)
	if _, err := svc.Generate(context.Background(), Request{AppID: "off", Transcript: "家計簿を作って。毎日通知して"}); err != nil {
		t.Fatal(err)
	}
	if f.calls[0].System != systemPrompt || !bytes.Equal(f.calls[0].Schema, GenerationSchema()) {
		t.Fatal("create: prompt / schema changed with the flag off")
	}
	f.replies = []string{`{"summary":"名前を変えました。","patch":[{"op":"replace","path":"/app/name","value":"家計"}]}`}
	if _, err := svc.Generate(context.Background(), Request{AppID: "off", Transcript: "名前を家計にして"}); err != nil {
		t.Fatal(err)
	}
	if f.calls[1].System != systemPrompt+"\n"+editRules || !bytes.Equal(f.calls[1].Schema, PatchGenerationSchema()) {
		t.Fatal("edit: prompt / schema changed with the flag off")
	}
	if strings.Contains(systemPrompt, "serverNeed") || bytes.Contains(GenerationSchema(), []byte("serverNeed")) || bytes.Contains(PatchGenerationSchema(), []byte("serverNeed")) {
		t.Fatal("static prompt / schemas must not mention serverNeed")
	}
}

// The section sits in block 1 (systemPrompt): edit = create + "\n" + editRules.
func TestServerNeed_SectionInBlockOne(t *testing.T) {
	ctx := withServerPush(context.Background(), true)
	create := serverPushPrompt(ctx, systemPrompt)
	edit := serverPushPrompt(ctx, systemPrompt+"\n"+editRules)
	if edit != create+"\n"+editRules {
		t.Fatal("edit must be the create prompt + editRules")
	}
	if strings.Count(create, serverNeedSection) != 1 || !strings.Contains(create, serverNeedSection+"\n\n## ") {
		t.Fatal("section must be one block before the next heading")
	}
	// logic / server-rules prompts and schemas stay as they are
	if serverPushPrompt(ctx, logicSystemPrompt) != logicSystemPrompt || serverPushPrompt(ctx, serverRulesPrompt) != serverRulesPrompt {
		t.Fatal("other passes must not get the section")
	}
	if !bytes.Equal(serverNeedSchema(WithOperation(ctx, OpLogic), PatchGenerationSchema()), PatchGenerationSchema()) {
		t.Fatal("the logic pass keeps PatchGenerationSchema")
	}
}

func TestServerNeed_Memoized(t *testing.T) {
	ctx := withServerPush(context.Background(), true)
	a, b := serverPushPrompt(ctx, systemPrompt), serverPushPrompt(ctx, systemPrompt)
	if unsafe.StringData(a) != unsafe.StringData(b) {
		t.Fatal("prompt rewrite must be memoized (stable bytes for the cache)")
	}
	gctx := WithOperation(ctx, OpGenerate)
	x, y := serverNeedSchema(gctx, GenerationSchema()), serverNeedSchema(gctx, GenerationSchema())
	if &x[0] != &y[0] {
		t.Fatal("schema must be memoized")
	}
}
