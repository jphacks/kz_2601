package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

// stores runs f on memory and, with the emulator, on Firestore (never stg/prod).
// env is unique per run because the emulator keeps data between runs.
func stores(t *testing.T, f func(t *testing.T, st ratelimit.Store, env string)) {
	t.Run("memory", func(t *testing.T) { f(t, nil, "test") })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		c, err := fsenv.NewClient(context.Background(), fsenv.Target{Project: "demo-kz2601", Database: "(default)"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		f(t, ratelimit.NewFirestore(c, "runnerQuotaTest"), "test"+strconv.FormatInt(time.Now().UnixNano(), 36))
	})
}

func newQuota(st ratelimit.Store, env string, lim Limits, pro ProChecker) (*Quota, *clock) {
	c := &clock{t: time.Date(2026, 11, 6, 23, 0, 0, 0, ratelimit.JST)}
	if st == nil {
		st = ratelimit.NewMemoryClock(c.now)
	}
	return New(Config{Limits: lim, Env: env, Now: c.now}, st, pro), c
}

func begin(t *testing.T, q *Quota, r Run) Decision {
	t.Helper()
	d, err := q.Begin(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

var (
	appA = Run{AppID: "app-a", CreatorUID: "alice", ActorUID: "bob", Attempt: 1}
	appB = Run{AppID: "app-b", CreatorUID: "carol", ActorUID: "bob", Attempt: 1}
)

func TestBeginStopsAtLimit(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{AppExecFree: 3}, nil)
		for i := range 3 {
			if d := begin(t, q, appA); !d.Allowed || d.Outcome != OutcomeOK {
				t.Fatalf("run %d: %+v", i, d)
			}
		}
		d := begin(t, q, appA)
		if d.Allowed || d.Outcome != OutcomeSkippedQuota || d.Block.Scope != ScopeAppExec || d.Block.Limit != 3 || d.Block.Used != 3 || !d.Charge.Empty() {
			t.Fatalf("over limit: %+v %+v", d, d.Block)
		}
		if !strings.HasPrefix(d.Block.CounterID, env+"_2026-11-06_rx_app_app-a") {
			t.Fatalf("counter id %q", d.Block.CounterID)
		}
	})
}

func TestAppsAreSeparate(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{AppExecFree: 1}, nil)
		begin(t, q, appA)
		if begin(t, q, appA).Allowed {
			t.Fatal("app-a must be stopped")
		}
		if d := begin(t, q, appB); !d.Allowed {
			t.Fatalf("app-b must keep running: %+v", d.Block)
		}
	})
}

func TestCreatorActorAndGlobal(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{CreatorExecFree: 2, ActorPerApp: 5, Global: 4}, nil)
		other := appA
		other.AppID = "app-a2"
		begin(t, q, appA)
		begin(t, q, other)
		if d := begin(t, q, appA); d.Allowed || d.Block.Scope != ScopeCreatorExec {
			t.Fatalf("creator over: %+v", d)
		}
		// actor per app
		q2, _ := newQuota(st, env+"x", Limits{ActorPerApp: 1}, nil)
		begin(t, q2, appA)
		if d := begin(t, q2, appA); d.Allowed || d.Block.Scope != ScopeActorExec {
			t.Fatalf("actor over: %+v", d)
		}
		sched := appA
		sched.ActorUID = ""
		if d := begin(t, q2, sched); !d.Allowed {
			t.Fatalf("schedule has no actor: %+v", d.Block)
		}
		// global
		q3, _ := newQuota(st, env+"g", Limits{Global: 2}, nil)
		begin(t, q3, appA)
		begin(t, q3, appB)
		if d := begin(t, q3, appA); d.Allowed || d.Outcome != OutcomeSkippedGlobal || d.Block.Scope != ScopeGlobal {
			t.Fatalf("global: %+v", d)
		}
	})
}

func TestDayRollsOverAtMidnightJST(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, c := newQuota(st, env, Limits{AppExecFree: 1}, nil)
		d := begin(t, q, appA)
		if want := time.Date(2026, 11, 7, 0, 0, 0, 0, ratelimit.JST); !d.ResetAt.Equal(want) {
			t.Fatalf("resetAt %v", d.ResetAt)
		}
		c.t = c.t.Add(59 * time.Minute)
		if begin(t, q, appA).Allowed {
			t.Fatal("same JST day")
		}
		c.t = c.t.Add(time.Minute)
		if d := begin(t, q, appA); !d.Allowed {
			t.Fatalf("new day: %+v", d.Block)
		}
	})
}

