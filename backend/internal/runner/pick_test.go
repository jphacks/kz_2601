package runner_test

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// pick (D_2212 / ng_2413 recommendation) runs through the same capped,
// counted reader as agg: the daily run logs a main dish nobody ate yet and
// tells the members its name (members with their consent); both entities
// are counted as reads. A second tick of the same run sends nothing again.
func TestReadsPickDaily(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		id := "RD-PICK" + h.sfx
		owner, m1 := "uid-rd-pick-o"+h.sfx, "uid-rd-pick-a"+h.sfx
		h.tokens[owner], h.tokens[m1] = []string{"tok-pick-o"}, []string{"tok-pick-a"}
		app := h.circle(id, owner, readsDSL(t, "server_rules_pick.json"), m1)
		h.consent(m1, id)
		h.tick()
		row := h.scheduleOf(id, "daily_pick")
		eaten := h.post(owner, app+"/entities/menus/records", `{"data":{"name":"唐揚げ","category":"main"}}`)
		h.post(owner, app+"/entities/menus/records", `{"data":{"name":"カレー","category":"main"}}`)
		h.post(m1, app+"/entities/menus/records", `{"data":{"name":"サラダ","category":"side"}}`)
		h.post(m1, app+"/entities/eats/records", `{"data":{"menu":"`+eaten+`"}}`)
		h.tick()
		before := len(h.sender.Pushes())
		h.tickAt(row.NextRunAt)
		picks := h.records(store.PublicRecords(id, "picks"))
		if len(picks) != 1 || picks[0].Data["name"] != "カレー" {
			t.Fatalf("picks %+v", picks)
		}
		pushes := h.sender.Pushes()[before:]
		if len(pushes) != 2 {
			t.Fatalf("pushes %+v", pushes)
		}
		for _, p := range pushes {
			if p.Body != "まだ食べていない カレー はどうですか" {
				t.Fatalf("push %+v", p)
			}
		}
		if out := h.logs.String(); !strings.Contains(out, `"reads":4`) {
			t.Fatalf("usage logs (menus + eats are counted as reads): %s", out)
		}
		h.tick()
		if n := len(h.sender.Pushes()) - before; n != 2 || len(h.records(store.PublicRecords(id, "picks"))) != 1 {
			t.Fatalf("sent again: %d pushes", n)
		}
	})
}
