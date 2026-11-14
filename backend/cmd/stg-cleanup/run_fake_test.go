package main

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// fakeStore is a cleanupStore without Firestore: run's plan, -expect gate
// and failure handling are checked without the emulator.
type fakeStore struct {
	smoke    store.SmokeData
	rates    []string
	users    map[string]store.TestUserSummary
	findErr  map[string]error // "smoke" | "rates" | "summarize"
	purgeErr map[string]error // "app:<id>" | "owner:<id>" | "rates" | "user:<uid>" | "tree:<uid>"
	calls    []string
	onPurge  func(appID string) // called inside PurgeSmokeApp (to cancel midway)
}

func (f *fakeStore) FindSmokeData(context.Context) (store.SmokeData, error) {
	return f.smoke, f.findErr["smoke"]
}

func (f *fakeStore) FindStgRateLimits(context.Context) ([]string, error) {
	return f.rates, f.findErr["rates"]
}

func (f *fakeStore) SummarizeTestUser(_ context.Context, uid string) (store.TestUserSummary, error) {
	if err := f.findErr["summarize"]; err != nil {
		return store.TestUserSummary{}, err
	}
	if s, ok := f.users[uid]; ok {
		return s, nil
	}
	return store.TestUserSummary{UID: uid, Docs: map[string]int{}}, nil
}

func (f *fakeStore) PurgeSmokeApp(ctx context.Context, uid, appID string) (store.PurgeResult, error) {
	f.calls = append(f.calls, "app:"+appID)
	if f.onPurge != nil {
		f.onPurge(appID)
		if err := ctx.Err(); err != nil {
			return store.PurgeResult{}, err
		}
	}
	return store.PurgeResult{}, f.purgeErr["app:"+appID]
}

func (f *fakeStore) DeleteSmokeOwner(_ context.Context, appID string) error {
	f.calls = append(f.calls, "owner:"+appID)
	return f.purgeErr["owner:"+appID]
}

func (f *fakeStore) DeleteStgRateLimits(_ context.Context, ids []string) error {
	f.calls = append(f.calls, "rates")
	return f.purgeErr["rates"]
}

func (f *fakeStore) PurgeTestUser(_ context.Context, uid string) (store.AccountPurgeResult, error) {
	f.calls = append(f.calls, "user:"+uid)
	return store.AccountPurgeResult{Apps: 1}, f.purgeErr["user:"+uid]
}

func (f *fakeStore) DeleteUserTree(_ context.Context, uid string) (int, error) {
	f.calls = append(f.calls, "tree:"+uid)
	return 2, f.purgeErr["tree:"+uid]
}

type fakeEnts struct {
	err   map[string]error
	calls []string
}

func (e *fakeEnts) PurgeUser(_ context.Context, uid string) (int, error) {
	e.calls = append(e.calls, uid)
	return 1, e.err[uid]
}

const smokeUID = "smokeOwnerUid123"

func newFake() *fakeStore {
	return &fakeStore{
		smoke: store.SmokeData{
			Apps: []store.SmokeApp{
				{UID: smokeUID, AppID: "smoke-1700000000", Docs: map[string]int{"apps": 1, "versions": 2}},
				{UID: smokeUID, AppID: "smoke-1700000001-walk", Docs: map[string]int{"apps": 1}},
			},
			Owners: []string{"smoke-1700000000"},
		},
		rates: []string{"stg_uid_secret_1", "stg_ip_2"},
		users: map[string]store.TestUserSummary{
			"tester1": {UID: "tester1", Exists: true, Docs: map[string]int{"apps": 3}},
			"paid1":   {UID: "paid1", Exists: true, Docs: map[string]int{"entitlements": 1}},
		},
	}
}

// all targets: 2 apps + 1 owner + 2 counters + 3 uids
var allOpts = options{smoke: true, rates: true, uids: []string{"tester1", "paid1", "ghost"}}

const allTargets = 8

