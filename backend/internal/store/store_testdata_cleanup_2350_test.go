package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

// The stg test-data cleanup (cmd/stg-cleanup, #293) against the emulator:
// only smoke apps / listed test users / stg counters are found and erased,
// and the read-only finders leave everything in place (#2350).

// cleanupFixture is one run's data: a smoke user and an ordinary user side
// by side, with ids unique to the run (the emulator is shared).
type cleanupFixture struct {
	f                   *Firestore
	sfx                 string
	smokeUID, realUID   string
	smokeApp, smokeWalk string // users/{smokeUID}/apps: smoke ids
	keepApp             string // users/{smokeUID}/apps: not a smoke id
	realApp, lookalike  string // users/{realUID}/apps
	stgIDs, otherIDs    []string
	all                 []*firestore.DocumentRef
}

func newCleanupFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	f := emulatorStore(t)
	sfx := runSuffix()
	n := time.Now().UnixNano() % 1e10
	x := &cleanupFixture{
		f: f, sfx: sfx,
		smokeUID: "stg-smoke" + sfx, realUID: "real-user" + sfx,
		smokeApp: fmt.Sprintf("smoke-%010d", n), smokeWalk: fmt.Sprintf("smoke-%010d-walk", n),
		keepApp: "notes" + sfx, realApp: "diary" + sfx, lookalike: "smoke-diary" + sfx,
		stgIDs:   []string{"stg_2026-11-13_uid_" + "u" + sfx[1:], "stg_2026-11-13_ip_0a1b2c" + fmt.Sprintf("%x", n)},
		otherIDs: []string{"prod_2026-11-13_uid_u" + sfx[1:], "stg_notadate_uid_u" + sfx[1:]},
	}
	put := func(ref *firestore.DocumentRef, data map[string]any) {
		t.Helper()
		if _, err := ref.Set(t.Context(), data); err != nil {
			t.Fatal(err)
		}
		x.all = append(x.all, ref)
	}
	app := func(uid, id string) {
		a := f.appRef(uid, id)
		put(a, map[string]any{"name": id})
		put(a.Collection("versions").Doc("v1"), map[string]any{"n": 1})
		put(a.Collection("records").Doc("r1"), map[string]any{"n": 1})
	}
	put(f.userRef(x.smokeUID), map[string]any{"createdAt": time.Now()})
	put(f.userRef(x.realUID), map[string]any{"createdAt": time.Now()})
	for _, id := range []string{x.smokeApp, x.smokeWalk, x.keepApp} {
		app(x.smokeUID, id)
	}
	for _, id := range []string{x.realApp, x.lookalike} {
		app(x.realUID, id)
	}
	put(f.ownerRef(x.smokeApp), map[string]any{"uid": x.smokeUID})
	put(f.ownerRef(x.realApp), map[string]any{"uid": x.realUID})
	put(f.c.Collection(feedbackCollection).Doc("fb-smoke"+sfx), map[string]any{"uid": x.smokeUID})
	put(f.c.Collection(feedbackCollection).Doc("fb-real"+sfx), map[string]any{"uid": x.realUID})
	put(f.c.Collection(sharesCollection).Doc("sh-smoke"+sfx), map[string]any{"ownerUid": x.smokeUID, "appId": x.keepApp})
	put(f.c.Collection(sharesCollection).Doc("sh-real"+sfx), map[string]any{"ownerUid": x.realUID, "appId": x.realApp})
	for _, id := range append(slices.Clone(x.stgIDs), x.otherIDs...) {
		put(f.c.Collection(rateLimitsCollection).Doc(id), map[string]any{"count": 1})
	}
	t.Cleanup(func() { _ = f.bulkDelete(context.Background(), x.all) })
	return x
}

