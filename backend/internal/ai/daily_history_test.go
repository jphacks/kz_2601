package ai

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #172 eval (no API): 「服薬チェック。毎日飲んだらチェックして連続日数も見たい」.
// The canonical medicine example (daily toggle + computed streak / last7)
// passes generation validation as is, and a streak on a date field that is
// not a daily toggle is sent back for regeneration.
const medicineStreakTranscript = "服薬チェック。毎日飲んだらチェックして連続日数も見たい"

func medicineExample(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/dsl/examples/medicine.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCreate_MedicineStreak(t *testing.T) {
	ex := medicineExample(t)
	f := &fakeCaller{replies: []string{string(ex)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: medicineStreakTranscript})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.Attempts != 1 {
		t.Fatalf("the example should pass first time: %+v", res.Meta)
	}
	app, err := dsl.Parse(res.DSL)
	if err != nil {
		t.Fatal(err)
	}
	var streak, last7 bool
	for _, fld := range app.Entities[0].Fields {
		if fld.Computed == nil {
			continue
		}
		b, _ := json.Marshal(fld.Computed)
		streak = streak || strings.Contains(string(b), `"op":"streak"`)
		last7 = last7 || strings.Contains(string(b), `"op":"doneInDays"`)
	}
	if !streak || !last7 {
		t.Fatalf("computed streak/doneInDays lost after post-processing: %s", res.DSL)
	}
	if fields := dsl.DailyToggleFields(app); !fields["medicine"]["taken_on"] {
		t.Fatalf("daily toggle lost: %v", fields)
	}
	if f := app.Entities[0].FieldByID("taken_on"); f == nil || f.DefaultValue != nil {
		t.Fatalf("daily toggle date must not get a defaultValue: %+v", f)
	}
}

func TestCreate_StreakOnNonDailyFieldRegenerates(t *testing.T) {
	ex := medicineExample(t)
	bad := strings.Replace(string(ex), `"computed": { "op": "streak", "args": [{ "ref": "taken_on" }] }`,
		`"computed": { "op": "streak", "args": [{ "ref": "started" }] }`, 1)
	bad = strings.Replace(bad, `{ "id": "taken_on", "name": "飲んだ日", "type": "date" },`,
		`{ "id": "taken_on", "name": "飲んだ日", "type": "date" }, { "id": "started", "name": "開始日", "type": "date" },`, 1)
	if bad == string(ex) {
		t.Fatal("fixture edit did not apply")
	}
	f := &fakeCaller{replies: []string{bad, string(ex)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: medicineStreakTranscript})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.Attempts != 2 {
		t.Fatalf("meta: %+v", res.Meta)
	}
	if !strings.Contains(f.calls[1].Messages[0].Content, `"started" is not a daily check`) {
		t.Fatalf("second prompt should carry the problem: %s", f.calls[1].Messages[0].Content)
	}
}

func TestSystemPromptTeachesStreak(t *testing.T) {
	for _, s := range []string{`{"op":"streak","args":[{"ref":"<その field>"}]}`, `{"op":"doneInDays","args":[{"ref":"<その field>"},7]}`} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("system prompt lacks %s", s)
		}
	}
	for _, s := range []string{"連続日数は今日の達成数で代替", "連続日数 → 直近7日の記録数"} {
		if strings.Contains(systemPrompt, s) {
			t.Errorf("system prompt still says %q (streak is supported since #172)", s)
		}
	}
}
