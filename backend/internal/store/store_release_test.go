package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cloud.google.com/go/firestore"
)

// setReleaseStatus stands in for the admin editing status in the console.
func setReleaseStatus(t *testing.T, s Store, id, status string) {
	t.Helper()
	var err error
	switch v := s.(type) {
	case *Memory:
		err = v.SetReleaseStatus(id, status)
	case *Firestore:
		_, err = v.c.Collection(releaseCollection).Doc(id).Update(t.Context(), []firestore.Update{{Path: "status", Value: status}})
	default:
		t.Fatalf("unknown store %T", s)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoreReleaseRequests(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		owner, other := "rel-owner"+sfx, "rel-other"+sfx
		a1, a2 := "REL1"+sfx, "REL2"+sfx
		dsl := json.RawMessage(`{"initialScreen":"home"}`)

		first, created, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: a1, OwnerUID: owner, Name: "N", Version: 3,
			DSL: dsl, Purpose: "p", Contact: "c@example.com"})
		if err != nil || !created || first.ID == "" || first.Status != ReleaseReceived || first.CreatedAt.IsZero() ||
			!first.UpdatedAt.Equal(first.CreatedAt) || first.DSL != nil || first.Version != 3 || first.Name != "N" {
			t.Fatalf("create: %+v %v %v", first, created, err)
		}

		// idempotent per (owner, app) while open, whatever the open status is
		for _, st := range []string{"", ReleaseReviewing, ReleasePlanned} {
			if st != "" {
				setReleaseStatus(t, s, first.ID, st)
			}
			again, created, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: a1, OwnerUID: owner, Purpose: "p2"})
			if err != nil || created || again.ID != first.ID || again.Purpose != "p" {
				t.Fatalf("open (%q) must be returned: %+v %v %v", st, again, created, err)
			}
		}

		// declined closes it: the next request is new
		setReleaseStatus(t, s, first.ID, ReleaseDeclined)
		second, created, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: a1, OwnerUID: owner, Purpose: "p3"})
		if err != nil || !created || second.ID == first.ID || second.Status != ReleaseReceived {
			t.Fatalf("after declined: %+v %v %v", second, created, err)
		}

		// another app, another owner of the same app: separate requests
		third, created, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: a2, OwnerUID: owner, Purpose: "p4"})
		if err != nil || !created {
			t.Fatalf("other app: %+v %v %v", third, created, err)
		}
		if _, created, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: a1, OwnerUID: other, Purpose: "p5"}); err != nil || !created {
			t.Fatalf("other owner: %v %v", created, err)
		}

		ids := func(rs []ReleaseRequest) []string {
			out := []string{}
			for _, r := range rs {
				if r.DSL != nil {
					t.Errorf("list must omit dsl: %+v", r)
				}
				out = append(out, r.ID)
			}
			return out
		}
		for _, tc := range []struct {
			name, uid, app string
			want           []string
		}{
			{"all apps newest first", owner, "", []string{third.ID, second.ID, first.ID}},
			{"one app", owner, a1, []string{second.ID, first.ID}},
			{"other app", owner, a2, []string{third.ID}},
			{"unknown app", owner, "NONE" + sfx, []string{}},
			{"unknown uid", "rel-nobody" + sfx, "", []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := s.ListReleaseRequests(ctx, tc.uid, tc.app)
				if err != nil || fmt.Sprint(ids(got)) != fmt.Sprint(tc.want) {
					t.Fatalf("got %v %v, want %v", ids(got), err, tc.want)
				}
			})
		}
	})
}

// ListReleaseRequests is "newest first" bounded by MaxReleaseRequestsPerUser:
// past the bound the oldest are the ones left out.
func TestStoreReleaseRequestsNewestWithinBound(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		owner := "rel-many" + sfx
		var oldest, newest string
		for i := range MaxReleaseRequestsPerUser + 1 {
			rr, _, err := s.CreateReleaseRequest(ctx, ReleaseRequest{AppID: fmt.Sprintf("M%03d%s", i, sfx), OwnerUID: owner, Purpose: "p"})
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				oldest = rr.ID
			}
			newest = rr.ID
		}
		got, err := s.ListReleaseRequests(ctx, owner, "")
		if err != nil || len(got) != MaxReleaseRequestsPerUser {
			t.Fatalf("len %d %v", len(got), err)
		}
		if got[0].ID != newest {
			t.Errorf("first must be the newest: %s, want %s", got[0].ID, newest)
		}
		for _, r := range got {
			if r.ID == oldest {
				t.Errorf("the oldest request must be the one left out, got it at the bound")
			}
		}
	})
}
