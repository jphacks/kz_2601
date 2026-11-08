package expreval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// D_2210: each candidate day counts the answers that point at it with a
// given status, using the existing computed agg (row ref + literal filter).
func TestSchedulePollCountsPerDay(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "dsl", "examples", "schedule_poll.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	days := []Record{
		{ID: "d1", Data: map[string]any{"date": "2026-11-20"}},
		{ID: "d2", Data: map[string]any{"date": "2026-11-21"}},
		{ID: "d3", Data: map[string]any{"date": "2026-11-22"}},
	}
	answer := func(id, day, name, status string) Record {
		return Record{ID: id, Data: map[string]any{"day": day, "name": name, "status": status}}
	}
	answers := []Record{
		answer("a1", "d1", "山田", "参加できる"),
		answer("a2", "d1", "佐藤", "参加できる"),
		answer("a3", "d1", "鈴木", "未定"),
		answer("a4", "d2", "山田", "参加できない"),
		answer("a5", "d2", "佐藤", "参加できる"),
		answer("a6", "d2", "鈴木", "未定"),
		answer("a7", "d2", "田中", "未定"),
	}
	src := SourceFunc(func(e string) []Record {
		switch e {
		case "days":
			return days
		case "answers":
			return answers
		}
		return nil
	})
	ev := New(Env{Entities: app.Entities, Source: src})
	var dayEntity *dsl.Entity
	for i := range app.Entities {
		if app.Entities[i].ID == "days" {
			dayEntity = &app.Entities[i]
		}
	}
	if dayEntity == nil {
		t.Fatal("entity days missing")
	}
	want := map[string][2]float64{"d1": {2, 1}, "d2": {1, 2}, "d3": {0, 0}}
	for _, d := range days {
		row := &Row{Entity: dayEntity, Record: d}
		w := want[d.ID]
		if got := ev.FieldValue("ok_count", row); got != NumberValue(w[0]) {
			t.Errorf("%s ok_count = %+v, want %v", d.ID, got, w[0])
		}
		if got := ev.FieldValue("maybe_count", row); got != NumberValue(w[1]) {
			t.Errorf("%s maybe_count = %+v, want %v", d.ID, got, w[1])
		}
	}
}
