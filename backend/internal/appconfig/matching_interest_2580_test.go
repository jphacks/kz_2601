package appconfig

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// interest2580Store fails the store calls of the interest endpoints when set.
type interest2580Store struct {
	store.Store
	ageAmong, blockedEither, listBlocks, quota, changePair, listPairs error
}

func (s *interest2580Store) AgeConfirmedAmong(ctx context.Context, uids []string) (map[string]bool, error) {
	if s.ageAmong != nil {
		return nil, s.ageAmong
	}
	return s.Store.AgeConfirmedAmong(ctx, uids)
}

func (s *interest2580Store) BlockedEither(ctx context.Context, uid string, others []string) (map[string]bool, error) {
	if s.blockedEither != nil {
		return nil, s.blockedEither
	}
	return s.Store.BlockedEither(ctx, uid, others)
}

func (s *interest2580Store) ListBlocks(ctx context.Context, uid string) ([]store.Block, error) {
	if s.listBlocks != nil {
		return nil, s.listBlocks
	}
	return s.Store.ListBlocks(ctx, uid)
}

func (s *interest2580Store) TakeMatchQuota(ctx context.Context, uid, day string, limit int) error {
	if s.quota != nil {
		return s.quota
	}
	return s.Store.TakeMatchQuota(ctx, uid, day, limit)
}

func (s *interest2580Store) ChangeMatchPair(ctx context.Context, appID, a, b string, fn func(cur *store.MatchPair) (*store.MatchPair, error)) (*store.MatchPair, error) {
	if s.changePair != nil {
		return nil, s.changePair
	}
	return s.Store.ChangeMatchPair(ctx, appID, a, b, fn)
}

func (s *interest2580Store) ListMatchPairs(ctx context.Context, appID, uid string) ([]store.MatchPair, error) {
	if s.listPairs != nil {
		return nil, s.listPairs
	}
	return s.Store.ListMatchPairs(ctx, appID, uid)
}

func newInterest2580Env(t *testing.T, app string) (matchEnv, *interest2580Store) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &interest2580Store{Store: store.NewMemory()}
	m := matchEnv{env: &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, app: app}
	m.ownApp("owner", app, matchingDSL(t))
	return m, st
}

func want2580(t *testing.T, what string, r resp, code int, errCode string) {
	t.Helper()
	if r.code != code || r.errCode() != errCode {
		t.Fatalf("%s: want %d %s, got %d %s", what, code, errCode, r.code, r.raw)
	}
}

