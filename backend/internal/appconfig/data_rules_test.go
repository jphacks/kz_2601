package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// shiftDSL (#272 #273): shifts only admins confirm or delete, one per slot,
// with a participant as staff; answers are one per person.
const shiftDSL = `{"schemaVersion":1,"app":{"name":"シフト","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"shift","name":"シフト","scope":"shared","deleteBy":"admins","unique":["day","slot"],"fields":[
  {"id":"day","name":"日","type":"date","required":true},
  {"id":"slot","name":"時間","type":"integer","required":true},
  {"id":"staff","name":"担当","type":"member"},
  {"id":"confirmed","name":"確定","type":"boolean","editableBy":"admins"},
  {"id":"memo","name":"メモ","type":"string"}]},
 {"id":"answer","name":"回答","scope":"shared","unique":["$author"],"deleteBy":"authorOrAdmins","fields":[
  {"id":"choice","name":"回答","type":"string"}]}],
"screens":[{"id":"home","title":"シフト","components":[
 {"type":"list","entity":"shift","item":{"title":"memo"}},
 {"type":"list","entity":"answer","item":{"title":"choice"}},
 {"type":"button","label":"確定する","visibleTo":"admins","action":{"type":"navigate","screen":"home"}}]}]}`

// joinShift makes owner's shift app and lets "staff" and "boss" join it.
func joinShift(t *testing.T, e *env, app string) {
	t.Helper()
	e.ownApp("owner", app, []byte(shiftDSL))
	tok := e.share("owner", app)
	for _, u := range []string{"staff", "boss"} {
		if r := e.do(u, http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"`+u+`"}`, nil); r.code != http.StatusCreated {
			t.Fatalf("%s import: %d %s", u, r.code, r.raw)
		}
	}
}

func wantAdminsOnly(t *testing.T, r resp, what string) {
	t.Helper()
	if r.code != http.StatusForbidden || r.reason() != adminsOnlyReason {
		t.Fatalf("%s: want 403 admins_only, got %d %s", what, r.code, r.raw)
	}
}

func wantDuplicate(t *testing.T, r resp, what string) {
	t.Helper()
	if r.code != http.StatusConflict || r.errCode() != "duplicate_record" || r.reason() != duplicateReason {
		t.Fatalf("%s: want 409 duplicate_record, got %d %s", what, r.code, r.raw)
	}
}

// #272 完了の条件: 作った人が最初の管理者・管理者は参加者を管理者にできる・確定と削除は管理者だけ。
func TestDataRoles(t *testing.T) { ownerStores(t, testDataRoles) }

func testDataRoles(t *testing.T, e *env, sfx string) {
	app := "/v1/apps/SHIFT-ROLES" + sfx
	recs := app + "/entities/shift/records"
	joinShift(t, e, app)

	// staff adds a shift but cannot confirm it
	wantAdminsOnly(t, e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9,"confirmed":true}}`, nil), "staff creates confirmed")
	r := e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9,"confirmed":false,"staff":"staff"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("staff create: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	wantAdminsOnly(t, e.do("staff", http.MethodPatch, recs+"/"+id, `{"data":{"confirmed":true}}`, nil), "staff confirms")
	if r = e.do("staff", http.MethodPatch, recs+"/"+id, `{"data":{"memo":"朝","confirmed":false}}`, nil); r.code != http.StatusOK {
		t.Fatalf("staff edits other fields (unchanged admin field): %d %s", r.code, r.raw)
	}

	// member values must be participants
	if r = e.do("staff", http.MethodPatch, recs+"/"+id, `{"data":{"staff":"stranger"}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("member value of a non-participant: %d %s", r.code, r.raw)
	}

	// the creator confirms someone else's record (edit is author) but cannot change its other fields
	if r = e.do("owner", http.MethodPatch, recs+"/"+id, `{"data":{"confirmed":true}}`, nil); r.code != http.StatusOK || r.body["data"].(map[string]any)["confirmed"] != true {
		t.Fatalf("owner confirms: %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodPatch, recs+"/"+id, `{"data":{"memo":"x"}}`, nil); r.code != http.StatusForbidden || r.reason() == adminsOnlyReason {
		t.Fatalf("an admin still cannot edit others' ordinary fields: %d %s", r.code, r.raw)
	}

	// boss is not an admin until the creator says so; staff cannot promote
	wantAdminsOnly(t, e.do("boss", http.MethodPatch, recs+"/"+id, `{"data":{"confirmed":false}}`, nil), "boss before admin")
	wantAdminsOnly(t, e.do("staff", http.MethodPatch, app+"/members/boss", `{"admin":true}`, nil), "staff promotes")
	wantNotMember(t, e.do("stranger", http.MethodPatch, app+"/members/boss", `{"admin":true}`, nil), "stranger promotes")
	if r = e.do("owner", http.MethodPatch, app+"/members/boss", `{"admin":true}`, nil); r.code != http.StatusOK || r.body["admin"] != true {
		t.Fatalf("owner promotes boss: %d %s", r.code, r.raw)
	}
	if r = e.do("boss", http.MethodPatch, recs+"/"+id, `{"data":{"confirmed":false}}`, nil); r.code != http.StatusOK {
		t.Fatalf("admin boss unconfirms: %d %s", r.code, r.raw)
	}
	// an admin promotes others; the creator stays admin
	if r = e.do("boss", http.MethodPatch, app+"/members/staff", `{"admin":true}`, nil); r.code != http.StatusOK {
		t.Fatalf("boss promotes staff: %d %s", r.code, r.raw)
	}
	if r = e.do("boss", http.MethodPatch, app+"/members/owner", `{"admin":false}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("the creator cannot be demoted: %d %s", r.code, r.raw)
	}
	if r = e.do("staff", http.MethodPatch, app+"/members/me", `{"admin":false}`, nil); r.code != http.StatusOK {
		t.Fatalf("staff steps down: %d %s", r.code, r.raw)
	}
	if r = e.do("boss", http.MethodPatch, app+"/members/nobody", `{"admin":true}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("promote a non-member: %d %s", r.code, r.raw)
	}

	// the member list says who is an admin
	r = e.do("boss", http.MethodGet, app+"/members", nil, nil)
	admins := map[string]bool{}
	for _, it := range r.body["items"].([]any) {
		m := it.(map[string]any)
		admins[m["uid"].(string)] = m["admin"] == true
	}
	if r.body["admin"] != true || !admins["owner"] || !admins["boss"] || admins["staff"] {
		t.Fatalf("members admin flags: %s", r.raw)
	}

	// deleteBy admins: even the author cannot delete; an admin can
	wantAdminsOnly(t, e.do("staff", http.MethodDelete, recs+"/"+id, nil, nil), "author deletes on deleteBy admins")
	wantAdminsOnly(t, e.do("staff", http.MethodDelete, recs+"?confirm=true", nil, nil), "non-admin delete all")
	if r = e.do("boss", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("admin deletes: %d %s", r.code, r.raw)
	}

	// authorOrAdmins: the author or an admin, not another member
	ans := app + "/entities/answer/records"
	r = e.do("staff", http.MethodPost, ans, `{"data":{"choice":"出る"}}`, nil)
	aid, _ := r.body["id"].(string)
	if r.code != http.StatusCreated {
		t.Fatalf("staff answers: %d %s", r.code, r.raw)
	}
	if r = e.do("boss", http.MethodPatch, app+"/members/boss", `{"admin":false}`, nil); r.code != http.StatusOK {
		t.Fatalf("boss steps down: %d %s", r.code, r.raw)
	}
	wantAdminsOnly(t, e.do("boss", http.MethodDelete, ans+"/"+aid, nil, nil), "another member deletes")
	if r = e.do("owner", http.MethodDelete, ans+"/"+aid, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("creator deletes on authorOrAdmins: %d %s", r.code, r.raw)
	}
}

// #273 完了の条件: 同じ時間の予約・1 人 2 回目の回答はサーバーが 409 で断る。
func TestDataUnique(t *testing.T) { ownerStores(t, testDataUnique) }

func testDataUnique(t *testing.T, e *env, sfx string) {
	app := "/v1/apps/SHIFT-UNIQUE" + sfx
	recs := app + "/entities/shift/records"
	joinShift(t, e, app)

	r := e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9},"clientId":"c1"}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("first: %d %s", r.code, r.raw)
	}
	first := r.body["id"].(string)
	// the same clientId again is the same record (idempotent), not a duplicate
	if r = e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9},"clientId":"c1"}`, nil); r.code != http.StatusCreated || r.body["id"] != first {
		t.Fatalf("retry: %d %s", r.code, r.raw)
	}
	wantDuplicate(t, e.do("boss", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9.0}}`, nil), "same slot by another member")
	r = e.do("boss", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":10}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("another slot: %d %s", r.code, r.raw)
	}
	second := r.body["id"].(string)
	// moving onto a taken slot is refused; editing other fields keeps the claim
	wantDuplicate(t, e.do("boss", http.MethodPatch, recs+"/"+second, `{"data":{"slot":9}}`, nil), "move onto a taken slot")
	if r = e.do("boss", http.MethodPatch, recs+"/"+second, `{"data":{"memo":"昼"}}`, nil); r.code != http.StatusOK {
		t.Fatalf("edit memo: %d %s", r.code, r.raw)
	}
	// moving frees the old slot
	if r = e.do("boss", http.MethodPatch, recs+"/"+second, `{"data":{"slot":11}}`, nil); r.code != http.StatusOK {
		t.Fatalf("move: %d %s", r.code, r.raw)
	}
	if r = e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":10}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("the freed slot: %d %s", r.code, r.raw)
	}
	// deleting frees it too
	if r = e.do("owner", http.MethodDelete, recs+"/"+first, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if r = e.do("boss", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("slot after delete: %d %s", r.code, r.raw)
	}
	// clearing everything frees every slot
	if r = e.do("owner", http.MethodDelete, recs+"?confirm=true", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete all: %d %s", r.code, r.raw)
	}
	if r = e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":11}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("slot after delete all: %d %s", r.code, r.raw)
	}

	// one answer per person
	ans := app + "/entities/answer/records"
	if r = e.do("staff", http.MethodPost, ans, `{"data":{"choice":"出る"}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("answer: %d %s", r.code, r.raw)
	}
	wantDuplicate(t, e.do("staff", http.MethodPost, ans, `{"data":{"choice":"休む"}}`, nil), "second answer")
	if r = e.do("boss", http.MethodPost, ans, `{"data":{"choice":"休む"}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("another person answers: %d %s", r.code, r.raw)
	}
}

// Records written before the entity had unique (or by a server rule) hold no
// claim; a new record with the same values is still refused.
func TestDataUniqueLegacyRecords(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id := "SHIFT-LEGACY" + sfx
		app := "/v1/apps/" + id
		joinShift(t, e, app)
		legacy := store.Record{ID: "rec_LEGACY" + sfx, Data: map[string]any{"day": "2026-11-07", "slot": int64(9)}, Author: "staff"}
		if _, _, err := st.CreateRecord(t.Context(), store.PublicRecords(id, "shift"), legacy); err != nil {
			t.Fatal(err)
		}
		wantDuplicate(t, e.do("boss", http.MethodPost, app+"/entities/shift/records", `{"data":{"day":"2026-11-07","slot":9}}`, nil), "same values as a legacy record")
		// editing the legacy record without touching its unique values is fine
		if r := e.do("staff", http.MethodPatch, app+"/entities/shift/records/"+legacy.ID, `{"data":{"memo":"昔の"}}`, nil); r.code != http.StatusOK {
			t.Fatalf("edit legacy: %d %s", r.code, r.raw)
		}
	})
}