func TestRefundAndRetries(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{AppExecFree: 1, AppWritesFree: 5, RunWritesFree: 0}, nil)
		d := begin(t, q, appA)
		w, err := q.Writes(context.Background(), d, 3)
		if err != nil || w.Allowed != 3 {
			t.Fatalf("writes: %+v %v", w, err)
		}
		// retries are not counted
		retry := appA
		retry.Attempt = 2
		if r := begin(t, q, retry); !r.Allowed || !r.Charge.Empty() {
			t.Fatalf("retry: %+v", r)
		}
		if begin(t, q, appA).Allowed {
			t.Fatal("limit 1 reached")
		}
		// failed for good: everything comes back
		if err := q.Refund(context.Background(), d.Charge, w.Charge); err != nil {
			t.Fatal(err)
		}
		d2 := begin(t, q, appA)
		if !d2.Allowed {
			t.Fatalf("after refund: %+v", d2.Block)
		}
		if w, _ := q.Writes(context.Background(), d2, 5); w.Allowed != 5 {
			t.Fatalf("writes after refund: %+v", w)
		}
	})
}

func TestProAndFree(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		lim := DefaultLimits
		lim.AppExecFree, lim.AppExecPro = 1, 3
		q, _ := newQuota(st, env, lim, proSet{"carol": true})
		begin(t, q, appA)
		if begin(t, q, appA).Allowed {
			t.Fatal("free app stops at 1")
		}
		for i := range 3 {
			if d := begin(t, q, appB); !d.Allowed || d.Plan != PlanPro {
				t.Fatalf("pro run %d: %+v", i, d)
			}
		}
		if begin(t, q, appB).Allowed {
			t.Fatal("pro app stops at 3")
		}
		// per-run shape: free 1 write / 1 recipient, Pro 10 / 200
		free, _ := newQuota(st, env+"s", DefaultLimits, proSet{"carol": true})
		fd, pd := begin(t, free, appA), begin(t, free, appB)
		ctx := context.Background()
		if w, _ := free.Writes(ctx, fd, 4); w.Allowed != 1 || w.Dropped != 3 || w.Block.Scope != ScopeRunWrites {
			t.Fatalf("free writes: %+v", w)
		}
		if w, _ := free.Writes(ctx, pd, 12); w.Allowed != 10 || w.Dropped != 2 {
			t.Fatalf("pro writes: %+v", w)
		}
		if r, _ := free.Recipients(ctx, fd, []string{"alice", "bob"}); len(r.Allowed) != 1 || r.Dropped[0].Block.Scope != ScopeRunRecipients {
			t.Fatalf("free recipients: %+v", r)
		}
		many := make([]string, 205)
		for i := range many {
			many[i] = "u" + strconv.Itoa(i)
		}
		if r, _ := free.Recipients(ctx, pd, many); len(r.Allowed) != 200 || len(r.Dropped) != 5 {
			t.Fatalf("pro recipients: %d %d", len(r.Allowed), len(r.Dropped))
		}
		if n, b := free.ScheduledRules(PlanFree, 2); n != 1 || b.Scope != ScopeScheduledRules {
			t.Fatalf("scheduled free: %d %+v", n, b)
		}
		if n, b := free.ScheduledRules(PlanPro, 5); n != 5 || b != nil {
			t.Fatalf("scheduled pro: %d %+v", n, b)
		}
	})
}

func TestAnonymousAndSharedNeedPlan(t *testing.T) {
	q, _ := newQuota(nil, "test", DefaultLimits, proSet{"anon-pro": true})
	anon := Run{AppID: "x", CreatorUID: "anon-1", CreatorAnonymous: true, Attempt: 1}
	if d := begin(t, q, anon); d.Allowed || d.Outcome != OutcomeSkippedPlan || d.Block.Scope != ScopeAnonymous || !d.Charge.Empty() {
		t.Fatalf("anonymous: %+v", d)
	}
	shared := Run{AppID: "x", CreatorUID: "free-1", Shared: true, Attempt: 1}
	if d := begin(t, q, shared); d.Allowed || d.Block.Scope != ScopeSharedNeedsPro {
		t.Fatalf("shared free: %+v", d)
	}
	// a verified Pro is Pro even when anonymous (#314)
	ap := Run{AppID: "x", CreatorUID: "anon-pro", CreatorAnonymous: true, Shared: true, Attempt: 1}
	if d := begin(t, q, ap); !d.Allowed || d.Plan != PlanPro {
		t.Fatalf("anonymous pro: %+v", d)
	}
	// a stopped run grants nothing
	d := begin(t, q, anon)
	if w, _ := q.Writes(context.Background(), d, 2); w.Allowed != 0 || w.Dropped != 2 {
		t.Fatalf("writes of skipped run: %+v", w)
	}
	if r, _ := q.Recipients(context.Background(), d, []string{"a"}); len(r.Allowed) != 0 || len(r.Dropped) != 1 {
		t.Fatalf("recipients of skipped run: %+v", r)
	}
}

