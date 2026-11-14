package runner_test

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// A lookup (tk_2413 roster reconciliation) runs through the same capped,
// counted reader as agg: a registration not on the roster notifies the
// owner, one on it does not, and the roster records are counted as reads.
func TestReadsLookupRoster(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RD-ROSTER"+h.sfx, "uid-rd-roster"+h.sfx
		h.tokens[uid] = []string{"tok-roster"}
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, readsDSL(t, "server_rules_lookup.json"), "Asia/Tokyo")
		h.tick()
		h.post(uid, app+"/entities/roster/records", `{"data":{"student_id":"A001","name":"青木"}}`)
		h.post(uid, app+"/entities/roster/records", `{"data":{"student_id":"A002","name":"井上"}}`)
		h.post(uid, app+"/entities/registrations/records", `{"data":{"student_id":"ａ００１","course":"数学"}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 0 {
			t.Fatalf("on the roster: %d pushes", n)
		}
		h.post(uid, app+"/entities/registrations/records", `{"data":{"student_id":"B999","course":"数学"}}`)
		h.tick()
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Body != "B999 は名簿にありません" {
			t.Fatalf("pushes: %+v", pushes)
		}
		if out := h.logs.String(); !strings.Contains(out, `"reads":2`) {
			t.Fatalf("usage logs (the roster is counted as reads): %s", out)
		}
	})
}

// The daily each run marks only the registrations missing from the roster.
func TestReadsLookupEach(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RD-ROSTER-EACH"+h.sfx, "uid-rd-roster-each"+h.sfx
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, readsDSL(t, "server_rules_lookup.json"), "Asia/Tokyo")
		h.tick()
		row := h.scheduleOf(id, "roster_check")
		h.post(uid, app+"/entities/roster/records", `{"data":{"student_id":"A001"}}`)
		h.post(uid, app+"/entities/registrations/records", `{"data":{"student_id":"A001"}}`)
		h.post(uid, app+"/entities/registrations/records", `{"data":{"student_id":"C3"}}`)
		h.tick()
		h.tickAt(row.NextRunAt)
		var marked []any
		for _, r := range h.records(store.PrivateRecords(uid, id, "registrations")) {
			if r.Data["mismatch"] == true {
				marked = append(marked, r.Data["student_id"])
			}
		}
		if len(marked) != 1 || marked[0] != "C3" {
			t.Fatalf("marked %v", marked)
		}
	})
}
