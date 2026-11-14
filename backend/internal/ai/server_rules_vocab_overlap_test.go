package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestServerRulesOverlapPrompt(t *testing.T) {
	if !strings.Contains(serverRulesPrompt, serverRulesOverlapSection) || !strings.Contains(serverRulesReadsSection, serverRulesOverlapSection) {
		t.Fatal("overlap section not in the prompt")
	}
	for _, s := range []string{"予約が重なったら知らせて", "ダブルブッキングを防ぎたい", "シフトが被ったら通知"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
}

const bookingsApp = `{"schemaVersion":1,"app":{"name":"会議室の予約","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"bookings","name":"予約","titleField":"title","fields":[
  {"id":"title","name":"件名","type":"string","required":true},
  {"id":"start","name":"開始","type":"datetime","required":true},
  {"id":"end","name":"終了","type":"datetime"}]}],
"screens":[{"id":"home","title":"予約","components":[{"type":"list","entity":"bookings","item":{"title":"title"}}]}]}`

// The prompt's double booking in the wire form validates and notifies on a
// clash only.
func TestServerRulesOverlapWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"double","on":{"record":"created","entity":"bookings"},
"lookup":{"op":"overlaps","entity":"bookings","field":"start","fieldEnd":"end","equals":"start","equalsEnd":"end"},
"do":[{"notify":{"to":{"kind":"owner"},"title":"予定が重なっています","body":"{title} がほかの予約と重なっています"}}]}]}`
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(bookingsApp), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	refs := listMap{"bookings": {{ID: "b1", Data: map[string]any{"title": "定例", "start": "2026-11-10T10:00:00+09:00", "end": "2026-11-10T11:00:00+09:00"}}}}
	plan := func(start, end string) engine.Plan {
		t.Helper()
		rec := engine.Trigger{AppID: "a", Kind: engine.KindCreated, Entity: "bookings"}
		rec.Record.ID = "n1"
		rec.Record.Data = map[string]any{"title": "面談", "start": start, "end": end}
		p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], rec, refs)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := plan("2026-11-10T10:30:00+09:00", "2026-11-10T11:30:00+09:00"); len(p.Notifies) != 1 || p.Notifies[0].Body != "面談 がほかの予約と重なっています" {
		t.Fatalf("clash: %+v", p)
	}
	// only the row's end reaches into b1: equalsEnd survives the wire form
	if p := plan("2026-11-10T09:00:00+09:00", "2026-11-10T10:30:00+09:00"); len(p.Notifies) != 1 {
		t.Fatalf("clash through the end: %+v", p)
	}
	if p := plan("2026-11-10T11:00:00+09:00", "2026-11-10T12:00:00+09:00"); !p.Empty() {
		t.Fatalf("back to back: %+v", p)
	}
}
