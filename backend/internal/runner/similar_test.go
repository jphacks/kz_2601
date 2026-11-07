package runner_test

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// similar (os_2403 scouting / F_2201 recommendation) runs through the same
// capped, counted reader as agg: a new circle notifies the author of the
// profile whose hobbies match best (with the usual consent of members), and
// the profiles are counted as reads.
func TestReadsSimilarScout(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		id := "RD-SIMILAR" + h.sfx
		owner, m1, m2 := "uid-rd-sim-o"+h.sfx, "uid-rd-sim-a"+h.sfx, "uid-rd-sim-b"+h.sfx
		h.tokens[owner], h.tokens[m1] = []string{"tok-sim-o"}, []string{"tok-sim-a"}
		app := h.circle(id, owner, readsDSL(t, "server_rules_similar.json"), m1, m2)
		h.tick()
		h.post(owner, app+"/entities/profiles/records", `{"data":{"name":"O","hobbies":"旅行、釣り、映画"}}`)
		h.post(m1, app+"/entities/profiles/records", `{"data":{"name":"A","hobbies":"映画、音楽"}}`)
		h.tick()
		h.post(m2, app+"/entities/circles/records", `{"data":{"name":"外遊び","hobbies":"旅行・映画"}}`)
		h.tick()
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Body != "外遊び があなたの趣味と 2 個合います" || pushes[0].Token != "tok-sim-o" {
			t.Fatalf("pushes: %+v", pushes)
		}
		if out := h.logs.String(); !strings.Contains(out, `"reads":2`) {
			t.Fatalf("usage logs (the profiles are counted as reads): %s", out)
		}
		// A member's profile matches best: no push without that member's consent.
		h.post(m2, app+"/entities/circles/records", `{"data":{"name":"鑑賞会","hobbies":"映画、音楽"}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 1 {
			t.Fatalf("pushes to a member without consent: %d", n)
		}
		if out := h.logs.String(); !strings.Contains(out, `"no_consent":1`) {
			t.Fatalf("logs: %s", out)
		}
	})
}

// The daily each run writes each profile's best match (the owner's and the
// members' records alike, never one's own).
func TestReadsSimilarEach(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id := "RD-SIMILAR-EACH" + h.sfx
		owner, m1 := "uid-rd-sime-o"+h.sfx, "uid-rd-sime-a"+h.sfx
		app := h.circle(id, owner, readsDSL(t, "server_rules_similar.json"), m1)
		h.tick()
		row := h.scheduleOf(id, "daily_match")
		h.post(owner, app+"/entities/profiles/records", `{"data":{"name":"O","hobbies":"音楽"}}`)
		h.post(owner, app+"/entities/profiles/records", `{"data":{"name":"O2","hobbies":"音楽、映画"}}`)
		h.post(m1, app+"/entities/profiles/records", `{"data":{"name":"A","hobbies":"映画"}}`)
		h.tick()
		h.tickAt(row.NextRunAt)
		best := map[string]any{}
		for _, r := range h.records(store.PublicRecords(id, "profiles")) {
			best[r.Data["name"].(string)] = r.Data["best_name"]
		}
		if best["O"] != nil || best["O2"] != "A" || best["A"] != "O2" {
			t.Fatalf("best %v", best)
		}
	})
}
