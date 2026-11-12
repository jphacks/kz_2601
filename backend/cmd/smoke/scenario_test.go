package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func example(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../../../docs/dsl/examples/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeAI answers /health and /v1/generate from per-transcript results.
// status != 0 overrides the reply for every generate call.
type fakeAI struct {
	results map[string]ai.Result
	status  int
	body    string
	auths   []string
}

func (f *fakeAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		w.WriteHeader(http.StatusOK)
	case "/v1/generate":
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, f.body)
			return
		}
		var req ai.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		res, ok := f.results[req.Transcript]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		res.AppID = req.AppID
		_ = json.NewEncoder(w).Encode(res)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func happyAI(t *testing.T) *fakeAI {
	k := example(t, "kakeibo")
	return &fakeAI{results: map[string]ai.Result{
		sayKakeibo:  {Mode: "full", Version: 2, DSL: k},
		sayBigger:   {Mode: "patch", Version: 3, DSL: k, Patch: []dsl.PatchOp{{Op: "replace", Path: "/app/name", Value: json.RawMessage(`"x"`)}}},
		sayUndo:     {Mode: "restore", Version: 4, RestoredFrom: 2, DSL: k, Summary: "戻しました"},
		sayTraining: {Mode: "full", Version: 2, DSL: example(t, "training")},
		sayWalk:     {Mode: "full", Version: 2, DSL: example(t, "walk")},
	}}
}