// #2580: bad requests to sendInterest and listInterests are refused.
func TestInterest2580BadRequests(t *testing.T) {
	m, _ := newInterest2580Env(t, "/v1/apps/I2580-BAD")
	m.adult("alice", "bob")
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")

	want2580(t, "unknown field", m.do("alice", http.MethodPost, m.app+"/interests", `{"bogus":1}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2580(t, "not json", m.do("alice", http.MethodPost, m.app+"/interests", `{`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2580(t, "no recordId", m.do("alice", http.MethodPost, m.app+"/interests", `{"fromRecordId":"`+pa+`"}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2580(t, "bad fromRecordId", m.do("alice", http.MethodPost, m.app+"/interests", `{"recordId":"`+pb+`","fromRecordId":"a b"}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2580(t, "bad box", m.do("alice", http.MethodGet, m.app+"/interests?box=all", nil, nil), http.StatusBadRequest, httpx.CodeBadRequest)

	// no declaration and ageConfirmed omitted
	r := m.do("carol", http.MethodPost, m.app+"/interests", `{"recordId":"`+pa+`","fromRecordId":"`+pb+`"}`, nil)
	if r.code != http.StatusBadRequest || r.reason() != reasonAgeUnconfirmed {
		t.Fatalf("no age: %d %s", r.code, r.raw)
	}

	// an app without matching, and an invalid appId
	plain := "/v1/apps/I2580-PLAIN"
	m.ownApp("owner", plain, sharedKakeibo(t))
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, plain + "/interests", `{"recordId":"` + pb + `","fromRecordId":"` + pa + `","ageConfirmed":true}`},
		{http.MethodGet, plain + "/interests", ""},
	} {
		var body any
		if c.body != "" {
			body = c.body
		}
		if r := m.do("alice", c.method, c.path, body, nil); r.code != http.StatusNotFound || r.reason() != reasonNoMatching {
			t.Fatalf("%s %s: %d %s", c.method, c.path, r.code, r.raw)
		}
	}
	want2580(t, "invalid appId", m.do("alice", http.MethodGet, "/v1/apps/undefined/interests", nil, nil), http.StatusBadRequest, httpx.CodeBadRequest)
}

// #2580: sending to oneself, to a deleted profile, or from someone else's profile.
func TestInterest2580TargetChecks(t *testing.T) {
	m, _ := newInterest2580Env(t, "/v1/apps/I2580-TARGET")
	m.adult("alice", "bob")
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")

	if r := m.send("alice", pa, pa); r.code != http.StatusBadRequest || !strings.Contains(string(r.raw), "yourself") {
		t.Fatalf("to oneself: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", pb, pb); r.code != http.StatusBadRequest || r.body["error"].(map[string]any)["details"].(map[string]any)["field"] != "fromRecordId" {
		t.Fatalf("from someone else's profile: %d %s", r.code, r.raw)
	}
	if r := m.do("bob", http.MethodDelete, m.app+"/entities/profile/records/"+pb, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("bob deletes his profile: %d %s", r.code, r.raw)
	}
	want2580(t, "deleted profile", m.send("alice", pb, pa), http.StatusNotFound, httpx.CodeNotFound)
	if got := uids(m.items("bob", "/interests")); got != "" {
		t.Fatalf("nothing reached bob: %q", got)
	}
}

// #2580: someone who blocked the caller looks like they got it, but never does.
func TestInterest2580BlockedByThem(t *testing.T) {
	m, _ := newInterest2580Env(t, "/v1/apps/I2580-BLK")
	m.adult("alice", "bob")
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")
	if r := m.do("bob", http.MethodPost, "/v1/me/blocks", `{"uid":"alice"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("block: %d %s", r.code, r.raw)
	}
	r := m.send("alice", pb, pa)
	if r.code != http.StatusOK || r.body["uid"] != "bob" {
		t.Fatalf("send to someone who blocked: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("bob", "/interests")); got != "" {
		t.Fatalf("bob got it: %q", got)
	}
	// accept / decline name a blocked person: 404, and oneself: 400
	want2580(t, "accept blocked", m.do("alice", http.MethodPost, m.app+"/interests/bob/accept", `{"ageConfirmed":true}`, nil), http.StatusNotFound, httpx.CodeNotFound)
	want2580(t, "accept self", m.do("alice", http.MethodPost, m.app+"/interests/alice/accept", `{"ageConfirmed":true}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2580(t, "accept bad uid", m.do("alice", http.MethodPost, m.app+"/interests/a%20b/accept", `{"ageConfirmed":true}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
}

// #2580: after the caller declined them, or they withdrew, sending starts afresh.
func TestInterest2580SendAgain(t *testing.T) {
	m, _ := newInterest2580Env(t, "/v1/apps/I2580-AGAIN")
	m.adult("alice", "bob", "carol")
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")
	pc := m.post("carol", "profile", "hobby", "チェス")

	// carol -> alice, alice declines, then alice sends to carol
	m.send("carol", pa, pc)
	if r := m.do("alice", http.MethodDelete, m.app+"/interests/carol", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("decline: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", pc, pa); r.code != http.StatusOK || r.body["status"] != "pending" {
		t.Fatalf("alice changes her mind: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("carol", "/interests")); got != "alice" {
		t.Fatalf("carol incoming %q", got)
	}

	// bob -> alice, bob withdraws, then alice sends to bob
	m.send("bob", pa, pb)
	if r := m.do("bob", http.MethodDelete, m.app+"/interests/alice", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("withdraw: %d %s", r.code, r.raw)
	}
	if r := m.send("alice", pb, pa); r.code != http.StatusOK || r.body["status"] != "pending" {
		t.Fatalf("after bob withdrew: %d %s", r.code, r.raw)
	}
	if got := uids(m.items("bob", "/interests")); got != "alice" {
		t.Fatalf("bob incoming %q", got)
	}
}

// #2580: store failures are 500 and change nothing.
func TestInterest2580StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	m, st := newInterest2580Env(t, "/v1/apps/I2580-ERR")
	appID := strings.TrimPrefix(m.app, "/v1/apps/")
	m.adult("alice", "bob")
	pa := m.post("alice", "profile", "hobby", "囲碁")
	pb := m.post("bob", "profile", "hobby", "将棋")
	internal := func(what string, set *error, r func() resp) {
		t.Helper()
		*set = boom
		got := r()
		*set = nil
		want2580(t, what, got, http.StatusInternalServerError, httpx.CodeInternal)
	}
	send := func() resp { return m.send("alice", pb, pa) }

	internal("BlockedEither", &st.blockedEither, send)
	internal("TakeMatchQuota", &st.quota, send)
	internal("ChangeMatchPair", &st.changePair, send)
	internal("ListMatchPairs", &st.listPairs, func() resp { return m.do("alice", http.MethodGet, m.app+"/interests?box=outgoing", nil, nil) })
	internal("otherOf BlockedEither", &st.blockedEither, func() resp {
		return m.do("bob", http.MethodPost, m.app+"/interests/alice/accept", `{"ageConfirmed":true}`, nil)
	})

	// ListBlocks is read only when someone in the pair blocked the other
	if r := m.do("bob", http.MethodPost, "/v1/me/blocks", `{"uid":"alice"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("block: %d %s", r.code, r.raw)
	}
	internal("ListBlocks", &st.listBlocks, send)
	m.do("bob", http.MethodDelete, "/v1/me/blocks/alice", nil, nil)

	// AgeConfirmedAmong is read only for a profile without the flag
	legacy := "rec_L2580"
	if _, _, err := st.CreateRecord(t.Context(), store.PublicRecords(appID, "profile"), store.Record{ID: legacy, Data: map[string]any{"hobby": "碁"}, Author: "bob", AuthorName: "bob"}); err != nil {
		t.Fatal(err)
	}
	internal("AgeConfirmedAmong", &st.ageAmong, func() resp { return m.send("alice", legacy, pa) })

	if got := uids(m.items("bob", "/interests")); got != "" {
		t.Fatalf("a failed send reached bob: %q", got)
	}
}
