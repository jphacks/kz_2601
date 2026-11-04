package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Shares are frozen snapshots: create once per token, revoke is idempotent (#49, #578).
func TestStoreShares(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		tok, owner := "tok"+sfx, "sh-owner"+sfx
		at := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC) // microseconds: Firestore's precision
		in := Share{Token: tok, AppID: "SHA" + sfx, OwnerUID: owner, Name: "N", Version: 3,
			DSL: json.RawMessage(`{"initialScreen":"home"}`), CreatedAt: at}
		if err := s.CreateShare(ctx, in); err != nil {
			t.Fatal(err)
		}

		cases := []struct {
			name  string
			token string
			err   error
			check func(t *testing.T, got *Share)
		}{
			{"created", tok, nil, func(t *testing.T, got *Share) {
				if got.Token != tok || got.AppID != in.AppID || got.OwnerUID != owner || got.Name != "N" || got.Version != 3 ||
					string(got.DSL) != string(in.DSL) || got.Revoked || !got.CreatedAt.Equal(at) {
					t.Fatalf("got %+v", got)
				}
			}},
			{"unknown", "nope" + sfx, ErrNotFound, nil},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := s.GetShare(ctx, c.token)
				if !errors.Is(err, c.err) {
					t.Fatalf("err %v, want %v", err, c.err)
				}
				if c.check != nil {
					c.check(t, got)
				}
			})
		}

		// the token is never reused, and the first snapshot stays
		if err := s.CreateShare(ctx, Share{Token: tok, Name: "other"}); !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("duplicate token: %v", err)
		}
		if got, _ := s.GetShare(ctx, tok); got == nil || got.Name != "N" {
			t.Fatalf("duplicate overwrote: %+v", got)
		}

		// CreatedAt defaults to now
		before := time.Now().Add(-time.Second)
		if err := s.CreateShare(ctx, Share{Token: tok + "-now", AppID: "SHA" + sfx, OwnerUID: owner}); err != nil {
			t.Fatal(err)
		}
		if got, err := s.GetShare(ctx, tok+"-now"); err != nil || got.CreatedAt.Before(before) {
			t.Fatalf("default createdAt: %+v %v", got, err)
		}

		// revoke keeps the document (GET still finds it, flagged) and is idempotent
		for i := range 2 {
			if err := s.RevokeShare(ctx, tok); err != nil {
				t.Fatalf("revoke #%d: %v", i+1, err)
			}
			got, err := s.GetShare(ctx, tok)
			if err != nil || !got.Revoked || string(got.DSL) != string(in.DSL) {
				t.Fatalf("after revoke #%d: %+v %v", i+1, got, err)
			}
		}
		if err := s.RevokeShare(ctx, "nope"+sfx); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoke unknown: %v", err)
		}
		// other shares are untouched
		if got, err := s.GetShare(ctx, tok+"-now"); err != nil || got.Revoked {
			t.Fatalf("sibling revoked: %+v %v", got, err)
		}
	})
}