func TestRunDryRunPlansWithoutDeleting(t *testing.T) {
	st, ents := newFake(), &fakeEnts{}
	var out bytes.Buffer
	if code := run(context.Background(), st, ents, allOpts, &out); code != 0 {
		t.Fatalf("code = %d, out:\n%s", code, out.String())
	}
	if len(st.calls) != 0 || len(ents.calls) != 0 {
		t.Fatalf("dry run deleted: %v %v", st.calls, ents.calls)
	}
	s := out.String()
	for _, want := range []string{
		"== smoke apps: 2, app_owners: 1",
		"docs=3 [apps=1 versions=2]",
		"== stg rate-limit counters: 2",
		"== test users: 3",
		"uid ghost status=\"\" docs=0 [] (no data: typo?)",
		"(has Pro entitlement / App Store binding",
		"== targets: 8 (documents counted: 11)",
		"add -apply -expect=8",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	// discovered uids and counter ids (which embed uids) are never printed
	for _, leak := range []string{smokeUID, "stg_uid_secret_1"} {
		if strings.Contains(s, leak) {
			t.Errorf("output leaks %q:\n%s", leak, s)
		}
	}
	if !strings.Contains(s, uidHash(smokeUID)) {
		t.Errorf("output lacks the smoke uid hash:\n%s", s)
	}
}

func TestRunApplyNeedsMatchingExpect(t *testing.T) {
	for _, expect := range []int{0, allTargets - 1, allTargets + 1} {
		st, ents := newFake(), &fakeEnts{}
		o := allOpts
		o.apply, o.expect = true, expect
		var out bytes.Buffer
		if code := run(context.Background(), st, ents, o, &out); code != 1 {
			t.Errorf("expect=%d: code = %d, want 1", expect, code)
		}
		if len(st.calls) != 0 || len(ents.calls) != 0 {
			t.Errorf("expect=%d: deleted %v %v", expect, st.calls, ents.calls)
		}
		if !strings.Contains(out.String(), "run the dry run again") {
			t.Errorf("expect=%d: output:\n%s", expect, out.String())
		}
	}
}

func TestRunApplyDeletesEveryTarget(t *testing.T) {
	st, ents := newFake(), &fakeEnts{}
	o := allOpts
	o.apply, o.expect = true, allTargets
	var out bytes.Buffer
	if code := run(context.Background(), st, ents, o, &out); code != 0 {
		t.Fatalf("code = %d, out:\n%s", code, out.String())
	}
	want := []string{
		"app:smoke-1700000000", "app:smoke-1700000001-walk", "owner:smoke-1700000000", "rates",
		"user:tester1", "tree:tester1", "user:paid1", "tree:paid1", "user:ghost", "tree:ghost",
	}
	if !slices.Equal(st.calls, want) {
		t.Fatalf("calls = %v, want %v", st.calls, want)
	}
	if !slices.Equal(ents.calls, []string{"tester1", "paid1", "ghost"}) {
		t.Fatalf("entitlement purges = %v", ents.calls)
	}
	if !strings.Contains(out.String(), "== done: failed=0") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunFindErrorsStopBeforeDeleting(t *testing.T) {
	for _, which := range []string{"smoke", "rates", "summarize"} {
		t.Run(which, func(t *testing.T) {
			st, ents := newFake(), &fakeEnts{}
			st.findErr = map[string]error{which: errors.New("boom")}
			o := allOpts
			o.apply, o.expect = true, allTargets
			var out bytes.Buffer
			if code := run(context.Background(), st, ents, o, &out); code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			if len(st.calls) != 0 || len(ents.calls) != 0 {
				t.Fatalf("deleted after a failed lookup: %v %v", st.calls, ents.calls)
			}
			if !strings.HasPrefix(out.String(), "ERROR ") {
				t.Fatalf("output:\n%s", out.String())
			}
		})
	}
}

func TestRunApplyFailuresContinueAndExitNonZero(t *testing.T) {
	tests := []struct {
		name       string
		storeErr   map[string]error
		entErr     map[string]error
		wantCalls  []string
		wantFailed string
	}{
		{
			name:     "one smoke app fails",
			storeErr: map[string]error{"app:smoke-1700000000": errors.New("x")},
			wantCalls: []string{
				"app:smoke-1700000000", "app:smoke-1700000001-walk", "owner:smoke-1700000000", "rates",
				"user:tester1", "tree:tester1", "user:paid1", "tree:paid1", "user:ghost", "tree:ghost",
			},
			wantFailed: "FAIL app smoke-1700000000",
		},
		{
			name:       "owner and counters fail",
			storeErr:   map[string]error{"owner:smoke-1700000000": errors.New("x"), "rates": errors.New("y")},
			wantFailed: "== done: failed=2",
		},
		{
			// an entitlement failure leaves the rest of that uid's tree for a rerun
			name:   "entitlement purge fails",
			entErr: map[string]error{"paid1": errors.New("x")},
			wantCalls: []string{
				"app:smoke-1700000000", "app:smoke-1700000001-walk", "owner:smoke-1700000000", "rates",
				"user:tester1", "tree:tester1", "user:paid1", "user:ghost", "tree:ghost",
			},
			wantFailed: "FAIL uid paid1",
		},
		{
			name:     "account purge fails",
			storeErr: map[string]error{"user:tester1": errors.New("x")},
			wantCalls: []string{
				"app:smoke-1700000000", "app:smoke-1700000001-walk", "owner:smoke-1700000000", "rates",
				"user:tester1", "user:paid1", "tree:paid1", "user:ghost", "tree:ghost",
			},
			wantFailed: "FAIL uid tester1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, ents := newFake(), &fakeEnts{err: tt.entErr}
			st.purgeErr = tt.storeErr
			o := allOpts
			o.apply, o.expect = true, allTargets
			var out bytes.Buffer
			if code := run(context.Background(), st, ents, o, &out); code != 1 {
				t.Fatalf("code = %d, want 1\n%s", code, out.String())
			}
			if tt.wantCalls != nil && !slices.Equal(st.calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", st.calls, tt.wantCalls)
			}
			if !strings.Contains(out.String(), tt.wantFailed) {
				t.Errorf("output lacks %q:\n%s", tt.wantFailed, out.String())
			}
		})
	}
}

func TestRunApplyInterruptedDeletesNothing(t *testing.T) {
	st, ents := newFake(), &fakeEnts{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := allOpts
	o.apply, o.expect = true, allTargets
	var out bytes.Buffer
	if code := run(ctx, st, ents, o, &out); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if len(st.calls) != 0 || len(ents.calls) != 0 {
		t.Fatalf("deleted after cancel: %v %v", st.calls, ents.calls)
	}
	if !strings.Contains(out.String(), "ERROR interrupted") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunApplyInterruptedMidwayStops(t *testing.T) {
	st, ents := newFake(), &fakeEnts{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.onPurge = func(string) { cancel() }
	o := allOpts
	o.apply, o.expect = true, allTargets
	var out bytes.Buffer
	if code := run(ctx, st, ents, o, &out); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !slices.Equal(st.calls, []string{"app:smoke-1700000000"}) || len(ents.calls) != 0 {
		t.Fatalf("calls after cancel: %v %v", st.calls, ents.calls)
	}
	if !strings.Contains(out.String(), "ERROR interrupted") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunOnlySelectedTargets(t *testing.T) {
	st := newFake()
	var out bytes.Buffer
	o := options{rates: true, apply: true, expect: 2}
	if code := run(context.Background(), st, &fakeEnts{}, o, &out); code != 0 {
		t.Fatalf("code = %d\n%s", code, out.String())
	}
	if !slices.Equal(st.calls, []string{"rates"}) {
		t.Fatalf("calls = %v, want only the counters", st.calls)
	}
}

func TestHelpers(t *testing.T) {
	if mode(true) != "APPLY" || mode(false) != "dry-run" {
		t.Errorf("mode: %q %q", mode(true), mode(false))
	}
	if n := total(nil); n != 0 {
		t.Errorf("total(nil) = %d", n)
	}
	if got := breakdown(map[string]int{"b": 2, "a": 1}); got != "[a=1 b=2]" {
		t.Errorf("breakdown = %q", got)
	}
	if got := breakdown(nil); got != "[]" {
		t.Errorf("breakdown(nil) = %q", got)
	}
	h := uidHash("u1")
	if len(h) != 12 || h != uidHash("u1") || h == uidHash("u2") || strings.Contains(h, "u1") {
		t.Errorf("uidHash = %q", h)
	}
}