func TestZeroDisablesCounter(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{}, nil)
		for range 5 {
			d := begin(t, q, appA)
			if !d.Allowed || !d.Charge.Empty() {
				t.Fatalf("nothing counted: %+v", d)
			}
		}
		d := begin(t, q, appA)
		if w, _ := q.Writes(context.Background(), d, 50); w.Allowed != 50 {
			t.Fatalf("writes: %+v", w)
		}
		if r, _ := q.Recipients(context.Background(), d, []string{"a", "b", "a"}); len(r.Allowed) != 2 {
			t.Fatalf("recipients: %+v", r)
		}
	})
}

func TestDailyWritesPartialFill(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		q, _ := newQuota(st, env, Limits{AppWritesPro: 5}, proSet{"carol": true})
		d := begin(t, q, appB)
		ctx := context.Background()
		if w, _ := q.Writes(ctx, d, 3); w.Allowed != 3 {
			t.Fatalf("first: %+v", w)
		}
		w, err := q.Writes(ctx, d, 4)
		if err != nil || w.Allowed != 2 || w.Dropped != 2 || w.Block.Scope != ScopeAppWrites || w.Block.Limit != 5 {
			t.Fatalf("partial: %+v %v", w, err)
		}
		if w, _ := q.Writes(ctx, d, 1); w.Allowed != 0 || w.Block.Used != 5 {
			t.Fatalf("full: %+v", w)
		}
	})
}

func TestRecipientLimits(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		lim := Limits{RecipientPerApp: 2, RecipientTotal: 3, AppNotifyPro: 100}
		q, _ := newQuota(st, env, lim, proSet{"alice": true, "carol": true})
		ctx := context.Background()
		da, db := begin(t, q, appA), begin(t, q, appB)
		for range 2 {
			if r, _ := q.Recipients(ctx, da, []string{"dave"}); len(r.Allowed) != 1 {
				t.Fatalf("under per-app: %+v", r)
			}
		}
		r, _ := q.Recipients(ctx, da, []string{"dave", "erin"})
		if len(r.Allowed) != 1 || r.Allowed[0] != "erin" || r.Dropped[0].Block.Scope != ScopeRecipientApp {
			t.Fatalf("per-app 2: %+v", r)
		}
		// dave got 2 from app-a; 1 more from app-b reaches the total of 3
		if r, _ := q.Recipients(ctx, db, []string{"dave"}); len(r.Allowed) != 1 {
			t.Fatalf("other app: %+v", r)
		}
		if r, _ := q.Recipients(ctx, db, []string{"dave"}); len(r.Allowed) != 0 || r.Dropped[0].Block.Scope != ScopeRecipientTotal {
			t.Fatalf("total: %+v", r)
		}
	})
}

func TestAppNotifyLimitAndRefundOfDropped(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		lim := Limits{AppNotifyPro: 3, RecipientPerApp: 1}
		q, _ := newQuota(st, env, lim, proSet{"carol": true})
		ctx := context.Background()
		d := begin(t, q, appB)
		if r, _ := q.Recipients(ctx, d, []string{"a"}); len(r.Allowed) != 1 {
			t.Fatal(r)
		}
		// "a" is over its per-app limit: it must not use the app's recipients
		r2, _ := q.Recipients(ctx, d, []string{"a", "b", "c"})
		if len(r2.Allowed) != 2 || r2.Dropped[0].UID != "a" {
			t.Fatalf("second: %+v", r2)
		}
		r, _ := q.Recipients(ctx, d, []string{"d", "e"})
		if len(r.Allowed) != 0 || r.Dropped[0].Block.Scope != ScopeAppNotify || r.Dropped[0].Block.Used != 3 {
			t.Fatalf("app notify full: %+v", r)
		}
		// d and e were not left counted per person: after refunding r2 they fit
		if err := q.Refund(ctx, r2.Charge); err != nil {
			t.Fatal(err)
		}
		if r, _ := q.Recipients(ctx, d, []string{"d", "e"}); len(r.Allowed) != 2 {
			t.Fatalf("after refund: %+v", r)
		}
	})
}

