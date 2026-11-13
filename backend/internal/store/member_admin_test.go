package store

import (
	"errors"
	"testing"
	"time"
)

// SetMemberAdmin (#2561): grant / revoke / non-member / owner not joined.
func TestStoreSetMemberAdminCases(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := t.Context()
		app := "ADM" + sfx
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		if _, err := s.AddMember(ctx, app, Member{UID: "m", Via: MemberViaShare, JoinedAt: t0}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddMember(ctx, app, Member{UID: "x", Via: MemberViaShare, JoinedAt: t0.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ClaimApp(ctx, "owner", app); err != nil {
			t.Fatal(err)
		}

		cases := []struct {
			name      string
			uid       string
			admin     bool
			wantErr   error
			wantAdmin bool
		}{
			{"grant", "m", true, nil, true},
			{"grant again", "m", true, nil, true},
			{"revoke", "m", false, nil, false},
			{"non-member", "stranger", true, ErrNotFound, false},
			// the store does not treat the owner as a member: not joined -> ErrNotFound
			{"owner not joined", "owner", true, ErrNotFound, false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				err := s.SetMemberAdmin(ctx, app, c.uid, c.admin)
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				if c.wantErr != nil {
					if _, gerr := s.GetMember(ctx, app, c.uid); !errors.Is(gerr, ErrNotFound) {
						t.Fatalf("not-found uid became a member: %v", gerr)
					}
					return
				}
				m, err := s.GetMember(ctx, app, c.uid)
				if err != nil || m.Admin != c.wantAdmin {
					t.Fatalf("member = %+v %v, want admin %v", m, err, c.wantAdmin)
				}
				if m.Via != MemberViaShare || !m.JoinedAt.Equal(t0) {
					t.Fatalf("other fields changed: %+v", m)
				}
			})
		}

		// other members and other apps are untouched
		if x, err := s.GetMember(ctx, app, "x"); err != nil || x.Admin {
			t.Fatalf("x = %+v %v", x, err)
		}
		if err := s.SetMemberAdmin(ctx, "OTHER"+app, "m", true); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other app: %v", err)
		}
	})
}
