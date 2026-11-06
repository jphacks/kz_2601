package ratelimit

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
)

type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

// The id, limit and reset Peek users see are the ones Allow charges.
func TestPeekReadsWhatAllowCharges(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 25, 23, 30, 0, 0, JST)}
	mem := NewMemory()
	cfg := Config{Limits: Limits{Anonymous: 7, SignedIn: 11, Pro: 13, Global: 1000}, Weights: DefaultWeights, Env: "test", Now: c.now}
	pro := auth.Identity{UID: "pro/1"}
	l := New(cfg, mem, proSet{pro.UID: true})

	for _, tc := range []struct {
		id   auth.Identity
		tier Tier
	}{{anon, TierAnonymous}, {signed, TierSignedIn}, {pro, TierPro}} {
		d := mustAllow(t, l, tc.id, "", 3)
		mustAllow(t, l, tc.id, "", 1)
		if got := TierOf(tc.tier == TierPro, tc.id.Anonymous); got != d.Tier {
			t.Errorf("%s: TierOf = %s, Allow = %s", tc.id.UID, got, d.Tier)
		}
		if got := cfg.DailyLimit(d.Tier); got != d.Limit {
			t.Errorf("%s: DailyLimit = %d, Allow = %d", tc.id.UID, got, d.Limit)
		}
		day, reset := cfg.Today(c.now())
		if !reset.Equal(d.ResetAt) {
			t.Errorf("reset = %v, Allow = %v", reset, d.ResetAt)
		}
		got, err := mem.Peek(context.Background(), []string{cfg.UserCounterID(day, tc.id.UID), "missing"})
		if err != nil || got[0] != 4 || got[1] != 0 {
			t.Errorf("%s: Peek = %v, %v (want [4 0])", tc.id.UID, got, err)
		}
	}
}

func TestTodayUsesJST(t *testing.T) {
	// 15:30 UTC is 00:30 JST of the next day.
	day, reset := Config{}.Today(time.Date(2026, 11, 6, 15, 30, 0, 0, time.UTC))
	if day != "2026-11-07" || !reset.Equal(time.Date(2026, 11, 8, 0, 0, 0, 0, JST)) {
		t.Fatalf("Today = %s %v", day, reset)
	}
	if id := (Config{}).UserCounterID(day, "a/b"); id != "local_2026-11-07_uid_a_b" {
		t.Fatalf("id = %s", id)
	}
}

// Runs against the local emulator only (make emulator); never staging/prod.
func TestFirestorePeek(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	c, err := firestore.NewClientWithDatabase(ctx, "demo-kz2601", firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := NewFirestore(c, "rateLimitsTest")
	id := "peek_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if _, err := s.Consume(ctx, []Counter{{ID: id, Scope: ScopeUser, Limit: 10}}, 3, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Peek(ctx, []string{id, id + "_missing"})
	if err != nil || len(got) != 2 || got[0] != 3 || got[1] != 0 {
		t.Fatalf("Peek = %v, %v", got, err)
	}
}
