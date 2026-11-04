package entitlement

import (
	"context"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// #449: a verified Pro purchase lifts the ai daily limit to the Pro tier.
func TestVerifiedProGetsProRateLimit(t *testing.T) {
	ctx := context.Background()
	st := NewMemory()
	e := newEnv(t, st)
	lim := ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, nil, NewProChecker(st, nil))

	dec, err := lim.Allow(ctx, auth.Identity{UID: "buyer"}, "", 1)
	if err != nil || dec.Tier != ratelimit.TierSignedIn || dec.Limit != ratelimit.DefaultLimits.SignedIn {
		t.Fatalf("before purchase: %+v %v", dec, err)
	}
	if _, err := e.svc.SubmitApple(ctx, "buyer", e.jws(t, appstoretest.Monthly(bundle, product, "o449", time.Now()))); err != nil {
		t.Fatal(err)
	}
	lim = ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, nil, NewProChecker(st, nil))
	dec, err = lim.Allow(ctx, auth.Identity{UID: "buyer"}, "", 1)
	if err != nil || dec.Tier != ratelimit.TierPro || dec.Limit != ratelimit.DefaultLimits.Pro {
		t.Fatalf("after purchase: %+v %v", dec, err)
	}
	// an anonymous account that bought Pro is Pro too (and skips the per-IP counter)
	dec, _ = lim.Allow(ctx, auth.Identity{UID: "buyer", Anonymous: true}, "203.0.113.9", 1)
	if dec.Tier != ratelimit.TierPro {
		t.Fatalf("anonymous buyer: %+v", dec)
	}
}
