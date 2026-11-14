package runner_test

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// pick with count (shift assignment, TK_2306 / ng_2412): a new shift goes
// to the staff member with the fewest shifts who is not off that day. The
// three entities go through the capped, counted reader; the write is
// counted like any rule write. A shift saved with a staff member is left
// alone (if).
func TestAssignShiftCreated(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		id := "RD-ASSIGN" + h.sfx
		owner, m1 := "uid-rd-assign-o"+h.sfx, "uid-rd-assign-a"+h.sfx
		app := h.circle(id, owner, readsDSL(t, "server_rules_assign.json"), m1)
		s1 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"田中"}}`)
		s2 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"鈴木"}}`)
		s3 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"佐藤"}}`)
		h.post(owner, app+"/entities/shifts/records", `{"data":{"day":"2026-11-09","staff":"`+s1+`"}}`)
		h.post(m1, app+"/entities/offs/records", `{"data":{"staff":"`+s3+`","day":"2026-11-10"}}`)
		h.tick()
		shift := h.post(m1, app+"/entities/shifts/records", `{"data":{"day":"2026-11-10"}}`)
		h.tick()
		var got any
		for _, r := range h.records(store.PublicRecords(id, "shifts")) {
			if r.ID == shift {
				got = r.Data["staff"]
			}
		}
		if got != s2 {
			t.Fatalf("staff %v, want %s (s1 has a shift, s3 is off)", got, s2)
		}
		out := h.logs.String()
		if !strings.Contains(out, `"reads":6`) || !strings.Contains(out, `"writes":1`) {
			t.Fatalf("usage logs (staff + shifts + offs read, one write): %s", out)
		}
	})
}

// Shifts saved one after another and run in the same tick go to different
// staff: a runner takes the events in order and finishes one (its writes
// included) before planning the next, so each counts the earlier ones.
func TestAssignShiftsCreatedInTurn(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		id := "RD-ASSIGN-SEQ" + h.sfx
		owner := "uid-rd-assign-seq" + h.sfx
		app := h.circle(id, owner, readsDSL(t, "server_rules_assign.json"))
		s1 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"田中"}}`)
		s2 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"鈴木"}}`)
		s3 := h.post(owner, app+"/entities/staff/records", `{"data":{"name":"佐藤"}}`)
		h.tick()
		var shifts []string
		for _, day := range []string{"2026-11-10", "2026-11-11", "2026-11-12"} {
			shifts = append(shifts, h.post(owner, app+"/entities/shifts/records", `{"data":{"day":"`+day+`"}}`))
		}
		h.tick()
		got := map[string]any{}
		for _, r := range h.records(store.PublicRecords(id, "shifts")) {
			got[r.ID] = r.Data["staff"]
		}
		seen := map[any]bool{}
		for _, s := range shifts {
			seen[got[s]] = true
		}
		if len(seen) != 3 || !seen[s1] || !seen[s2] || !seen[s3] {
			t.Fatalf("staff %v, want one shift each for %s %s %s", got, s1, s2, s3)
		}
	})
}
