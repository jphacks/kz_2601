package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// dmDSL (#271): group messages (members) and 1 対 1 notes (member), both
// read only by the sender and the recipients; read is a mark.
const dmDSL = `{"schemaVersion":1,"app":{"name":"連絡","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"message","name":"メッセージ","scope":"shared","addressedTo":"to","marks":[{"id":"read","icon":"eye","label":"既読"}],"fields":[
  {"id":"to","name":"宛先","type":"members","required":true},
  {"id":"body","name":"本文","type":"string"}]},
 {"id":"note","name":"ひとこと","scope":"shared","addressedTo":"to","fields":[
  {"id":"to","name":"宛先","type":"member"},
  {"id":"body","name":"本文","type":"string"}]}],
"screens":[{"id":"home","title":"受信箱","components":[
 {"type":"list","entity":"message","filter":{"field":"$author","op":"neq","value":{"me":true}},"item":{"title":"body","mark":"read"}},
 {"type":"list","entity":"note","item":{"title":"body"}}]}]}`

// joinDM: alice makes the app; bob and carol join; stranger does not.
func joinDM(t *testing.T, e *env, app string) {
	t.Helper()
	e.ownApp("alice", app, []byte(dmDSL))
	tok := e.share("alice", app)
	for _, u := range []string{"bob", "carol"} {
		if r := e.do(u, http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"`+u+`"}`, nil); r.code != http.StatusCreated {
			t.Fatalf("%s import: %d %s", u, r.code, r.raw)
		}
	}
}

func listIDs(t *testing.T, e *env, uid, path string) []string {
	t.Helper()
	r := e.do(uid, http.MethodGet, path, nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("%s list %s: %d %s", uid, path, r.code, r.raw)
	}
	var ids []string
	for _, it := range r.body["items"].([]any) {
		ids = append(ids, it.(map[string]any)["id"].(string))
	}
	return ids
}

func post(t *testing.T, e *env, uid, path, body string) string {
	t.Helper()
	r := e.do(uid, http.MethodPost, path, body, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("%s post %s: %d %s", uid, body, r.code, r.raw)
	}
	return r.body["id"].(string)
}

// #271 完了の条件: 宛先付きの記録は送った人と宛先の人だけが読め、ほかの人には一覧・取得・変更・印・通報のどこからも見えない。
func TestAddressedRecords(t *testing.T) { ownerStores(t, testAddressedRecords) }

func testAddressedRecords(t *testing.T, e *env, sfx string) {
	app := "/v1/apps/DM-READ" + sfx
	msgs := app + "/entities/message/records"
	joinDM(t, e, app)

	toBob := post(t, e, "alice", msgs, `{"data":{"to":["bob"],"body":"bob だけ"}}`)
	group := post(t, e, "bob", msgs, `{"data":{"to":["alice","carol","carol"],"body":"みんな"}}`)
	bobToCarol := post(t, e, "bob", msgs, `{"data":{"to":["carol"],"body":"carol だけ"}}`)

	// the recipients must be participants, and there must be one
	for body, what := range map[string]string{
		`{"data":{"to":["stranger"],"body":"x"}}`: "a non-participant",
		`{"data":{"to":[],"body":"x"}}`:           "no recipient",
		`{"data":{"body":"x"}}`:                   "missing recipients",
		`{"data":{"to":"bob","body":"x"}}`:        "a string for members",
	} {
		if r := e.do("alice", http.MethodPost, msgs, body, nil); r.code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: want 422, got %d %s", what, r.code, r.raw)
		}
	}
	if r := e.do("alice", http.MethodPost, app+"/entities/note/records", `{"data":{"to":"stranger"}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("member note to a non-participant: %d %s", r.code, r.raw)
	}

	// list: the author and the recipients only (the app's creator is not special)
	want := map[string][]string{"alice": {toBob, group}, "bob": {toBob, group, bobToCarol}, "carol": {group, bobToCarol}}
	for uid, ids := range want {
		got := listIDs(t, e, uid, msgs)
		if len(got) != len(ids) {
			t.Fatalf("%s sees %v, want %v", uid, got, ids)
		}
		for _, id := range ids {
			if !strings.Contains(strings.Join(got, ","), id) {
				t.Fatalf("%s misses %s: %v", uid, id, got)
			}
		}
	}
	wantNotMember(t, e.do("stranger", http.MethodGet, msgs, nil, nil), "stranger lists")

	// every other path: 404 as if the record did not exist
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, msgs + "/" + toBob, ""},
		{http.MethodPatch, msgs + "/" + toBob, `{"data":{"body":"x"}}`},
		{http.MethodDelete, msgs + "/" + toBob, ""},
		{http.MethodPut, msgs + "/" + toBob + "/marks/read", ""},
		{http.MethodDelete, msgs + "/" + toBob + "/marks/read", ""},
		{http.MethodPost, "/v1/reports", `{"kind":"record","appId":"DM-READ` + sfx + `","entityId":"message","recordId":"` + toBob + `","reason":"harassment"}`},
	} {
		var body any
		if c.body != "" {
			body = c.body
		}
		if r := e.do("carol", c.method, c.path, body, nil); r.code != http.StatusNotFound || strings.Contains(string(r.raw), "bob だけ") {
			t.Fatalf("carol %s %s: want 404, got %d %s", c.method, c.path, r.code, r.raw)
		}
	}

	// the recipient reads it and marks it read (unread = not marked by me)
	r := e.do("bob", http.MethodGet, msgs+"/"+toBob, nil, nil)
	if r.code != http.StatusOK || r.body["data"].(map[string]any)["body"] != "bob だけ" {
		t.Fatalf("bob gets: %d %s", r.code, r.raw)
	}
	if mine := r.body["marks"].(map[string]any)["read"].(map[string]any)["mine"]; mine != false {
		t.Fatalf("unread first: %s", r.raw)
	}
	if r = e.do("bob", http.MethodPut, msgs+"/"+toBob+"/marks/read", nil, nil); r.code != http.StatusOK || r.body["mine"] != true {
		t.Fatalf("bob marks read: %d %s", r.code, r.raw)
	}
	// a recipient does not edit; the author edits the text but not the recipients
	if r = e.do("bob", http.MethodPatch, msgs+"/"+toBob, `{"data":{"body":"x"}}`, nil); r.code != http.StatusForbidden {
		t.Fatalf("recipient edits: %d %s", r.code, r.raw)
	}
	if r = e.do("alice", http.MethodPatch, msgs+"/"+toBob, `{"data":{"to":["bob","carol"]}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("change recipients: %d %s", r.code, r.raw)
	}
	if r = e.do("alice", http.MethodPatch, msgs+"/"+toBob, `{"data":{"to":["bob"],"body":"直した"}}`, nil); r.code != http.StatusOK {
		t.Fatalf("author edits: %d %s", r.code, r.raw)
	}

	// clearing "all" as a non-admin removes only one's own
	if r = e.do("carol", http.MethodDelete, msgs+"?confirm=true", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("carol clears: %d %s", r.code, r.raw)
	}
	if got := listIDs(t, e, "bob", msgs); len(got) != 3 {
		t.Fatalf("carol's clear touched others: %v", got)
	}

	// a later DSL without addressedTo does not open the messages sent before
	e.do("alice", http.MethodPut, app+"/dsl", putBody([]byte(strings.Replace(dmDSL, `"addressedTo":"to",`, "", 1))), map[string]string{"If-Match": `"2"`})
	if r = e.do("alice", http.MethodGet, app+"/dsl", nil, nil); strings.Contains(string(r.raw), `"addressedTo":"to","marks"`) {
		t.Fatalf("dsl not replaced: %s", r.raw)
	}
	if got := listIDs(t, e, "carol", msgs); len(got) != 2 {
		t.Fatalf("after the DSL change carol sees %v", got)
	}
	if r = e.do("carol", http.MethodGet, msgs+"/"+toBob, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("after the DSL change carol gets: %d %s", r.code, r.raw)
	}
}

// The events stream tells a participant only about records they can read.
func TestAddressedEvents(t *testing.T) {
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		srv := httptest.NewServer(e.h)
		defer srv.Close()
		appID := "DM-EVENTS" + sfx
		msgs := "/v1/apps/" + appID + "/entities/message/records"
		joinDM(t, e, "/v1/apps/"+appID)

		events, stop := openEvents(t, srv, "carol", appID)
		defer stop()
		if hello := next(t, events); hello.name != "hello" {
			t.Fatalf("hello: %+v", hello)
		}
		post(t, e, "alice", msgs, `{"data":{"to":["bob"],"body":"秘密"}}`)
		select {
		case ev := <-events:
			t.Fatalf("carol was told about a message to bob: %+v", ev)
		case <-time.After(700 * time.Millisecond):
		}
		post(t, e, "alice", msgs, `{"data":{"to":["carol"],"body":"carol へ"}}`)
		if ev := next(t, events); ev.name != "records" || ev.data["entity"] != "message" {
			t.Fatalf("carol's message: %+v", ev)
		}
	})
}

// Readers are stored when a record is created, from the author and the recipients.
func TestAddressedReadersStored(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		app := "/v1/apps/DM-STORED" + sfx
		joinDM(t, e, app)
		id := post(t, e, "bob", app+"/entities/note/records", `{"data":{"to":"carol","body":"x"}}`)
		rec, err := st.GetRecord(t.Context(), store.PublicRecords("DM-STORED"+sfx, "note"), id)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(rec.Readers, ",") != "bob,carol" {
			t.Fatalf("readers: %v", rec.Readers)
		}
	})
}
