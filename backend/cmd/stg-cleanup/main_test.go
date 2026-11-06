package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestGuard(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		allowProd bool
		ok        bool
	}{
		{"no project", map[string]string{}, true, false},
		{"prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, false, false},
		{"prod db with flag", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, true, true},
		{"prod db named explicitly", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_DATABASE": "(default)"}, false, false},
		{"configured prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": "tailorit-prod", "PROD_FIRESTORE_PROJECT": "tailorit-prod"}, false, false},
		{"stg named db", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_DATABASE": "stg"}, false, true},
		{"stg project", map[string]string{"GOOGLE_CLOUD_PROJECT": "tailorit-stg"}, false, true},
		{"APP_ENV=stg on prod db still needs the shared override", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "stg"}, true, false},
		{"emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := envOf(c.env)
			err := guard(fsenv.FromEnv(getenv), c.allowProd, getenv)
			if (err == nil) != c.ok {
				t.Fatalf("want ok=%v, got %v", c.ok, err)
			}
		})
	}
}

func TestReadUIDs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "uids.txt")
	if err := os.WriteFile(file, []byte("# testers\nuidB  # iPhone\n\nuidC\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readUIDs(" uidA ,", file)
	if err != nil || strings.Join(got, ",") != "uidA,uidB,uidC" {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range []string{"a,a", "users/x", "a b", "*", strings.Repeat("x", 129)} {
		if _, err := readUIDs(bad, ""); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	many := make([]string, maxUIDs+1)
	for i := range many {
		many[i] = "u" + strconv.Itoa(i)
	}
	if _, err := readUIDs(strings.Join(many, ","), ""); err == nil {
		t.Error("more than maxUIDs accepted")
	}
}

func TestValidate(t *testing.T) {
	if validate(options{expect: -1}) == nil {
		t.Error("no selector accepted")
	}
	if validate(options{smoke: true, apply: true, expect: -1}) == nil {
		t.Error("-apply without -expect accepted")
	}
	if err := validate(options{smoke: true, apply: true, expect: 0}); err != nil {
		t.Error(err)
	}
}

func TestSmokeAppIDRe(t *testing.T) {
	for _, id := range []string{"smoke-1730000000", "smoke-1730000000-training", "smoke-1730000000-walk", "smoke-1730000000-probe", "smoke-1730000000-noauth"} {
		if !store.SmokeAppIDRe.MatchString(id) {
			t.Errorf("%s not matched", id)
		}
	}
	for _, id := range []string{"smoke-", "smoke-notes", "smoke-1730000000-mine", "my-smoke-1730000000", "Smoke-1730000000", "smoke-1730000000x", "smoke-123"} {
		if store.SmokeAppIDRe.MatchString(id) {
			t.Errorf("%s matched", id)
		}
	}
	for _, id := range []string{"stg_2026-11-01_uid_abc", "stg_2026-11-01_ip_0a1b2c3d", "stg_2026-11-01_global"} {
		if !store.StgRateLimitIDRe.MatchString(id) {
			t.Errorf("%s not matched", id)
		}
	}
	for _, id := range []string{"prod_2026-11-01_global", "local_2026-11-01_global", "stg_notes", "stg_2026-11-01_other", "xstg_2026-11-01_global"} {
		if store.StgRateLimitIDRe.MatchString(id) {
			t.Errorf("%s matched", id)
		}
	}
}

// emulatorFS opens a fresh emulator project per test, so other tests' data
// (and other packages running at the same time) never shows up as targets.
func emulatorFS(t *testing.T) *store.Firestore {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	project := "demo-sc-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	f, err := store.NewFirestore(t.Context(), project, "(default)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

type world struct {
	smokeUID, testUID, realUID string
	smokeApps                  []string // in smokeUID's tree
	realSmokeLike              string   // real user's app that only looks like smoke
}

func seed(t *testing.T, f *store.Firestore) world {
	t.Helper()
	ctx := context.Background()
	w := world{smokeUID: "anonSmoke1", testUID: "stgTester1", realUID: "realUser1",
		smokeApps: []string{"smoke-1730000000", "smoke-1730000000-walk", "smoke-1730000000-probe"}, realSmokeLike: "smoke-notes"}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ensure := func(uid, app string) {
		t.Helper()
		_, _, err := f.EnsureApp(ctx, uid, app, json.RawMessage(`{}`), "x")
		must(err)
	}
	record := func(uid, app string) {
		t.Helper()
		_, _, err := f.CreateRecord(ctx, store.PrivateRecords(uid, app, "item"), store.Record{ID: "r1", Data: map[string]any{"v": 1}})
		must(err)
	}
	for _, app := range w.smokeApps {
		ensure(w.smokeUID, app)
	}
	record(w.smokeUID, w.smokeApps[0])
	_, err := f.ClaimApp(ctx, w.smokeUID, w.smokeApps[0])
	must(err)
	_, err = f.ClaimApp(ctx, w.smokeUID, "smoke-1730000000-noauth") // owner entry without an app
	must(err)
	// a real user opened a smoke appId (GET /dsl seeds a copy): that copy is smoke too
	ensure(w.realUID, "smoke-1730000000")

	// the test user
	ensure(w.testUID, "TEST-APP")
	record(w.testUID, "TEST-APP")
	_, err = f.ClaimApp(ctx, w.testUID, "TEST-APP")
	must(err)
	must(f.CreateShare(ctx, store.Share{Token: "tok-test", AppID: "TEST-APP", OwnerUID: w.testUID, DSL: json.RawMessage(`{}`), CreatedAt: time.Now()}))
	_, err = f.CreateFeedback(ctx, store.Feedback{UID: w.testUID, Kind: store.FeedbackBug, Message: "t", ConsentVersion: "v1"}, 0)
	must(err)
	c := f.Client()
	_, err = c.Collection("users").Doc(w.testUID).Collection("entitlements").Doc("pro").Set(ctx, map[string]any{"active": true})
	must(err)
	_, err = c.Collection("appstore_transactions").Doc("otid-test").Set(ctx, map[string]any{"uid": w.testUID})
	must(err)

	// the real user: must survive everything
	ensure(w.realUID, "REAL-APP")
	record(w.realUID, "REAL-APP")
	ensure(w.realUID, w.realSmokeLike)
	_, err = f.ClaimApp(ctx, w.realUID, "REAL-APP")
	must(err)
	_, err = f.ClaimApp(ctx, w.realUID, w.realSmokeLike)
	must(err)
	must(f.CreateShare(ctx, store.Share{Token: "tok-real", AppID: "REAL-APP", OwnerUID: w.realUID, DSL: json.RawMessage(`{}`), CreatedAt: time.Now()}))
	_, err = f.CreateFeedback(ctx, store.Feedback{UID: w.realUID, Kind: store.FeedbackBug, Message: "r", ConsentVersion: "v1"}, 0)
	must(err)
	_, err = c.Collection("appstore_transactions").Doc("otid-real").Set(ctx, map[string]any{"uid": w.realUID})
	must(err)
	for _, id := range []string{"stg_2026-11-01_uid_stgTester1", "stg_2026-11-01_global", "prod_2026-11-01_uid_realUser1", "prod_2026-11-01_global", "stg_notes"} {
		_, err = c.Collection("rateLimits").Doc(id).Set(ctx, map[string]any{"count": 1})
		must(err)
	}
	return w
}

// snapshot is every existing document path in the project.
func snapshot(t *testing.T, f *store.Firestore) map[string]bool {
	t.Helper()
	ctx := context.Background()
	out := map[string]bool{}
	var walk func(cols *firestore.CollectionIterator)
	walk = func(cols *firestore.CollectionIterator) {
		for {
			col, err := cols.Next()
			if errors.Is(err, iterator.Done) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			refs, err := col.DocumentRefs(ctx).GetAll()
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range refs {
				if s, err := r.Get(ctx); err == nil && s.Exists() {
					_, rel, _ := strings.Cut(r.Path, "/documents/")
					out[rel] = true
				}
				walk(r.Collections(ctx))
			}
		}
	}
	walk(f.Client().Collections(ctx))
	return out
}

func runCleanup(t *testing.T, f *store.Firestore, o options) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := run(context.Background(), f, entitlement.NewFirestore(f.Client()), o, &buf)
	return code, buf.String()
}

func TestFirestoreDryRunDeletesNothing(t *testing.T) {
	f := emulatorFS(t)
	w := seed(t, f)
	before := snapshot(t, f)
	code, out := runCleanup(t, f, options{smoke: true, rates: true, uids: []string{w.testUID}, expect: -1})
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	// 3 smoke apps of the smoke user + the real user's smoke copy + 2 app_owners + 2 stg counters + 1 test user
	for _, want := range []string{"== targets: 9", "-apply -expect=9", "stg rate-limit counters: 2", "app smoke-1730000000-probe", "app_owners/smoke-1730000000-noauth", "uid " + w.testUID, "has Pro entitlement"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, w.realSmokeLike) || strings.Contains(out, "REAL-APP") || strings.Contains(out, w.smokeUID) {
		t.Errorf("output names a non-target or a discovered uid\n%s", out)
	}
	if after := snapshot(t, f); len(after) != len(before) {
		t.Fatalf("dry run changed data: %d -> %d docs", len(before), len(after))
	}
}

func TestFirestoreApplyWrongExpectDeletesNothing(t *testing.T) {
	f := emulatorFS(t)
	w := seed(t, f)
	before := snapshot(t, f)
	code, out := runCleanup(t, f, options{smoke: true, rates: true, uids: []string{w.testUID}, apply: true, expect: 8})
	if code != 1 || !strings.Contains(out, "-expect=8 but 9 targets") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if after := snapshot(t, f); len(after) != len(before) {
		t.Fatalf("data changed: %d -> %d docs", len(before), len(after))
	}
}

func TestFirestoreApplyDeletesOnlyTargets(t *testing.T) {
	f := emulatorFS(t)
	w := seed(t, f)
	before := snapshot(t, f)
	testFeedback := map[string]bool{}
	for p := range before {
		if strings.HasPrefix(p, "feedback/") && uidField(t, f, p) == w.testUID {
			testFeedback[p] = true
		}
	}
	if len(testFeedback) != 1 {
		t.Fatalf("seeded feedback: %v", testFeedback)
	}
	code, out := runCleanup(t, f, options{smoke: true, rates: true, uids: []string{w.testUID}, apply: true, expect: 9})
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	after := snapshot(t, f)
	for p := range before {
		target := strings.Contains(p, "/smoke-1730000000") || strings.HasPrefix(p, "app_owners/smoke-1730000000") ||
			strings.Contains(p, w.testUID) || p == "shares/tok-test" || p == "app_owners/TEST-APP" || strings.HasPrefix(p, "rateLimits/stg_2026") || p == "appstore_transactions/otid-test" ||
			testFeedback[p]
		if target && after[p] {
			t.Errorf("target left: %s", p)
		}
		if !target && !after[p] {
			t.Errorf("non-target deleted: %s", p)
		}
	}
	for p := range after {
		if !before[p] {
			t.Errorf("new document: %s", p)
		}
	}
	for _, keep := range []string{"users/realUser1/apps/REAL-APP", "users/realUser1/apps/smoke-notes", "app_owners/REAL-APP", "app_owners/smoke-notes", "shares/tok-real", "appstore_transactions/otid-real", "rateLimits/prod_2026-11-01_uid_realUser1", "rateLimits/prod_2026-11-01_global", "rateLimits/stg_notes"} {
		if !after[keep] {
			t.Errorf("%s deleted", keep)
		}
	}
	// the test user can start over: no tombstone left
	if gone, err := f.UserDeleted(context.Background(), w.testUID); err != nil || gone {
		t.Errorf("test user still marked: %v %v", gone, err)
	}

	// a second run finds nothing
	code, out = runCleanup(t, f, options{smoke: true, rates: true, expect: -1})
	if code != 0 || !strings.Contains(out, "== targets: 0") {
		t.Fatalf("rerun: code %d\n%s", code, out)
	}
}

func TestFirestoreUIDsOnlyLeavesSmoke(t *testing.T) {
	f := emulatorFS(t)
	w := seed(t, f)
	code, out := runCleanup(t, f, options{uids: []string{w.testUID}, apply: true, expect: 1})
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	after := snapshot(t, f)
	if !after["users/anonSmoke1/apps/smoke-1730000000"] || !after["app_owners/smoke-1730000000-noauth"] {
		t.Fatal("smoke data deleted without -smoke")
	}
	if !after["rateLimits/stg_2026-11-01_global"] {
		t.Fatal("counters deleted without -stg-ratelimits")
	}
	if after["users/stgTester1/apps/TEST-APP"] {
		t.Fatal("test user data left")
	}
}

// uidField reads the uid field of path ("" when absent).
func uidField(t *testing.T, f *store.Firestore, path string) string {
	t.Helper()
	s, err := f.Client().Doc(path).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.Data()["uid"].(string)
	return uid
}
