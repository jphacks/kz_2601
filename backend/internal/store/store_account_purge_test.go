package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

type feedbackGetter interface {
	GetFeedback(ctx context.Context, id string) (*Feedback, error)
}

// What account_purge_test.go leaves out: a listing put back after DELETE /v1/me
// (in and out of uid's tree), feedback, the report quota, releases of several apps.
func TestStoreAccountPurgeInteractions(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		leaver, stayer := "pi-leaver"+sfx, "pi-stayer"+sfx
		inTree, notTree := "PI-LIST"+sfx, "PI-NOTREE"+sfx
		dsl := json.RawMessage(`{}`)
		_, _, err := s.EnsureApp(ctx, leaver, inTree, dsl, "x")
		must(err)
		for _, app := range []string{inTree, notTree} {
			_, _, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: app, OwnerUID: leaver, Contact: "x@example.com"})
			must(err)
		}
		_, _, err = s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: inTree, OwnerUID: stayer, Contact: "y@example.com"})
		must(err)

		var leaverFB []string
		for _, m := range []string{"m1", "m2"} {
			fb, err := s.CreateFeedback(ctx, Feedback{UID: leaver, Kind: FeedbackBug, Message: m, ConsentVersion: "v1"}, 0)
			must(err)
			leaverFB = append(leaverFB, fb.ID)
		}
		stayerFB, err := s.CreateFeedback(ctx, Feedback{UID: stayer, Kind: FeedbackBug, Message: "s", ConsentVersion: "v1"}, 0)
		must(err)

		day := "2026-11-01"
		rep := func(target string) Report {
			return Report{ID: ReportID(leaver, ReportListing, target), ReporterUID: leaver, Kind: ReportListing,
				TargetID: target, Reason: "spam", Status: "open"}
		}
		if c, err := s.AddReport(ctx, rep("PI-T1"+sfx), day, 1); err != nil || !c {
			t.Fatalf("report: %v %v", c, err)
		}

		must(s.DeactivateUser(ctx, leaver))
		// listings put back after the delete request (a request racing DELETE /v1/me)
		for _, app := range []string{inTree, notTree} {
			_, err := s.PutCatalogApp(ctx, leaver, CatalogApp{ID: app, Name: "x", DSL: dsl})
			must(err)
			_, err = s.RecordInstall(ctx, app, stayer, "copy-"+app, true)
			must(err)
		}

		res, err := s.PurgeAccount(ctx, leaver)
		must(err)
		want := AccountPurgeResult{Apps: 1, Listings: 2, Releases: 2, Feedback: 2, Reports: 1}
		if res != want {
			t.Errorf("result:\n got %+v\nwant %+v", res, want)
		}
		if again, err := s.PurgeAccount(ctx, leaver); err != nil || again != (AccountPurgeResult{}) {
			t.Errorf("second run must find nothing: %+v %v", again, err)
		}

		for _, app := range []string{inTree, notTree} {
			if _, err := s.GetCatalogApp(ctx, app); !errors.Is(err, ErrNotFound) {
				t.Errorf("listing %s: %v", app, err)
			}
			if _, err := s.GetInstall(ctx, app, stayer); !errors.Is(err, ErrNotFound) {
				t.Errorf("install of %s goes with the listing: %v", app, err)
			}
		}
		fr, ok := s.(feedbackGetter)
		if !ok {
			t.Fatalf("%T cannot read feedback", s)
		}
		for _, id := range leaverFB {
			if _, err := fr.GetFeedback(ctx, id); !errors.Is(err, ErrNotFound) {
				t.Errorf("leaver feedback %s: %v", id, err)
			}
		}
		if _, err := fr.GetFeedback(ctx, stayerFB.ID); err != nil {
			t.Errorf("stayer feedback stays: %v", err)
		}
		if rr, err := s.ListReleaseRequests(ctx, stayer, ""); err != nil || len(rr) != 1 {
			t.Errorf("stayer releases stay: %v %v", rr, err)
		}
		// report_quota/{uid} went with the reports: the day's count starts over
		if c, err := s.AddReport(ctx, rep("PI-T2"+sfx), day, 1); err != nil || !c {
			t.Errorf("quota after purge: %v %v", c, err)
		}
	})
}

