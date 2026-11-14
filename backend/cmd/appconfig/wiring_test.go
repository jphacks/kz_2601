package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// AUTH_MODE=firebase without a project must fail closed (every request 401),
// never fall back to the dev verifier that trusts bearer values as uids.
func TestNewVerifier(t *testing.T) {
	ctx := context.Background()
	if _, ok := newVerifier(ctx, auth.ModeDev, "", quietLogger()).(auth.DevVerifier); !ok {
		t.Fatal("dev mode must use DevVerifier")
	}
	v := newVerifier(ctx, auth.ModeFirebase, "", quietLogger())
	if _, ok := v.(auth.NotConfigured); !ok {
		t.Fatalf("firebase without project = %T, want NotConfigured", v)
	}
	if _, err := v.VerifyIDToken(ctx, "dev-uid"); err == nil {
		t.Fatal("NotConfigured must reject every token")
	}
	// an unknown mode is not dev
	if _, ok := newVerifier(ctx, "Dev", "", quietLogger()).(auth.DevVerifier); ok {
		t.Fatal("only the exact dev mode may trust bearer uids")
	}
}

func TestNewStoreFallsBackToMemory(t *testing.T) {
	ctx := context.Background()
	t.Setenv("STORE", "")
	st, closeStore := newStore(ctx, fsenv.Target{}, quietLogger())
	defer closeStore()
	if _, ok := st.(*store.Memory); !ok {
		t.Fatalf("no project = %T, want memory", st)
	}
	t.Setenv("STORE", "memory")
	st, closeStore = newStore(ctx, fsenv.Target{Project: "p"}, quietLogger())
	defer closeStore()
	if _, ok := st.(*store.Memory); !ok {
		t.Fatalf("STORE=memory = %T, want memory", st)
	}
}

func TestNewExt(t *testing.T) {
	ctx := context.Background()
	for _, v := range []string{"", "   ", "/"} {
		t.Setenv("EXT_RUNNER_URL", v)
		if r := newExt(ctx, quietLogger()); r != nil {
			t.Fatalf("EXT_RUNNER_URL=%q must turn ext off, got %T", v, r)
		}
	}
	t.Setenv("EXT_RUNNER_URL", " http://localhost:8090/ ")
	r := newExt(ctx, quietLogger())
	c, ok := r.(*extapi.Client)
	if !ok {
		t.Fatalf("localhost runner = %T", r)
	}
	if c.BaseURL != "http://localhost:8090" || c.HTTP == nil || c.HTTP.Timeout != 20*time.Second {
		t.Fatalf("client = %+v", c)
	}
}

// Defaults (no env) start every optional feature off without exiting.
func TestDefaultWiring(t *testing.T) {
	for _, k := range []string{"COIN_BILLING_MODE", "COIN_PACKS", "CALLS_ENABLED", "PLAN_LIMITS_MODE", "RATE_LIMIT_STORE",
		"APPSTORE_BUNDLE_IDS", "APPSTORE_PRODUCT_IDS", "APPSTORE_ENVIRONMENTS"} {
		t.Setenv(k, "")
	}
	log := quietLogger()
	st := store.NewMemory()
	ents := newEntitlements(st, log)
	if _, ok := ents.Store.(*entitlement.Memory); !ok {
		t.Fatalf("entitlement store = %T, want memory", ents.Store)
	}
	if len(ents.Verifier.BundleIDs) == 0 || len(ents.Verifier.Environments) == 0 {
		t.Fatalf("policy = %+v", ents.Verifier.Policy)
	}
	cs := newCoins(st, ents, log)
	if cs.Counters != nil {
		t.Fatal("memory store must not read Firestore counters")
	}
	// coin packs keep Pro's bundle ids / environments but never Pro's products
	if len(cs.Verifier.BundleIDs) != len(ents.Verifier.BundleIDs) {
		t.Fatalf("coin policy bundle ids = %v", cs.Verifier.BundleIDs)
	}
	for _, p := range cs.Verifier.ProductIDs {
		for _, pro := range ents.Verifier.ProductIDs {
			if p == pro {
				t.Fatalf("coin verifier accepts the Pro product %q", p)
			}
		}
	}
	cl := newCalls(st, cs, log)
	if cl.Config.Enabled || cl.Rooms != nil {
		t.Fatalf("calls must be off by default: %+v", cl.Config)
	}
	_ = planLimits(log)
	if usage(st, log) == nil {
		t.Fatal("usage option is nil")
	}
}

type proStore struct {
	entitlement.Store
	pros map[string]*entitlement.Pro
	err  error
}

func (s proStore) Get(_ context.Context, uid string) (*entitlement.Pro, error) {
	return s.pros[uid], s.err
}

func TestProPeriodEnd(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := now.Add(10 * 24 * time.Hour)
	ents := &entitlement.Service{
		Verifier: &appstore.Verifier{Policy: appstore.Policy{Environments: []string{appstore.EnvProduction}}},
		Store: proStore{pros: map[string]*entitlement.Pro{
			"pro":     {Environment: appstore.EnvProduction, ExpiresAt: end},
			"expired": {Environment: appstore.EnvProduction, ExpiresAt: now.Add(-time.Hour)},
			"sandbox": {Environment: appstore.EnvSandbox, ExpiresAt: end},
			"revoked": {Environment: appstore.EnvProduction, ExpiresAt: end, RevokedAt: now},
		}},
		Now: func() time.Time { return now },
	}
	f := proPeriodEnd(ents)
	ctx := context.Background()
	if got, err := f(ctx, "pro"); err != nil || !got.Equal(end) {
		t.Fatalf("pro = %v %v, want %v", got, err, end)
	}
	for _, uid := range []string{"expired", "sandbox", "revoked", "none"} {
		if got, err := f(ctx, uid); err != nil || !got.IsZero() {
			t.Fatalf("%s = %v %v, want zero", uid, got, err)
		}
	}
	boom := errors.New("boom")
	ents.Store = proStore{err: boom}
	if got, err := f(ctx, "pro"); !errors.Is(err, boom) || !got.IsZero() {
		t.Fatalf("store error = %v %v", got, err)
	}
}
