package expreval

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// SP_2305-style adaptive amount: each week's target is the base amount
// scaled by last week's completion rate (80% keeps the base), clamped to
// 0.5x..1.5x, using only computed agg + date filters + arithmetic.
func TestAdaptiveStudyTargetFromLastWeek(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "dsl", "examples", "adaptive_study.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w := app.Lint(); len(w) > 0 {
		t.Errorf("lint warnings: %v", w)
	}
	week := func(id, start string, base float64) Record {
		return Record{ID: id, Data: map[string]any{"start": start, "base": base}}
	}
	weeks := []Record{
		week("w1", "2026-11-02", 300), // last week 10-26..11-01: 1/4 done
		week("w2", "2026-11-09", 300), // 11-02..11-08: 4/5 done
		week("w3", "2026-11-16", 300), // 11-09..11-15: 4/4 done
		week("w4", "2026-11-23", 200), // 11-16..11-22: 2/3 done
		week("w5", "2026-11-30", 300), // 11-23..11-29: no records
	}
	n := 0
	log := func(date string, done bool) Record {
		n++
		return Record{ID: fmt.Sprintf("l%d", n), Data: map[string]any{"date": date, "task": "勉強", "minutes": 30.0, "done": done}}
	}
	logs := []Record{
		log("2026-10-26", true), log("2026-10-28", false), log("2026-10-30", false), log("2026-11-01", false),
		log("2026-11-02", true), log("2026-11-03", true), log("2026-11-05", true), log("2026-11-07", false), log("2026-11-08", true),
		log("2026-11-09", true), log("2026-11-10", true), log("2026-11-12", true), log("2026-11-15", true),
		log("2026-11-16", true), log("2026-11-18", false), log("2026-11-22", true),
	}
	src := SourceFunc(func(e string) []Record {
		switch e {
		case "weeks":
			return weeks
		case "logs":
			return logs
		}
		return nil
	})
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, tokyo) // Wednesday of the 11-09 week
	ev := New(Env{Entities: app.Entities, Source: src, Now: now, Location: tokyo})

	var weekEntity *dsl.Entity
	for i := range app.Entities {
		if app.Entities[i].ID == "weeks" {
			weekEntity = &app.Entities[i]
		}
	}
	if weekEntity == nil {
		t.Fatal("entity weeks missing")
	}
	// total, done, rate %, target minutes
	want := map[string][4]float64{
		"w1": {4, 1, 25, 150},  // 300*25/80 = 93.75 -> clamped to 150
		"w2": {5, 4, 80, 300},  // 80% keeps the base
		"w3": {4, 4, 100, 375}, // 300*100/80
		"w4": {3, 2, 67, 168},  // round(66.7) = 67; 200*67/80 = 167.5 -> 168
		"w5": {0, 0, 80, 300},  // no history: keep the base
	}
	for _, w := range weeks {
		row := &Row{Entity: weekEntity, Record: w}
		exp := want[w.ID]
		for i, f := range []string{"last_total", "last_done", "last_rate", "target"} {
			if got := ev.FieldValue(f, row); got != NumberValue(exp[i]) {
				t.Errorf("%s %s = %+v, want %v", w.ID, f, got, exp[i])
			}
		}
	}

	// stat.value: this week's (11-09..11-15) completion rate.
	var stat *dsl.Expr
	for _, s := range app.Screens {
		for _, c := range s.Components {
			if c.ID == "rate_now" {
				stat = c.Value
			}
		}
	}
	if stat == nil {
		t.Fatal("stat rate_now missing")
	}
	if got := ev.Eval(*stat, nil); got != NumberValue(100) {
		t.Errorf("rate_now = %+v, want 100", got)
	}
}
