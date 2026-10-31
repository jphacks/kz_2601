package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMemoryPurgeAccount(t *testing.T) {
	testPurgeAccount(t, NewMemory(), "")
}

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestorePurgeAccount(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sfx := time.Now().Format("150405.000000")
	testPurgeAccount(t, f, sfx)

	refs, err := f.userRef("leaver" + sfx).Collection("apps").DocumentRefs(ctx).GetAll()
	if err != nil || len(refs) != 0 {
		t.Fatalf("users/{uid}/apps must be empty: %d %v", len(refs), err)
	}
	snap, err := f.userRef("leaver" + sfx).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Data(); got["status"] != AccountPurged || got["deletedAt"] != nil {
		t.Fatalf("tombstone: %v", got)
	}
}

// leaver owns SOLO (nobody else) and GROUP (stayer joined and posted), and
// joined stayer's THEIRS. After the purge only the shared parts stay.
func testPurgeAccount(t *testing.T, s Store, sfx string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	leaver, stayer := "leaver"+sfx, "stayer"+sfx
	solo, group, theirs := "SOLO"+sfx, "GROUP"+sfx, "THEIRS"+sfx
	initial, _ := json.Marshal(dsl.Initial())
	for _, a := range []struct{ uid, app string }{{leaver, solo}, {leaver, group}, {leaver, theirs}, {stayer, group}, {stayer, theirs}} {
		_, _, err := s.EnsureApp(ctx, a.uid, a.app, initial, "x")
		must(err)
	}
	for _, o := range []struct{ uid, app string }{{leaver, solo}, {leaver, group}, {stayer, theirs}} {
		_, err := s.ClaimApp(ctx, o.uid, o.app)
		must(err)
		ok, err := s.PublishApp(ctx, o.uid, o.app, initial, "x", 1)
		must(err)
		if !ok {
			t.Fatalf("publish %s", o.app)
		}
	}
	_, _, err := s.CreateRecord(ctx, PrivateRecords(leaver, solo, "memo"), Record{ID: "priv", Data: map[string]any{"v": 1}})
	must(err)
	post := func(app, id, uid, name string) {
		t.Helper()
		_, _, err := s.CreateRecord(ctx, PublicRecords(app, "post"), Record{ID: id, Entity: "post", Author: uid, AuthorName: name, Data: map[string]any{}})
		must(err)
	}
	post(solo, "p-solo", leaver, "Leaver")
	post(group, "p-group-l", leaver, "Leaver")
	post(group, "p-group-s", stayer, "Stayer")
	post(theirs, "p-theirs-l", leaver, "Leaver")
	post(theirs, "p-theirs-s", stayer, "Stayer")
	_, err = s.AddMember(ctx, group, Member{UID: stayer, DisplayName: "Stayer", Via: MemberViaShare})
	must(err)
	_, err = s.AddMember(ctx, theirs, Member{UID: leaver, DisplayName: "Leaver", Via: MemberViaShare})
	must(err)
	_, err = s.SetMark(ctx, PublicRecords(theirs, "post"), "p-theirs-s", "like", leaver, true)
	must(err)
	_, err = s.SetMark(ctx, PublicRecords(group, "post"), "p-group-s", "like", leaver, true)
	must(err)
	_, err = s.SetMark(ctx, PublicRecords(group, "post"), "p-group-s", "like", stayer, true)
	must(err)
	must(s.CreateShare(ctx, Share{Token: "tok-l" + sfx, AppID: solo, OwnerUID: leaver, DSL: initial}))
	must(s.CreateShare(ctx, Share{Token: "tok-s" + sfx, AppID: theirs, OwnerUID: stayer, DSL: initial}))
	_, err = s.PutCatalogApp(ctx, leaver, CatalogApp{ID: solo, DSL: initial})
	must(err)
	_, err = s.PutCatalogApp(ctx, stayer, CatalogApp{ID: theirs, DSL: initial})
	must(err)
	_, err = s.RecordInstall(ctx, solo, stayer, "copy-s"+sfx, true)
	must(err)
	_, err = s.RecordInstall(ctx, theirs, leaver, "copy-l"+sfx, true)
	must(err)
	_, _, err = s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: solo, OwnerUID: leaver, Contact: "x@example.com"})
	must(err)
	_, _, err = s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: theirs, OwnerUID: stayer, Contact: "y@example.com"})
	must(err)

	if _, err := s.PurgeAccount(ctx, leaver); !errors.Is(err, ErrNotDeleted) {
		t.Fatalf("active account must not be purged: %v", err)
	}
	must(s.DeactivateUser(ctx, leaver))

	has := func(list []DeletedAccount, uid string) bool {
		for _, d := range list {
			if d.UID == uid {
				return true
			}
		}
		return false
	}
	if l, err := s.DeletedAccounts(ctx, time.Now().Add(-time.Hour), 0); err != nil || has(l, leaver) {
		t.Fatalf("not old enough yet: %v %v", l, err)
	}
	l, err := s.DeletedAccounts(ctx, time.Now().Add(time.Hour), 0)
	if err != nil || !has(l, leaver) || has(l, stayer) {
		t.Fatalf("candidates: %v %v", l, err)
	}

	res, err := s.PurgeAccount(ctx, leaver)
	must(err)
	want := AccountPurgeResult{Apps: 3, KeptMirrors: 1, Shares: 1, Releases: 1, Owners: 1, Installs: 1,
		Memberships: 1, Marks: 2, PostsScrubbed: 2}
	if res != want {
		t.Errorf("result:\n got %+v\nwant %+v", res, want)
	}
	again, err := s.PurgeAccount(ctx, leaver)
	must(err)
	if again != (AccountPurgeResult{}) {
		t.Errorf("second run must find nothing: %+v", again)
	}

	// erased
	for _, id := range []string{solo, group, theirs} {
		if _, err := s.GetApp(ctx, leaver, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("leaver app %s: %v", id, err)
		}
	}
	if _, err := s.GetPublicApp(ctx, solo); !errors.Is(err, ErrNotFound) {
		t.Errorf("solo mirror: %v", err)
	}
	if _, err := s.GetRecord(ctx, PublicRecords(solo, "post"), "p-solo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("solo post: %v", err)
	}
	if _, err := s.GetShare(ctx, "tok-l"+sfx); !errors.Is(err, ErrNotFound) {
		t.Errorf("share: %v", err)
	}
	if _, err := s.GetInstall(ctx, solo, stayer); !errors.Is(err, ErrNotFound) {
		t.Errorf("install of the taken-down listing: %v", err)
	}
	if _, err := s.GetInstall(ctx, theirs, leaver); !errors.Is(err, ErrNotFound) {
		t.Errorf("leaver install: %v", err)
	}
	if _, err := s.AppOwner(ctx, solo); !errors.Is(err, ErrNotFound) {
		t.Errorf("solo owner: %v", err)
	}
	if rr, err := s.ListReleaseRequests(ctx, leaver, ""); err != nil || len(rr) != 0 {
		t.Errorf("releases: %v %v", rr, err)
	}
	if _, err := s.GetMember(ctx, theirs, leaver); !errors.Is(err, ErrNotFound) {
		t.Errorf("membership: %v", err)
	}
	marked, err := s.MarkedBy(ctx, PublicRecords(theirs, "post"), []string{"p-theirs-s"}, []string{"like"}, leaver)
	if err != nil || len(marked) != 0 {
		t.Errorf("leaver mark: %v %v", marked, err)
	}
	if r, err := s.GetRecord(ctx, PublicRecords(theirs, "post"), "p-theirs-s"); err != nil || r.MarkCounts["like"] != 0 {
		t.Errorf("like count: %+v %v", r, err)
	}
	if r, err := s.GetRecord(ctx, PublicRecords(group, "post"), "p-group-s"); err != nil || r.MarkCounts["like"] != 1 {
		t.Errorf("group like count: %+v %v", r, err)
	}

	// kept (shared with other people)
	for _, p := range []struct{ app, id string }{{group, "p-group-l"}, {theirs, "p-theirs-l"}} {
		r, err := s.GetRecord(ctx, PublicRecords(p.app, "post"), p.id)
		if err != nil || r.Author != leaver || r.AuthorName != "" {
			t.Errorf("leaver post %s kept without name: %+v %v", p.id, r, err)
		}
	}
	for _, p := range []struct{ app, id string }{{group, "p-group-s"}, {theirs, "p-theirs-s"}} {
		if r, err := s.GetRecord(ctx, PublicRecords(p.app, "post"), p.id); err != nil || r.AuthorName != "Stayer" {
			t.Errorf("stayer post %s: %+v %v", p.id, r, err)
		}
	}
	if _, err := s.GetPublicApp(ctx, group); err != nil {
		t.Errorf("group mirror stays for its members: %v", err)
	}
	if o, err := s.AppOwner(ctx, group); err != nil || o != leaver {
		t.Errorf("group owner stays with the mirror: %q %v", o, err)
	}
	if _, err := s.GetMember(ctx, group, stayer); err != nil {
		t.Errorf("stayer membership: %v", err)
	}
	for _, id := range []string{group, theirs} {
		if _, err := s.GetApp(ctx, stayer, id); err != nil {
			t.Errorf("stayer app %s: %v", id, err)
		}
	}
	if _, err := s.GetShare(ctx, "tok-s"+sfx); err != nil {
		t.Errorf("stayer share: %v", err)
	}
	if _, err := s.GetCatalogApp(ctx, theirs); err != nil {
		t.Errorf("stayer listing: %v", err)
	}
	if rr, err := s.ListReleaseRequests(ctx, stayer, ""); err != nil || len(rr) != 1 {
		t.Errorf("stayer releases: %v %v", rr, err)
	}

	// tombstone
	must(s.MarkAccountPurged(ctx, leaver))
	must(s.MarkAccountPurged(ctx, leaver))
	must(s.DeactivateUser(ctx, leaver)) // a late DELETE /v1/me does not re-arm it
	if gone, err := s.UserDeleted(ctx, leaver); err != nil || !gone {
		t.Errorf("purged uid stays deleted: %v %v", gone, err)
	}
	if l, err := s.DeletedAccounts(ctx, time.Now().Add(time.Hour), 0); err != nil || has(l, leaver) {
		t.Errorf("purged uid is not a candidate: %v %v", l, err)
	}
	if err := s.MarkAccountPurged(ctx, stayer); !errors.Is(err, ErrNotDeleted) {
		t.Errorf("active uid: %v", err)
	}
}