// missing lists the paths of refs that are gone.
func (x *cleanupFixture) missing(t *testing.T, refs []*firestore.DocumentRef) []string {
	t.Helper()
	var gone []string
	for _, r := range refs {
		if _, err := r.Get(t.Context()); isNotFound(err) {
			gone = append(gone, r.Path)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	return gone
}

// realRefs are the ordinary user's documents, which nothing may touch.
func (x *cleanupFixture) realRefs() []*firestore.DocumentRef {
	var out []*firestore.DocumentRef
	for _, r := range x.all {
		if strings.Contains(r.Path, x.realUID) || strings.HasSuffix(r.Path, "fb-real"+x.sfx) || strings.HasSuffix(r.Path, "sh-real"+x.sfx) {
			out = append(out, r)
		}
	}
	for _, id := range x.otherIDs {
		out = append(out, x.f.c.Collection(rateLimitsCollection).Doc(id))
	}
	return out
}

func TestFindSmokeDataOnlySmoke_2350(t *testing.T) {
	x := newCleanupFixture(t)
	got, err := x.f.FindSmokeData(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]SmokeApp{}
	for _, a := range got.Apps {
		if a.UID == x.smokeUID || a.UID == x.realUID {
			found[a.AppID] = a
		}
	}
	if len(found) != 2 {
		t.Fatalf("found %v, want %s and %s only", found, x.smokeApp, x.smokeWalk)
	}
	for _, id := range []string{x.smokeApp, x.smokeWalk} {
		a, ok := found[id]
		if !ok || a.UID != x.smokeUID || a.Docs["apps"] != 1 || a.Docs["versions"] != 1 || a.Docs["records"] != 1 {
			t.Fatalf("%s: %+v", id, a)
		}
	}
	if !slices.Contains(got.Owners, x.smokeApp) || slices.Contains(got.Owners, x.realApp) || slices.Contains(got.Owners, x.lookalike) {
		t.Fatalf("owners %v", got.Owners)
	}
	// read only: a dry run leaves every document
	if gone := x.missing(t, x.all); len(gone) != 0 {
		t.Fatalf("dry run removed %v", gone)
	}
}

func TestPurgeSmokeAppOnlyThatApp_2350(t *testing.T) {
	x := newCleanupFixture(t)
	ctx := t.Context()
	for _, id := range []string{x.keepApp, x.lookalike, x.realApp} {
		if _, err := x.f.PurgeSmokeApp(ctx, x.realUID, id); !errors.Is(err, ErrNotSmoke) {
			t.Fatalf("%s: err %v, want ErrNotSmoke", id, err)
		}
	}
	if err := x.f.DeleteSmokeOwner(ctx, x.realApp); !errors.Is(err, ErrNotSmoke) {
		t.Fatalf("owner of %s: %v", x.realApp, err)
	}
	res, err := x.f.PurgeSmokeApp(ctx, x.smokeUID, x.smokeApp)
	if err != nil || res.Versions != 1 || res.Records != 1 {
		t.Fatalf("purge %+v %v", res, err)
	}
	if err := x.f.DeleteSmokeOwner(ctx, x.smokeApp); err != nil {
		t.Fatal(err)
	}
	app := x.f.appRef(x.smokeUID, x.smokeApp)
	gone := x.missing(t, x.all)
	want := []string{app.Path, app.Collection("versions").Doc("v1").Path, app.Collection("records").Doc("r1").Path, x.f.ownerRef(x.smokeApp).Path}
	slices.Sort(gone)
	slices.Sort(want)
	if !slices.Equal(gone, want) {
		t.Fatalf("removed %v, want %v", gone, want)
	}
	// a retry is not an error
	if _, err := x.f.PurgeSmokeApp(ctx, x.smokeUID, x.smokeApp); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestSummarizeTestUserCountsOnlyThatUser_2350(t *testing.T) {
	x := newCleanupFixture(t)
	sum, err := x.f.SummarizeTestUser(t.Context(), x.smokeUID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"apps": 3, "versions": 3, "records": 3, feedbackCollection: 1, sharesCollection: 1}
	for k, n := range want {
		if sum.Docs[k] != n {
			t.Fatalf("%s = %d, want %d (%v)", k, sum.Docs[k], n, sum.Docs)
		}
	}
	if !sum.Exists || sum.Status != "" || sum.Docs[ownersCollection] != 1 {
		t.Fatalf("summary %+v", sum)
	}
	if gone := x.missing(t, x.all); len(gone) != 0 {
		t.Fatalf("summary removed %v", gone)
	}
}

func TestDeleteUserTreeKeepsOthers_2350(t *testing.T) {
	x := newCleanupFixture(t)
	n, err := x.f.DeleteUserTree(t.Context(), x.smokeUID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 9 { // 3 apps × (app + version + record)
		t.Fatalf("deleted %d, want 9", n)
	}
	if _, err := x.f.userRef(x.smokeUID).Get(t.Context()); !isNotFound(err) {
		t.Fatalf("users/%s still there: %v", x.smokeUID, err)
	}
	if gone := x.missing(t, x.realRefs()); len(gone) != 0 {
		t.Fatalf("ordinary user's data removed: %v", gone)
	}
}

func TestFindStgRateLimitsOnlyStg_2350(t *testing.T) {
	x := newCleanupFixture(t)
	ids, err := x.f.FindStgRateLimits(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range x.stgIDs {
		if !slices.Contains(ids, id) {
			t.Fatalf("%s not found in %v", id, ids)
		}
	}
	for _, id := range x.otherIDs {
		if slices.Contains(ids, id) {
			t.Fatalf("%s found", id)
		}
	}
	if gone := x.missing(t, x.all); len(gone) != 0 {
		t.Fatalf("find removed %v", gone)
	}
	if err := x.f.DeleteStgRateLimits(t.Context(), append(slices.Clone(x.stgIDs), x.otherIDs[0])); err == nil {
		t.Fatal("a non-stg counter was accepted")
	}
	if gone := x.missing(t, x.all); len(gone) != 0 {
		t.Fatalf("a refused delete removed %v", gone)
	}
	if err := x.f.DeleteStgRateLimits(t.Context(), x.stgIDs); err != nil {
		t.Fatal(err)
	}
	if gone := x.missing(t, x.realRefs()); len(gone) != 0 {
		t.Fatalf("non-stg counters removed: %v", gone)
	}
}
