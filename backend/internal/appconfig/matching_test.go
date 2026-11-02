package appconfig

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func matchingDSL(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../pkg/dsl/testdata/matching.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type matchEnv struct {
	*env
	app string
}

func (m matchEnv) post(uid, entity, field, value string) string {
	m.t.Helper()
	r := m.do(uid, http.MethodPost, m.app+"/entities/"+entity+"/records", `{"data":{"`+field+`":"`+value+`"}}`, nil)
	if r.code != http.StatusCreated {
		m.t.Fatalf("%s posts %s: %d %s", uid, entity, r.code, r.raw)
	}
	return r.body["id"].(string)
}

func (m matchEnv) items(uid, path string) []any {
	m.t.Helper()
	r := m.do(uid, http.MethodGet, m.app+path, nil, nil)
	if r.code != http.StatusOK {
		m.t.Fatalf("%s GET %s: %d %s", uid, path, r.code, r.raw)
	}
	items, _ := r.body["items"].([]any)
	return items
}

// uids lists items[].uid (interests / matches) or items[].author (records).
func uids(items []any) string {
	var out []string
	for _, it := range items {
		m := it.(map[string]any)
		if u, ok := m["uid"].(string); ok {
			out = append(out, u)
		} else {
			out = append(out, m["author"].(string))
		}
	}
	return strings.Join(out, ",")
}

func (m matchEnv) send(from, toProfile, fromProfile string) resp {
	m.t.Helper()
	return m.do(from, http.MethodPost, m.app+"/interests", `{"recordId":"`+toProfile+`","fromRecordId":"`+fromProfile+`","ageConfirmed":true}`, nil)
}

// #507 完了の条件: お互いに OK した時だけつながり、つながった相手だけが連絡先を見られる。
func TestMatching(t *testing.T) { ownerStores(t, testMatching) }

func testMatching(t *testing.T, e *env, sfx string) {
	m := matchEnv{env: e, app: "/v1/apps/MATCH-1" + sfx}
	e.ownApp("owner", m.app, matchingDSL(t))
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")
	pc := m.post("carol", "profile", "hobby", "チェス")
	ca := m.post("alice", "contact", "how", "alice@example.com")
	m.post("bob", "contact", "how", "bob@example.com")

	// before any match: only one's own contact
	if got := uids(m.items("bob", "/entities/contact/records")); got != "bob" {
		t.Fatalf("bob sees contacts of %q", got)
	}
	if r := m.do("bob", http.MethodGet, m.app+"/entities/contact/records/"+ca, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("unmatched contact by id: %d %s", r.code, r.raw)
	}

	// guards
	if r := m.do("alice", http.MethodPost, m.app+"/interests", `{"recordId":"`+pb+`","fromRecordId":"`+pa+`"}`, nil); r.code != http.StatusBadRequest || r.reason() != reasonAgeUnconfirmed {
		t.Fatalf("no age confirmation: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", pb, pc); r.code != http.StatusBadRequest {
		t.Fatalf("someone else's profile as fromRecordId: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", pa, pa); r.code != http.StatusBadRequest {
		t.Fatalf("to oneself: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", "rec_NONE", pa); r.code != http.StatusNotFound {
		t.Fatalf("unknown profile: %d %s", r.code, r.raw)
	}

	// alice -> bob, bob accepts
	if r := m.send("alice", pb, pa); r.code != http.StatusOK || r.body["status"] != "pending" || r.body["uid"] != "bob" {
		t.Fatalf("send: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("alice", "/interests?box=outgoing")); got != "bob" {
		t.Fatalf("alice outgoing %q", got)
	}
	in := m.items("bob", "/interests")
	if uids(in) != "alice" || in[0].(map[string]any)["profileRecordId"] != pa {
		t.Fatalf("bob incoming %v", in)
	}
	if got := uids(m.items("bob", "/entities/contact/records")); got != "bob" {
		t.Fatalf("pending is not a match: %q", got)
	}
	if r := m.do("alice", http.MethodPost, m.app+"/interests/bob/accept", `{"ageConfirmed":true}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("the sender cannot accept their own interest: %d %s", r.code, r.raw)
	}
	if r := m.do("bob", http.MethodPost, m.app+"/interests/alice/accept", `{"ageConfirmed":true}`, nil); r.code != http.StatusOK || r.body["status"] != "matched" {
		t.Fatalf("accept: %d %s", r.code, r.raw)
	}
	for _, u := range []string{"alice", "bob"} {
		if got := len(m.items(u, "/matches")); got != 1 {
			t.Fatalf("%s matches: %d", u, got)
		}
	}
	if got := uids(m.items("bob", "/entities/contact/records")); !strings.Contains(got, "alice") {
		t.Fatalf("bob sees alice's contact after the match: %q", got)
	}
	if r := m.do("bob", http.MethodGet, m.app+"/entities/contact/records/"+ca, nil, nil); r.code != http.StatusOK {
		t.Fatalf("matched contact by id: %d %s", r.code, r.raw)
	}
	if r := m.do("carol", http.MethodGet, m.app+"/entities/contact/records/"+ca, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("carol is not matched: %d %s", r.code, r.raw)
	}

	// a decline is never shown to the sender
	if r := m.send("carol", pa, pc); r.code != http.StatusOK {
		t.Fatalf("carol send: %d %s", r.code, r.raw)
	}
	if r := m.do("alice", http.MethodDelete, m.app+"/interests/carol", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("decline: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("alice", "/interests")); got != "" {
		t.Fatalf("alice incoming after decline %q", got)
	}
	if got := uids(m.items("carol", "/interests?box=outgoing")); got != "alice" {
		t.Fatalf("carol still sees pending: %q", got)
	}
	if r := m.send("carol", pa, pc); r.code != http.StatusOK || r.body["status"] != "pending" {
		t.Fatalf("re-sending after a decline stays pending: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("alice", "/interests")); got != "" {
		t.Fatalf("a re-send does not reach alice again: %q", got)
	}

	// mutual send = match
	m.send("carol", pb, pc)
	if r := m.send("bob", pc, pb); r.code != http.StatusOK || r.body["status"] != "matched" {
		t.Fatalf("mutual send: %d %s", r.code, r.raw)
	}

	// block either way hides the profile, the match and the contact
	if r := m.do("alice", http.MethodPost, "/v1/me/blocks", `{"uid":"bob"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("block: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("bob", "/entities/profile/records")); strings.Contains(got, "alice") {
		t.Fatalf("bob still sees alice's profile: %q", got)
	}
	if got := uids(m.items("bob", "/matches")); got != "carol" {
		t.Fatalf("bob matches while blocked: %q", got)
	}
	if got := uids(m.items("bob", "/entities/contact/records")); strings.Contains(got, "alice") {
		t.Fatalf("bob sees alice's contact while blocked: %q", got)
	}
	if r := m.send("alice", pb, pa); r.code != http.StatusBadRequest || r.reason() != reasonBlocked {
		t.Fatalf("send to someone one blocked: %d %s", r.code, r.raw)
	}
	m.do("alice", http.MethodDelete, "/v1/me/blocks/bob", nil, nil)

	// unmatch
	if r := m.do("bob", http.MethodDelete, m.app+"/matches/alice", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("unmatch: %d %s", r.code, r.raw)
	}
	if r := m.do("bob", http.MethodDelete, m.app+"/matches/alice", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("unmatch twice: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("alice", "/entities/contact/records")); strings.Contains(got, "bob") {
		t.Fatalf("contact after unmatch: %q", got)
	}
	if got := uids(m.items("alice", "/matches")); got != "" {
		t.Fatalf("alice matches after unmatch: %q", got)
	}

	// an app without matching
	plain := "/v1/apps/PLAIN-1" + sfx
	e.ownApp("owner", plain, sharedKakeibo(t))
	if r := e.do("alice", http.MethodGet, plain+"/matches", nil, nil); r.code != http.StatusNotFound || r.reason() != reasonNoMatching {
		t.Fatalf("no matching: %d %s", r.code, r.raw)
	}
}

func TestMatchingDailyCap(t *testing.T) {
	e := newEnv(t)
	m := matchEnv{env: e, app: "/v1/apps/MATCH-CAP"}
	e.ownApp("owner", m.app, matchingDSL(t))
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")
	for i := range InterestsPerDay {
		if r := m.send("alice", pb, pa); r.code != http.StatusOK {
			t.Fatalf("send %d: %d %s", i, r.code, r.raw)
		}
	}
	if r := m.send("alice", pb, pa); r.code != http.StatusTooManyRequests {
		t.Fatalf("over the cap: %d %s", r.code, r.raw)
	}
}

// The roles come from the published DSL: a copy edited to drop match does
// not open the matched-only records.
func TestMatchingUsesPublishedDSL(t *testing.T) {
	e := newEnv(t)
	m := matchEnv{env: e, app: "/v1/apps/MATCH-PUB"}
	e.ownApp("owner", m.app, matchingDSL(t))
	m.post("owner", "contact", "how", "owner@example.com")
	edited := strings.Replace(string(matchingDSL(t)), `"match": "matched",`, ``, 1)
	e.do("mallory", http.MethodGet, m.app+"/dsl", nil, nil)
	if r := e.do("mallory", http.MethodPut, m.app+"/dsl", putBody([]byte(edited)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("mallory edits her copy: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("mallory", "/entities/contact/records")); got != "" {
		t.Fatalf("mallory reads contacts of %q", got)
	}
}