// fakeAppconfig answers GET /dsl (version) and GET /versions (items).
func fakeAppconfig(version int, items string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/dsl"):
			_, _ = io.WriteString(w, `{"appId":"x","version":`+itoa(version)+`,"dsl":{}}`)
		case strings.HasSuffix(r.URL.Path, "/versions"):
			_, _ = io.WriteString(w, `{"items":`+items+`}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newRunner(aiURL, cfgURL string) *runner {
	r := &runner{ai: aiURL, appconfig: cfgURL, token: "Bearer tok", tokenKind: "flag", http: http.DefaultClient}
	if cfgURL != "" {
		r.store = appconfigclient.NewHTTP(cfgURL)
	}
	return r
}

func counts(steps []step) (pass, warn, fail int) {
	for _, s := range steps {
		switch {
		case !s.OK:
			fail++
		case s.Warn:
			warn++
		default:
			pass++
		}
	}
	return
}

const restoreItems = `[{"version":4,"source":{"kind":"restore","restoredFrom":2}},{"version":3}]`

// The three demo scenarios pass against an ai that behaves as documented.
func TestScenariosPass(t *testing.T) {
	f := happyAI(t)
	aiSrv := httptest.NewServer(f)
	defer aiSrv.Close()
	cfg := httptest.NewServer(fakeAppconfig(1, restoreItems))
	defer cfg.Close()

	r := newRunner(aiSrv.URL, cfg.URL)
	ctx := context.Background()
	r.health(ctx)
	r.kakeibo(ctx, "smoke-1")
	r.training(ctx, "smoke-1-training")
	r.walk(ctx, "smoke-1-walk")

	if p, w, fl := counts(r.steps); fl != 0 || w != 0 || p != 9 {
		t.Fatalf("pass/warn/fail = %d/%d/%d, want 9/0/0: %+v", p, w, fl, r.steps)
	}
	for _, a := range f.auths {
		if a != "Bearer tok" {
			t.Fatalf("generate sent Authorization %q", a)
		}
	}
	if strings.Join(r.created, ",") != "smoke-1,smoke-1-training,smoke-1-walk" {
		t.Fatalf("tracked = %v", r.created)
	}
	if r.print(false) != 0 {
		t.Fatal("print must report 0 failures")
	}
}

// Each broken ai answer becomes a FAIL step and stops the scenario.
func TestKakeiboFailures(t *testing.T) {
	k := example(t, "kakeibo")
	other := example(t, "training")
	cases := []struct {
		name   string
		mutate func(f *fakeAI)
		items  string
		steps  int // steps recorded (A-0 + generate steps + A-4)
		want   string
	}{
		{"500", func(f *fakeAI) { f.status, f.body = 500, `{"error":"boom"}` }, restoreItems, 2, "status=500"},
		{"bad json", func(f *fakeAI) { f.status, f.body = 200, "not json" }, restoreItems, 2, "bad JSON"},
		{"invalid dsl", func(f *fakeAI) {
			f.results[sayKakeibo] = ai.Result{Mode: "full", Version: 2, DSL: json.RawMessage(`{"schemaVersion":1}`)}
		}, restoreItems, 2, "dsl.Parse"},
		{"not full", func(f *fakeAI) { f.results[sayKakeibo] = ai.Result{Mode: "patch", Version: 2, DSL: k} }, restoreItems, 2, "want full"},
		{"edit skips a version", func(f *fakeAI) { f.results[sayBigger] = ai.Result{Mode: "patch", Version: 9, DSL: k} }, restoreItems, 3, "want 3"},
		{"undo from wrong version", func(f *fakeAI) {
			f.results[sayUndo] = ai.Result{Mode: "restore", Version: 4, RestoredFrom: 1, DSL: k}
		}, restoreItems, 4, "want restore from 2"},
		{"undo wrong version", func(f *fakeAI) {
			f.results[sayUndo] = ai.Result{Mode: "restore", Version: 7, RestoredFrom: 2, DSL: k}
		}, restoreItems, 4, "version 7, want 4"},
		{"undo different dsl", func(f *fakeAI) {
			f.results[sayUndo] = ai.Result{Mode: "restore", Version: 4, RestoredFrom: 2, DSL: other}
		}, restoreItems, 4, "differs"},
		{"no versions", nil, `[]`, 5, "no versions"},
		{"latest not restore", nil, `[{"version":4,"source":{"kind":"ai"}}]`, 5, "not source.kind=restore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := happyAI(t)
			if tc.mutate != nil {
				tc.mutate(f)
			}
			aiSrv := httptest.NewServer(f)
			defer aiSrv.Close()
			cfg := httptest.NewServer(fakeAppconfig(1, tc.items))
			defer cfg.Close()
			r := newRunner(aiSrv.URL, cfg.URL)
			r.kakeibo(context.Background(), "smoke-1")
			if len(r.steps) != tc.steps {
				t.Fatalf("steps = %d, want %d: %+v", len(r.steps), tc.steps, r.steps)
			}
			last := r.steps[len(r.steps)-1]
			if last.OK || !strings.Contains(last.Note, tc.want) {
				t.Fatalf("last step = %+v, want FAIL with %q", last, tc.want)
			}
		})
	}
}

// appconfig on an older version or on another store: A-0 FAIL / A-4 WARN.
func TestKakeiboAppconfigMismatch(t *testing.T) {
	aiSrv := httptest.NewServer(happyAI(t))
	defer aiSrv.Close()

	cfg := httptest.NewServer(fakeAppconfig(3, `[{"version":1}]`))
	defer cfg.Close()
	r := newRunner(aiSrv.URL, cfg.URL)
	r.kakeibo(context.Background(), "smoke-1")
	if r.steps[0].OK || !strings.Contains(r.steps[0].Note, "version 1, got 3") {
		t.Fatalf("A-0 = %+v", r.steps[0])
	}
	if last := r.steps[len(r.steps)-1]; !last.Warn || !strings.Contains(last.Note, "in-memory store") {
		t.Fatalf("A-4 = %+v, want WARN", last)
	}

	// appconfig down: A-0 and A-4 fail, the ai steps still run
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	r = newRunner(aiSrv.URL, downURL)
	r.kakeibo(context.Background(), "smoke-2")
	if p, _, fl := counts(r.steps); fl != 2 || p != 3 {
		t.Fatalf("steps %+v", r.steps)
	}

	// without -appconfig only the three ai steps run
	r = newRunner(aiSrv.URL, "")
	r.kakeibo(context.Background(), "smoke-3")
	if p, w, fl := counts(r.steps); p != 3 || w != 0 || fl != 0 {
		t.Fatalf("steps %+v", r.steps)
	}
}

func TestTrainingAndWalkChecks(t *testing.T) {
	f := happyAI(t)
	f.results[sayTraining] = ai.Result{Mode: "full", Version: 2, DSL: example(t, "kakeibo")} // no daily 08:xx notification
	f.results[sayWalk] = ai.Result{Mode: "full", Version: 2, DSL: example(t, "training")}    // no photoField
	aiSrv := httptest.NewServer(f)
	defer aiSrv.Close()
	r := newRunner(aiSrv.URL, "")
	r.training(context.Background(), "t")
	if r.steps[0].OK || !strings.Contains(r.steps[0].Note, "no daily 08:xx") {
		t.Fatalf("training = %+v", r.steps[0])
	}
	r.walk(context.Background(), "w")
	if r.steps[1].OK || r.steps[1].Note != "no photoField" {
		t.Fatalf("walk without photoField = %+v", r.steps[1])
	}

	// a library-only photoField cannot take the walk photo
	f.results[sayWalk] = ai.Result{Mode: "full", Version: 2, DSL: json.RawMessage(strings.ReplaceAll(string(example(t, "walk")), `"camera"`, `"library"`))}
	r.walk(context.Background(), "w")
	if r.steps[2].OK || !strings.Contains(r.steps[2].Note, "source=library") {
		t.Fatalf("walk library = %+v", r.steps[2])
	}
}

func TestAuthProbe(t *testing.T) {
	for _, tc := range []struct {
		status   int
		fallback bool
		ok, warn bool
	}{
		{http.StatusUnauthorized, true, true, true},
		{http.StatusInternalServerError, true, false, false},
		{http.StatusOK, false, true, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }))
		r := newRunner("", srv.URL)
		got := r.authProbe(context.Background(), "a")
		srv.Close()
		s := r.steps[0]
		if got != tc.fallback || s.OK != tc.ok || s.Warn != tc.warn {
			t.Fatalf("status %d: fallback=%v step=%+v", tc.status, got, s)
		}
		if tc.status == http.StatusOK && (len(r.created) != 1 || r.created[0] != "a-probe") {
			t.Fatalf("probe app not tracked: %v", r.created)
		}
		if tc.status != http.StatusOK && len(r.created) != 0 {
			t.Fatalf("rejected probe tracked: %v", r.created)
		}
	}

	// unreachable appconfig is a FAIL and a fallback
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	r := newRunner("", u)
	if !r.authProbe(context.Background(), "a") || r.steps[0].OK {
		t.Fatalf("unreachable: %+v", r.steps)
	}

	// no -appconfig: nothing to probe
	r = newRunner("", "")
	if r.authProbe(context.Background(), "a") || len(r.steps) != 0 {
		t.Fatalf("no appconfig: %+v", r.steps)
	}
}

func TestUnauthenticatedGenerateIs401(t *testing.T) {
	for _, tc := range []struct {
		status   int
		ok, warn bool
	}{
		{http.StatusUnauthorized, true, false},
		{http.StatusOK, true, true}, // ai accepts anonymous calls: WARN, not PASS
	} {
		f := &fakeAI{status: tc.status, body: "{}"}
		srv := httptest.NewServer(f)
		r := newRunner(srv.URL, "")
		r.unauthenticatedGenerateIs401(context.Background(), "a")
		srv.Close()
		if s := r.steps[0]; s.OK != tc.ok || s.Warn != tc.warn {
			t.Fatalf("status %d: %+v", tc.status, s)
		}
		if len(f.auths) != 1 || f.auths[0] != "" {
			t.Fatalf("the probe must not send a token: %q", f.auths)
		}
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	r := newRunner(u, "")
	r.unauthenticatedGenerateIs401(context.Background(), "a")
	if r.steps[0].OK {
		t.Fatalf("unreachable ai must FAIL: %+v", r.steps)
	}
}

func TestHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer bad.Close()
	r := newRunner(ok.URL, bad.URL)
	r.health(context.Background())
	if len(r.steps) != 2 || !r.steps[0].OK || r.steps[1].OK || !strings.Contains(r.steps[1].Note, "status=503") {
		t.Fatalf("steps %+v", r.steps)
	}
	r = newRunner(ok.URL, "")
	r.health(context.Background())
	if len(r.steps) != 1 {
		t.Fatalf("empty appconfig must be skipped: %+v", r.steps)
	}
}

// rewrite sends every request to srv (no live identitytoolkit call).
type rewrite struct{ srv *url.URL }

func (rw rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = rw.srv.Scheme, rw.srv.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestResolveToken(t *testing.T) {
	r := newRunner("", "")
	r.resolveToken(context.Background(), "Bearer abc", "")
	if r.token != "Bearer abc" || r.tokenKind != "flag" {
		t.Fatalf("flag token = %q %q", r.token, r.tokenKind)
	}
	r.resolveToken(context.Background(), "abc", "")
	if r.token != "Bearer abc" {
		t.Fatalf("bare flag token = %q", r.token)
	}

	t.Setenv("SMOKE_UID", "")
	r.resolveToken(context.Background(), "", "")
	if r.token != "Bearer dev-smoke" || r.tokenKind != "dev" {
		t.Fatalf("dev token = %q %q", r.token, r.tokenKind)
	}
	t.Setenv("SMOKE_UID", "u1")
	r.resolveToken(context.Background(), "", "")
	if r.token != "Bearer u1" {
		t.Fatalf("SMOKE_UID token = %q", r.token)
	}

	key := strings.Repeat("k", 39)
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotKey = req.URL.Query().Get("key")
		if req.URL.Path != "/v1/accounts:signUp" || req.Method != http.MethodPost {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, `{"idToken":"`+strings.Repeat("i", 20)+`","localId":"anon"}`)
	}))
	defer srv.Close()
	su, _ := url.Parse(srv.URL)
	r = newRunner("", "")
	r.http = &http.Client{Transport: rewrite{su}}
	r.resolveToken(context.Background(), "", key)
	if gotKey != key || r.tokenKind != "firebase" || r.token != "Bearer "+strings.Repeat("i", 20) {
		t.Fatalf("firebase token = %q %q key=%q", r.token, r.tokenKind, gotKey)
	}
	if len(r.steps) != 1 || !r.steps[0].OK || r.steps[0].Note != "uid=anon" {
		t.Fatalf("steps %+v", r.steps)
	}
}

func TestFirebaseAnonymousErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{400, `{"error":{"message":"ADMIN_ONLY_OPERATION"}}`, "400 ADMIN_ONLY_OPERATION"},
		{500, "plain failure", "500 plain failure"},
		{200, `{"localId":"x"}`, "200"}, // no idToken
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		su, _ := url.Parse(srv.URL)
		_, _, err := firebaseAnonymous(context.Background(), &http.Client{Transport: rewrite{su}}, "k")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("status %d: err = %v, want %q", tc.status, err, tc.want)
		}
	}

	// a failed sign-up falls back to the dev token with a WARN step
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	su, _ := url.Parse(srv.URL)
	t.Setenv("SMOKE_UID", "")
	r := newRunner("", "")
	r.http = &http.Client{Transport: rewrite{su}}
	r.resolveToken(context.Background(), "", "k")
	if r.tokenKind != "dev" || len(r.steps) != 1 || !r.steps[0].Warn {
		t.Fatalf("fallback: kind=%q steps=%+v", r.tokenKind, r.steps)
	}

	// transport error
	failing := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, _, err := firebaseAnonymous(context.Background(), failing, "k"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("transport error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOverlaps(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{
		{Components: []dsl.Component{{Area: "A1:B2"}, {Area: "B2:C3"}, {Area: "D1:F1"}, {}}},
		{Components: []dsl.Component{{Area: "A1:F2"}, {Area: "not-an-area"}}},
	}}
	if n := overlaps(app); n != 2 { // one overlapping pair + one unparsable area
		t.Fatalf("overlaps = %d, want 2", n)
	}
	if n := overlaps(&dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{{Area: "A1:A1"}, {Area: "B1:B1"}}}}}); n != 0 {
		t.Fatalf("disjoint overlaps = %d", n)
	}
}

func TestPhotoSource(t *testing.T) {
	nested := &dsl.App{Screens: []dsl.Screen{
		{Components: []dsl.Component{{Type: "text"}}},
		{Components: []dsl.Component{{Type: "vstack", Children: []dsl.Component{{Type: dsl.CompPhotoField, Source: "library"}}}}},
	}}
	if s := photoSource(nested); s != "library" {
		t.Fatalf("nested = %q", s)
	}
	def := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{{Type: dsl.CompPhotoField}}}}}
	if s := photoSource(def); s != "default" {
		t.Fatalf("no source = %q", s)
	}
	if s := photoSource(&dsl.App{}); s != "" {
		t.Fatalf("none = %q", s)
	}
}

func TestCanon(t *testing.T) {
	if string(canon(json.RawMessage(`{ "b":1, "a":[1, 2] }`))) != `{"a":[1,2],"b":1}` {
		t.Fatal("canon must normalise key order and spacing")
	}
	if string(canon(json.RawMessage(`{bad`))) != `{bad` {
		t.Fatal("canon must return invalid JSON unchanged")
	}
}

func TestReportingHelpers(t *testing.T) {
	if nonZero(0) != "" || nonZero(12) != "12" {
		t.Fatal("nonZero")
	}
	if orDash("") != "-" || orDash("x") != "x" {
		t.Fatal("orDash")
	}
	if cell("a|b\nc") != `a\|b c` {
		t.Fatalf("cell = %q", cell("a|b\nc"))
	}
	if trunc("abc", 3) != "abc" || trunc("abcd", 3) != "abc…" {
		t.Fatal("trunc ascii")
	}
}

// trunc must not split a multi-byte rune (Japanese error bodies end up in the
// Markdown table).
func TestTruncKeepsUTF8(t *testing.T) {
	s := strings.Repeat("あ", 50) // 150 bytes
	for _, n := range []int{1, 2, 100, 120, 149} {
		got := trunc(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("trunc(%d) produced invalid UTF-8: %q", n, got)
		}
		if len(strings.TrimSuffix(got, "…")) > n {
			t.Fatalf("trunc(%d) kept %d bytes", n, len(got))
		}
	}
}

func TestPrintFallbackCounts(t *testing.T) {
	r := &runner{steps: []step{{Name: "a", OK: true}, {Name: "b", OK: true, Warn: true}, {Name: "c"}, {Name: "d"}}}
	if n := r.print(true); n != 2 {
		t.Fatalf("failed = %d, want 2", n)
	}
}
