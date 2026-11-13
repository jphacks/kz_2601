package appconfig

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// memberReads counts member documents read: one per GetMember, one per
// member a ListMembers returns (what Firestore bills).
type memberReads struct {
	*store.Memory
	gets, lists, listed int
}

func (m *memberReads) GetMember(ctx context.Context, appID, uid string) (*store.Member, error) {
	m.gets++
	return m.Memory.GetMember(ctx, appID, uid)
}

func (m *memberReads) ListMembers(ctx context.Context, appID string, limit int) ([]store.Member, error) {
	ms, err := m.Memory.ListMembers(ctx, appID, limit)
	m.lists++
	m.listed += len(ms)
	return ms, err
}

func (m *memberReads) reset() { m.gets, m.lists, m.listed = 0, 0, 0 }

// #2297: checking who a record is addressed to reads members/{uid} for each
// listed uid, not the member list; the outcome (owner / member / outsider) is
// the same as before.
func TestAddressedChecksReadOneMember(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &memberReads{Memory: store.NewMemory()}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	app := "/v1/apps/DM-READS"
	msgs := app + "/entities/message/records"
	joinDM(t, e, app)
	for i := range 60 { // a big club: the old check read all of them
		if _, err := st.AddMember(t.Context(), "DM-READS", store.Member{UID: fmt.Sprintf("p%02d", i), Via: store.MemberViaShare}); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		name, by, body string
		code, gets     int
	}{
		{"owner → member", "alice", `{"data":{"to":["bob"],"body":"x"}}`, http.StatusCreated, 1},
		{"member → owner", "bob", `{"data":{"to":["alice"],"body":"x"}}`, http.StatusCreated, 1}, // bob's own membership only
		{"owner → members, a repeat read once", "alice", `{"data":{"to":["bob","carol","bob"],"body":"x"}}`, http.StatusCreated, 2},
		{"member → owner and member", "bob", `{"data":{"to":["alice","p07"],"body":"x"}}`, http.StatusCreated, 2},
		{"outsider recipient", "alice", `{"data":{"to":["bob","stranger"],"body":"x"}}`, http.StatusUnprocessableEntity, 2},
		{"not a uid", "alice", `{"data":{"to":["a/b"],"body":"x"}}`, http.StatusUnprocessableEntity, 0},
		{"outsider writer", "stranger", `{"data":{"to":["alice"],"body":"x"}}`, http.StatusForbidden, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			st.reset()
			r := e.do(c.by, http.MethodPost, msgs, c.body, nil)
			if r.code != c.code {
				t.Fatalf("code %d, want %d: %s", r.code, c.code, r.raw)
			}
			if st.lists != 0 || st.gets != c.gets {
				t.Errorf("reads: %d lists (%d members), %d gets; want 0 lists, %d gets", st.lists, st.listed, st.gets, c.gets)
			}
		})
	}
}
