package runner_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #340 reads: agg in a condition / with, on.each, read caps.

func readsDSL(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../pkg/dsl/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var tokyo = schedule.Location("Asia/Tokyo")

// scheduleOf returns appID's schedule row of rule.
func (h *harness) scheduleOf(appID, rule string) store.Schedule {
	h.t.Helper()
	for _, s := range h.schedules(appID) {
		if s.RuleID == rule {
			return s
		}
	}
	h.t.Fatalf("no schedule of %s/%s", appID, rule)
	return store.Schedule{}
}

// A monthly total over the budget notifies the owner with the total; the
// first expense (under the budget) does not.
func TestReadsAggCondition(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RD-BUDGET"+h.sfx, "uid-rd-budget"+h.sfx
		h.tokens[uid] = []string{"tok-budget"}
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, readsDSL(t, "server_rules_reads.json"), "Asia/Tokyo")
		h.tick()
		today := time.Now().In(tokyo).Format("2006-01-02")
		h.post(uid, app+"/entities/expenses/records", `{"data":{"memo":"家賃","date":"`+today+`","amount":30000}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 0 {
			t.Fatalf("under the budget: %d pushes", n)
		}
		h.post(uid, app+"/entities/expenses/records", `{"data":{"memo":"旅行","date":"`+today+`","amount":30000}}`)
		h.tick()
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Body != "今月の支出が 60000 円になりました" {
			t.Fatalf("pushes: %+v", pushes)
		}
		out := h.logs.String()
		if !strings.Contains(out, `"reads":2`) || strings.Contains(out, "60000") {
			t.Fatalf("usage logs (reads counted, no values): %s", out)
		}
	})
}

// The app's daily reads cap stops the run (skipped_quota, nothing sent).
func TestReadsAppQuota(t *testing.T) {
	stores(t, options{limits: func(l *quota.Limits) { l.AppReadsPro, l.AppReadsFree = 1, 1 }}, func(t *testing.T, h *harness) {
		id, uid := "RD-QUOTA"+h.sfx, "uid-rd-quota"+h.sfx
		h.tokens[uid] = []string{"tok-quota"}
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, readsDSL(t, "server_rules_reads.json"), "Asia/Tokyo")
		h.tick()
		today := time.Now().In(tokyo).Format("2006-01-02")
		h.post(uid, app+"/entities/expenses/records", `{"data":{"date":"`+today+`","amount":60000}}`)
		h.post(uid, app+"/entities/expenses/records", `{"data":{"date":"`+today+`","amount":60000}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 0 {
			t.Fatalf("over the read quota: %d pushes", n)
		}
		evs := h.events(id)
		if len(evs) != 2 {
			t.Fatalf("events: %+v", evs)
		}
		for _, ev := range evs {
			if ev.RuleOutcomes["over_budget"] != quota.OutcomeSkippedQuota {
				t.Fatalf("outcome: %+v", ev)
			}
		}
		if !strings.Contains(h.logs.String(), `"limit":"app_reads"`) {
			t.Fatalf("logs: %s", h.logs.String())
		}
	})
}

// on.each: the daily rule notifies once per task due in 3 days (in the
// creator's zone); the same tick again sends nothing.
func TestReadsEachDueSoon(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RD-DUE"+h.sfx, "uid-rd-due"+h.sfx
		h.tokens[uid] = []string{"tok-due"}
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, readsDSL(t, "server_rules_reads.json"), "Asia/Tokyo")
		h.tick()
		row := h.scheduleOf(id, "due_soon")
		day := row.NextRunAt.In(tokyo)
		in3 := day.AddDate(0, 0, 3).Format("2006-01-02")
		h.post(uid, app+"/entities/tasks/records", `{"data":{"title":"レポート","due":"`+in3+`"}}`)
		h.post(uid, app+"/entities/tasks/records", `{"data":{"title":"済んだ","due":"`+in3+`","done":true}}`)
		h.post(uid, app+"/entities/tasks/records", `{"data":{"title":"まだ先","due":"`+day.AddDate(0, 0, 9).Format("2006-01-02")+`"}}`)
		h.tickAt(row.NextRunAt)
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Body != "レポート の締め切りまであと 3 日です" {
			t.Fatalf("pushes: %+v", pushes)
		}
		if res := h.tickAt(row.NextRunAt); res.Claimed != 0 {
			t.Fatalf("second tick: %+v", res)
		}
		if n := len(h.sender.Pushes()); n != 1 {
			t.Fatalf("pushes after second tick: %d", n)
		}
	})
}

// Ranking on shared data: each player gets its rank; the run cap stops a
// run that would read more records than allowed (no rank written).
func TestReadsEachRanking(t *testing.T) {
	run := func(t *testing.T, h *harness, id string) (store.RecordSet, store.Schedule) {
		owner, m1 := "uid-rd-rank-o"+h.sfx, "uid-rd-rank-m"+h.sfx
		app := h.circle(id, owner, readsDSL(t, "server_rules_ranking.json"), m1)
		h.tick()
		h.post(owner, app+"/entities/players/records", `{"data":{"name":"A","score":100}}`)
		h.post(m1, app+"/entities/players/records", `{"data":{"name":"B","score":300}}`)
		h.post(m1, app+"/entities/players/records", `{"data":{"name":"C","score":200}}`)
		row := h.scheduleOf(id, "ranking")
		h.tickAt(row.NextRunAt)
		return store.PublicRecords(id, "players"), row
	}
	stores(t, options{}, func(t *testing.T, h *harness) {
		rs, _ := run(t, h, "RD-RANK"+h.sfx)
		ranks := map[string]float64{}
		for _, r := range h.records(rs) {
			ranks[r.Data["name"].(string)] = num(r.Data["rank"])
		}
		if ranks["A"] != 3 || ranks["B"] != 1 || ranks["C"] != 2 {
			t.Fatalf("ranks: %v", ranks)
		}
	})
	stores(t, options{limits: func(l *quota.Limits) { l.RunReadsPro = 2 }}, func(t *testing.T, h *harness) {
		id := "RD-RANKCAP" + h.sfx
		rs, _ := run(t, h, id)
		for _, r := range h.records(rs) {
			if _, ok := r.Data["rank"]; ok {
				t.Fatalf("rank written over the run cap: %+v", r)
			}
		}
		evs := h.scheduleEvents(id)
		if len(evs) != 1 || evs[0].RuleOutcomes["ranking"] != quota.OutcomeSkippedQuota {
			t.Fatalf("schedule events: %+v", evs)
		}
		if !strings.Contains(h.logs.String(), `"limit":"run_reads"`) {
			t.Fatalf("logs: %s", h.logs.String())
		}
	})
}
