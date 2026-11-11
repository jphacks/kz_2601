package appconfig

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// A profile posted before the 18+ check, by someone who has not declared,
// is shown to nobody else until they declare.
func TestMatchingHidesUndeclaredProfiles(t *testing.T) {
	storeEnvs(t, testMatchingHidesUndeclaredProfiles)
}

func testMatchingHidesUndeclaredProfiles(t *testing.T, e *env, st store.Store, sfx string) {
	m := matchEnv{env: e, app: "/v1/apps/MATCH-HIDE" + sfx}
	appID := strings.TrimPrefix(m.app, "/v1/apps/")
	e.ownApp("owner", m.app, matchingDSL(t))
	alice, dave, frank := "alice"+sfx, "dave"+sfx, "frank"+sfx
	m.adult(alice, frank)
	pa := m.post(alice, "profile", "hobby", "囲碁")
	rs := store.PublicRecords(appID, "profile")
	if rec, err := st.GetRecord(t.Context(), rs, pa); err != nil || !rec.OwnerAgeConfirmed {
		t.Fatalf("a new profile carries the declaration: %+v %v", rec, err)
	}

	// records made before the check: no flag on either
	m.items(dave, "/entities/contact/records") // dave opens the app
	legacy := func(uid, id string) {
		t.Helper()
		if _, _, err := st.CreateRecord(t.Context(), rs, store.Record{ID: id, Data: map[string]any{"hobby": "碁"}, Author: uid, AuthorName: uid}); err != nil {
			t.Fatal(err)
		}
	}
	tag := strings.ReplaceAll(sfx, "-", "")
	pd, pf := "rec_D"+tag, "rec_F"+tag
	legacy(dave, pd)
	legacy(frank, pf)

	got := uids(m.items(alice, "/entities/profile/records"))
	if strings.Contains(got, dave) || !strings.Contains(got, frank) || !strings.Contains(got, alice) {
		t.Fatalf("alice sees %q: dave hidden, frank (declared, no flag) shown", got)
	}
	if r := m.do(alice, http.MethodGet, m.app+"/entities/profile/records/"+pd, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("hidden profile by id: %d %s", r.code, r.raw)
	}
	if r := m.send(alice, pd, pa); r.code != http.StatusNotFound {
		t.Fatalf("interest to a hidden profile: %d %s", r.code, r.raw)
	}
	if got := uids(m.items(dave, "/entities/profile/records")); got != dave {
		t.Fatalf("dave sees only his own: %q", got)
	}

	// dave declares: his profile is marked and shown
	m.adult(dave)
	if rec, err := st.GetRecord(t.Context(), rs, pd); err != nil || !rec.OwnerAgeConfirmed {
		t.Fatalf("declaring marks the profile: %+v %v", rec, err)
	}
	if got := uids(m.items(alice, "/entities/profile/records")); !strings.Contains(got, dave) {
		t.Fatalf("alice sees %q after dave declared", got)
	}
	if r := m.send(alice, pd, pa); r.code != http.StatusOK {
		t.Fatalf("interest after declaring: %d %s", r.code, r.raw)
	}
}