func TestStoreDeletedAccounts(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		u1, u2, active := "da-1"+sfx, "da-2"+sfx, "da-active"+sfx
		if err := s.DeactivateUser(ctx, u1); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if err := s.DeactivateUser(ctx, u2); err != nil {
			t.Fatal(err)
		}
		mine := func(cutoff time.Time) []DeletedAccount {
			t.Helper()
			all, err := s.DeletedAccounts(ctx, cutoff, 0)
			if err != nil {
				t.Fatal(err)
			}
			var out []DeletedAccount
			for _, d := range all {
				switch d.UID {
				case u1, u2, active:
					out = append(out, d)
				}
			}
			return out
		}
		got := mine(time.Now().Add(time.Hour))
		if len(got) != 2 || got[0].UID != u1 || got[1].UID != u2 || got[0].DeletedAt.IsZero() || !got[0].DeletedAt.Before(got[1].DeletedAt) {
			t.Fatalf("oldest first, active excluded: %+v", got)
		}
		d1, d2 := got[0].DeletedAt, got[1].DeletedAt

		// a second DELETE /v1/me keeps the first deletedAt
		if err := s.DeactivateUser(ctx, u2); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name   string
			cutoff time.Time
			want   []string
		}{
			{"before both", d1, nil},
			{"between", d1.Add(time.Microsecond), []string{u1}},
			{"after both", d2.Add(time.Microsecond), []string{u1, u2}},
		} {
			got := mine(tc.cutoff)
			ok := len(got) == len(tc.want)
			for i := range got {
				ok = ok && got[i].UID == tc.want[i]
			}
			if !ok {
				t.Errorf("%s: %+v, want %v", tc.name, got, tc.want)
			}
		}
		if l, err := s.DeletedAccounts(ctx, time.Now().Add(time.Hour), 1); err != nil || len(l) != 1 {
			t.Errorf("limit: %+v %v", l, err)
		}

		if _, err := s.PurgeAccount(ctx, u1); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkAccountPurged(ctx, u1); err != nil {
			t.Fatal(err)
		}
		if got := mine(time.Now().Add(time.Hour)); len(got) != 1 || got[0].UID != u2 {
			t.Errorf("a purged tombstone is not a candidate: %+v", got)
		}
		if err := s.MarkAccountPurged(ctx, active); !errors.Is(err, ErrNotDeleted) {
			t.Errorf("active uid: %v", err)
		}
	})
}

// Firestore keeps subcollections whose parent document is gone, so a run
// that failed half way leaves such trees behind; the retry must finish them.
func TestFirestoreAccountPurgeRetry(t *testing.T) {
	f := emulatorStore(t)
	ctx := t.Context()
	sfx := runSuffix()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	leaver, stayer := "pr-leaver"+sfx, "pr-stayer"+sfx
	half, theirs := "PR-HALF"+sfx, "PR-THEIRS"+sfx
	dsl := json.RawMessage(`{}`)

	// leaver's app whose document a failed run already deleted
	_, _, err := f.EnsureApp(ctx, leaver, half, dsl, "x")
	must(err)
	_, err = f.CommitVersion(ctx, leaver, half, Commit{BaseVersion: 1, DSL: dsl})
	must(err)
	_, _, err = f.CreateRecord(ctx, PrivateRecords(leaver, half, "memo"), Record{ID: "m1", Data: map[string]any{}})
	must(err)

	// leaver's mark on a post in stayer's app whose record document is gone
	for _, uid := range []string{leaver, stayer} {
		_, _, err := f.EnsureApp(ctx, uid, theirs, dsl, "x")
		must(err)
	}
	ok, err := f.PublishApp(ctx, stayer, theirs, dsl, "x", 1)
	if err != nil || !ok {
		t.Fatalf("publish: %v %v", ok, err)
	}
	rs := PublicRecords(theirs, "post")
	_, _, err = f.CreateRecord(ctx, rs, Record{ID: "p1", Entity: "post", Author: stayer, Data: map[string]any{}})
	must(err)
	_, err = f.SetMark(ctx, rs, "p1", "like", leaver, true)
	must(err)

	must(f.DeactivateUser(ctx, leaver))
	_, err = f.appRef(leaver, half).Delete(ctx)
	must(err)
	_, err = f.recRef(rs, "p1").Delete(ctx)
	must(err)

	res, err := f.PurgeAccount(ctx, leaver)
	must(err)
	want := AccountPurgeResult{Apps: 2, Marks: 1}
	if res != want {
		t.Errorf("result:\n got %+v\nwant %+v", res, want)
	}
	for _, c := range []struct {
		name string
		col  *firestore.CollectionRef
	}{
		{"users/{uid}/apps", f.userRef(leaver).Collection("apps")},
		{"versions", f.appRef(leaver, half).Collection("versions")},
		{"private records", f.appRef(leaver, half).Collection("entities").Doc("memo").Collection("records")},
		{"leaver's mark", f.recRef(rs, "p1").Collection("marks").Doc("like").Collection("by")},
	} {
		refs, err := c.col.DocumentRefs(ctx).GetAll()
		if err != nil || len(refs) != 0 {
			t.Errorf("%s left behind: %d %v", c.name, len(refs), err)
		}
	}
	if again, err := f.PurgeAccount(ctx, leaver); err != nil || again != (AccountPurgeResult{}) {
		t.Errorf("third run must find nothing: %+v %v", again, err)
	}
}