// failing is a Store whose Consume/Refund always fail (Firestore down).
type failing struct{}

func (failing) Consume(context.Context, []ratelimit.Counter, int, time.Time) (ratelimit.Usage, error) {
	return ratelimit.Usage{}, errors.New("unavailable")
}

func (failing) Refund(context.Context, []ratelimit.Counter, int, time.Time) error {
	return errors.New("unavailable")
}

func TestFirestoreDownCountsInMemory(t *testing.T) {
	var buf bytes.Buffer
	q, _ := newQuota(failing{}, "test", Limits{AppExecFree: 2}, nil)
	q.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	d := begin(t, q, appA)
	if !d.Allowed || len(d.Charge.Parts) != 1 || !d.Charge.Parts[0].Memory {
		t.Fatalf("first: %+v", d)
	}
	begin(t, q, appA)
	if begin(t, q, appA).Allowed {
		t.Fatal("memory must keep counting (not fail open)")
	}
	if err := q.Refund(context.Background(), d.Charge); err != nil {
		t.Fatal(err)
	}
	if !begin(t, q, appA).Allowed {
		t.Fatal("memory refund")
	}
	if !strings.Contains(buf.String(), "counting in memory") {
		t.Fatalf("no warning: %s", buf.String())
	}
}

func TestNearLimit(t *testing.T) {
	q, _ := newQuota(nil, "test", Limits{AppExecFree: 5}, nil)
	for i := range 3 {
		if begin(t, q, appA).NearLimit {
			t.Fatalf("run %d near", i)
		}
	}
	if !begin(t, q, appA).NearLimit { // 4/5 = 80%
		t.Fatal("4 of 5 must warn")
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{"APP_ENV": "stg", "RUNNER_GLOBAL_DAILY": "0", "RUNNER_LIMIT_APP_EXEC_PRO": "50"}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultLimits
	want.Global, want.AppExecPro = 0, 50
	if cfg.Env != "stg" || cfg.Limits != want {
		t.Fatalf("%+v", cfg)
	}
	if DefaultLimits.Global != 10000 || DefaultLimits.RecipientPerApp != 20 || DefaultLimits.RecipientTotal != 100 || DefaultLimits.RunRecipientsPro != 200 || DefaultLimits.RunWritesPro != 10 {
		t.Fatalf("defaults drifted from #338: %+v", DefaultLimits)
	}
	if _, err := ConfigFromEnv(func(k string) string {
		if k == "RUNNER_LIMIT_RECIPIENT_TOTAL" {
			return "-1"
		}
		return ""
	}); err == nil {
		t.Fatal("negative must fail")
	}
}

func TestUsageLog(t *testing.T) {
	var buf bytes.Buffer
	q, _ := newQuota(nil, "test", DefaultLimits, nil)
	q.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	q.LogUsage(context.Background(), Usage{
		AppID: "app-a", CreatorUID: "alice", ActorUID: "bob", RuleID: "r1", Trigger: TriggerRecordCreated,
		Outcome: OutcomeOK, Writes: 2, NotifyRecipients: 3, RecipientsDropped: 1, Limit: ScopeRecipientApp,
	})
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["event"] != UsageEvent || m["msg"] != UsageEvent || m["app_id"] != "app-a" || m["outcome"] != "ok" || m["limit"] != ScopeRecipientApp {
		t.Fatalf("%v", m)
	}
	if m["creator_uid_hash"] != httpx.UIDHash("alice") || m["actor_uid_hash"] != httpx.UIDHash("bob") {
		t.Fatalf("hash: %v", m)
	}
	if strings.Contains(buf.String(), `"alice"`) || strings.Contains(buf.String(), `"bob"`) {
		t.Fatalf("raw uid in log: %s", buf.String())
	}
	if got, want := m["est_cost_usd"].(float64), costExecUSD+3*costRecipientUSD+2*costWriteUSD; math.Abs(got-want) > 1e-12 {
		t.Fatalf("cost %v want %v", got, want)
	}
	if EstimateCostUSD(Usage{Outcome: OutcomeSkippedQuota}) != 0 {
		t.Fatal("skipped run costs nothing")
	}
}
