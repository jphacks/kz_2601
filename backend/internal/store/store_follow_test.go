package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestStoreFollows(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		app, other := "FOLLOW"+sfx, "FOLLOW2"+sfx
		a, b, c := "fo-a"+sfx, "fo-b"+sfx, "fo-c"+sfx
		list := func(app, uid string) []string {
			t.Helper()
			got, err := s.ListFollows(ctx, app, uid)
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		if got := list(app, a); len(got) != 0 {
			t.Fatalf("none yet: %v", got)
		}
		for _, st := range []struct {
			name    string
			target  string
			on      bool
			wantLen int
		}{
			{"unfollow before any is a no-op", b, false, 0},
			{"a follows b", b, true, 1},
			{"again (idempotent)", b, true, 1},
			{"a follows c", c, true, 2},
			{"a unfollows b", b, false, 1},
			{"again (idempotent)", b, false, 1},
			{"a follows b again (goes last)", b, true, 2},
		} {
			if n, err := s.SetFollow(ctx, app, a, st.target, st.on); err != nil || n != st.wantLen {
				t.Fatalf("%s: %d %v, want %d", st.name, n, err, st.wantLen)
			}
		}
		if got := list(app, a); !slices.Equal(got, []string{c, b}) {
			t.Errorf("a follows %v, want [c b] in order", got)
		}
		if got := list(other, a); len(got) != 0 {
			t.Errorf("follows are per app: %v", got)
		}
		if got := list(app, b); len(got) != 0 {
			t.Errorf("following is one way: %v", got)
		}
		for _, u := range []string{c, b} {
			if _, err := s.SetFollow(ctx, app, a, u, false); err != nil {
				t.Fatal(err)
			}
		}
		if got := list(app, a); len(got) != 0 {
			t.Errorf("all removed: %v", got)
		}
	})
}

func TestFollowsCap(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	for i := range MaxFollows {
		if _, err := s.SetFollow(ctx, "APP", "me", "u"+string(rune('a'+i%26))+string(rune('0'+i/26%10))+string(rune('0'+i/260)), true); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.SetFollow(ctx, "APP", "me", "one-more", true); !errors.Is(err, ErrFollowsFull) || n != MaxFollows {
		t.Fatalf("past the cap: %d %v", n, err)
	}
	if n, err := s.SetFollow(ctx, "APP", "me", "ua00", false); err != nil || n != MaxFollows-1 {
		t.Fatalf("unfollow at the cap: %d %v", n, err)
	}
}

func TestStoreAccountPurgeFollows(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		app := "PF-APP"+sfx
		leaver, fan, idol := "pf-leaver"+sfx, "pf-fan"+sfx, "pf-idol"+sfx
		if _, _, err := s.EnsureApp(ctx, leaver, app, json.RawMessage(`{}`), "x"); err != nil {
			t.Fatal(err)
		}
		for _, f := range [][2]string{{leaver, idol}, {fan, leaver}, {fan, idol}} {
			if _, err := s.SetFollow(ctx, app, f[0], f[1], true); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.DeactivateUser(ctx, leaver); err != nil {
			t.Fatal(err)
		}
		res, err := s.PurgeAccount(ctx, leaver)
		if err != nil || res.Follows != 2 {
			t.Fatalf("purge: %+v %v (want Follows 2)", res, err)
		}
		if got, _ := s.ListFollows(ctx, app, leaver); len(got) != 0 {
			t.Errorf("leaver's follows stay: %v", got)
		}
		if got, _ := s.ListFollows(ctx, app, fan); !slices.Equal(got, []string{idol}) {
			t.Errorf("fan follows %v, want only idol", got)
		}
	})
}
