package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// init swapped every base line (a renamed base line would silently keep
// "agg は使えない" in the prompt).
func TestServerRulesReadsPrompt(t *testing.T) {
	for _, l := range serverRulesReadsLines {
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, l[1]) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if !strings.HasSuffix(serverRulesPrompt, serverRulesReadsSection) {
		t.Fatal("reads section not appended")
	}
	for _, s := range []string{"agg・streak・doneInDays・computed field・setting は使えない"} {
		if strings.Contains(serverRulesPrompt, s) {
			t.Fatalf("stale rule %q", s)
		}
	}
}

const expenseApp = `{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"expenses","name":"支出","titleField":"memo","fields":[
  {"id":"memo","name":"メモ","type":"string"},
  {"id":"date","name":"日付","type":"date","required":true},
  {"id":"amount","name":"金額","type":"currency","required":true}]},
 {"id":"summaries","name":"週のまとめ","fields":[
  {"id":"week","name":"週","type":"date","required":true},
  {"id":"total","name":"合計","type":"currency"}]}],
"screens":[{"id":"home","title":"家計","components":[{"type":"list","entity":"expenses","item":{"title":"memo"}}]}]}`

// with and values in the wire form ([{name|field, value}]).
const weeklyRules = `{"unchanged":false,"declined":"","serverRules":[{"id":"weekly","on":{"every":"weekly","weekday":"sun","time":"20:00"},
"do":[{"createRecord":{"entity":"summaries","values":[
  {"field":"week","value":{"op":"today","args":[]}},
  {"field":"total","value":{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisWeek","value":null}}}]}},
 {"notify":{"to":{"kind":"owner"},"title":"今週のまとめ","body":"今週は {total} 円でした",
  "with":[{"name":"total","value":{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisWeek","value":null}}}]}}]}]}`

type listMap map[string][]expreval.Record

func (m listMap) Record(_ context.Context, entity, id string) (expreval.Record, bool, error) {
	for _, r := range m[entity] {
		if r.ID == id {
			return r, true, nil
		}
	}
	return expreval.Record{}, false, nil
}

func (m listMap) List(_ context.Context, entity string) ([]expreval.Record, error) {
	return m[entity], nil
}

func TestServerRulesPass_WeeklySummaryWithAgg(t *testing.T) {
	res, _, a := generateWithRules(t, "家計簿。毎週日曜の夜に今週の合計を記録して知らせて", expenseApp, weeklyRules)
	if !strings.Contains(string(res.DSL), `"with":{"total"`) {
		t.Fatalf("wire with must become a map: %s", res.DSL)
	}
	recs := listMap{"expenses": {
		{ID: "e1", Data: map[string]any{"date": "2026-11-02", "amount": 1200.0}},
		{ID: "e2", Data: map[string]any{"date": "2026-11-01", "amount": 999.0}}, // last week (Sunday)
	}}
	p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], engine.Trigger{AppID: "a", Kind: engine.KindScheduled, At: simAt}, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].Values["total"] != 1200.0 || len(p.Notifies) != 1 || p.Notifies[0].Body != "今週は 1200 円でした" {
		t.Fatalf("plan: %+v", p)
	}
}

func TestWithFromWire(t *testing.T) {
	bad := []map[string]any{{"do": []any{map[string]any{"notify": map[string]any{"with": []any{
		map[string]any{"name": "a", "value": 1.0}, map[string]any{"name": "a", "value": 2.0}}}}}}}
	if _, err := serverRulesFromWire(bad); err == nil || !strings.Contains(err.Error(), "written twice") {
		t.Fatalf("duplicate name: %v", err)
	}
	empty := []map[string]any{{"do": []any{map[string]any{"notify": map[string]any{"with": []any{}}}}}}
	out, err := serverRulesFromWire(empty)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out[0]["do"].([]any)[0].(map[string]any)["notify"].(map[string]any)["with"]; ok {
		t.Fatal("empty with must be dropped")
	}
}
