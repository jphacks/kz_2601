package appconfig

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// proSet is a ProChecker over a map (true = Pro).
type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

func newProEnv(t *testing.T, pro proSet) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithPro(pro)))}
}

func ifMatch(v int) map[string]string {
	return map[string]string{"If-Match": `"` + strconv.Itoa(v) + `"`}
}

// wantPro checks a 402 pro_required listing exactly ids.
func wantPro(t *testing.T, what string, r resp, ids ...string) {
	t.Helper()
	if r.code != http.StatusPaymentRequired || r.errCode() != "pro_required" {
		t.Fatalf("%s: want 402 pro_required, got %d %s", what, r.code, r.raw)
	}
	e, _ := r.body["error"].(map[string]any)
	d, _ := e["details"].(map[string]any)
	fs, _ := d["features"].([]any)
	if len(fs) != len(ids) {
		t.Fatalf("%s: features %v, want %v", what, fs, ids)
	}
	for i, id := range ids {
		f, _ := fs[i].(map[string]any)
		if f["id"] != id || f["title"] == "" || f["reason"] == "" {
			t.Fatalf("%s: feature %d = %v, want %s", what, i, f, id)
		}
	}
}

// #316 完了の条件: a client calling appconfig directly (a modified app) cannot
// save paid features without Pro; Pro can; an app without them stays free.
func TestProGateDSLSave(t *testing.T) {
	e := newProEnv(t, proSet{"pro": true})
	put := func(uid, app string, dsl []byte) resp {
		r := e.do(uid, http.MethodGet, app+"/dsl", nil, nil)
		return e.do(uid, http.MethodPut, app+"/dsl", putBody(dsl), ifMatch(r.version()))
	}

	if r := put("free", "/v1/apps/PG-1", kakeibo(t)); r.code != http.StatusCreated {
		t.Fatalf("no paid feature, free: %d %s", r.code, r.raw)
	}
	wantPro(t, "shared, free", put("free", "/v1/apps/PG-2", sharedKakeibo(t)), "sharedData")
	wantPro(t, "rules, free", put("free", "/v1/apps/PG-3", []byte(privateRules)), "serverRules")
	if r := e.do("free", http.MethodGet, "/v1/apps/PG-3/dsl", nil, nil); r.code != http.StatusOK || r.version() != 1 {
		t.Fatalf("a refused save leaves the app as it was: %d %s", r.code, r.raw)
	}
	r := e.do("free", http.MethodGet, "/v1/apps/PG-1/dsl", nil, nil)
	wantPro(t, "patch adding rules, free", e.do("free", http.MethodPatch, "/v1/apps/PG-1/dsl",
		`{"patch":[{"op":"add","path":"/entities/0/scope","value":"public"}]}`, ifMatch(r.version())), "sharedData")

	for _, app := range []string{"/v1/apps/PG-4", "/v1/apps/PG-5"} {
		if r := put("pro", app, sharedKakeibo(t)); r.code != http.StatusCreated {
			t.Fatalf("shared, pro: %d %s", r.code, r.raw)
		}
	}
	if r := put("pro", "/v1/apps/PG-6", []byte(privateRules)); r.code != http.StatusCreated {
		t.Fatalf("rules, pro: %d %s", r.code, r.raw)
	}

	// POST /apps with a dsl and duplicate are saves too
	wantPro(t, "create with rules, free", e.do("free", http.MethodPost, "/v1/apps", map[string]any{"id": "PG-7", "dsl": rawJSON(privateRules)}, nil), "serverRules")
	if r := e.do("pro", http.MethodPost, "/v1/apps", map[string]any{"id": "PG-8", "dsl": rawJSON(privateRules)}, nil); r.code != http.StatusCreated {
		t.Fatalf("create with rules, pro: %d %s", r.code, r.raw)
	}
	if r := e.do("free", http.MethodPost, "/v1/apps/PG-1/duplicate", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("duplicate without paid features: %d %s", r.code, r.raw)
	}
}

func rawJSON(s string) any {
	return jsonRaw(s)
}

type jsonRaw string

func (j jsonRaw) MarshalJSON() ([]byte, error) { return []byte(j), nil }

// A lapsed Pro keeps the app: it opens, its records read, edits that add no
// paid feature save; adding one, restoring a paid version, duplicating and
// writing shared records need Pro again.
func TestProGateLapsed(t *testing.T) {
	pro := proSet{"owner": true}
	e := newProEnv(t, pro)
	const app = "/v1/apps/PG-LAPSE"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, kakeibo(t)) // v2: private
	r := e.do("owner", http.MethodGet, app+"/dsl", nil, nil)
	if r = e.do("owner", http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), ifMatch(r.version())); r.code != http.StatusCreated { // v3
		t.Fatalf("shared while pro: %d %s", r.code, r.raw)
	}
	id := e.post("owner", recs, `{"data":{"amount":100}}`)
	e.post("friend", recs, `{"data":{"amount":5}}`) // a participant writes for free

	pro["owner"] = false // the subscription ended

	if r = e.do("owner", http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK || r.version() != 3 {
		t.Fatalf("open: %d %s", r.code, r.raw)
	}
	if r = e.do("friend", http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 2 {
		t.Fatalf("read shared: %d %s", r.code, r.raw)
	}
	// an edit that keeps sharedData and adds nothing paid is saved
	if r = e.do("owner", http.MethodPatch, app+"/dsl", `{"patch":[{"op":"replace","path":"/app/name","value":"家計簿2"}]}`, ifMatch(3)); r.code != http.StatusCreated { // v4
		t.Fatalf("edit, lapsed: %d %s", r.code, r.raw)
	}
	wantPro(t, "add rules, lapsed", e.do("owner", http.MethodPut, app+"/dsl", putBody(withRules(t, sharedKakeibo(t))), ifMatch(4)), "serverRules")
	wantPro(t, "duplicate, lapsed", e.do("owner", http.MethodPost, app+"/duplicate", nil, nil), "sharedData")

	// shared writes: the creator pays; the participant is told it is the creator's plan
	wantPro(t, "creator writes", e.do("owner", http.MethodPost, recs, `{"data":{"amount":1}}`, nil), "sharedData")
	r = e.do("friend", http.MethodPost, recs, `{"data":{"amount":1}}`, nil)
	wantPro(t, "participant writes", r, "sharedData")
	if d := r.body["error"].(map[string]any)["details"].(map[string]any); d["reason"] != "creator_needs_pro" {
		t.Fatalf("participant reason: %v", d)
	}
	wantPro(t, "creator edits a record", e.do("owner", http.MethodPatch, recs+"/"+id, `{"data":{"amount":2}}`, nil), "sharedData")
	// deleting one's own record stays open
	if r = e.do("owner", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete own record: %d %s", r.code, r.raw)
	}

	// restoring the private v2 is fine; restoring the shared v3 from there is not
	if r = e.do("owner", http.MethodPost, app+"/versions/2/restore", nil, nil); r.code != http.StatusCreated { // v5
		t.Fatalf("restore private: %d %s", r.code, r.raw)
	}
	wantPro(t, "restore shared", e.do("owner", http.MethodPost, app+"/versions/3/restore", nil, nil), "sharedData")

	pro["owner"] = true // renewed
	if r = e.do("owner", http.MethodPost, app+"/versions/3/restore", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("restore shared, pro: %d %s", r.code, r.raw)
	}
	if r = e.do("friend", http.MethodPost, recs, `{"data":{"amount":1}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("participant writes, creator pro: %d %s", r.code, r.raw)
	}
}

// withRules adds a scheduled owner notification to a DSL.
func withRules(t *testing.T, raw []byte) []byte {
	t.Helper()
	s := string(raw)
	i := len(s) - 1
	for i >= 0 && s[i] != '}' {
		i--
	}
	if i < 0 {
		t.Fatal("not an object")
	}
	return []byte(s[:i] + `, "serverRules": [{"id": "morning", "on": {"every": "daily", "time": "07:00"},
	  "do": [{"notify": {"to": {"kind": "owner"}, "title": "家計簿", "body": "今日の支出をつけましょう"}}]}]}`)
}

// Without WithPro (tests, tools) nothing is enforced.
func TestProGateOffWithoutChecker(t *testing.T) {
	e := newEnv(t)
	e.ownApp("free", "/v1/apps/PG-OFF", []byte(privateRules))
}

// A participant's copy of a shared app is not checked at save (the creator
// pays; the copy never publishes, and its own rules run as the participant,
// whom the runner stops without Pro).
func TestProGateParticipantCopy(t *testing.T) {
	pro := proSet{"owner": true}
	e := newProEnv(t, pro)
	const app = "/v1/apps/PG-PART"
	e.ownApp("owner", app, sharedKakeibo(t))
	r := e.do("friend", http.MethodGet, app+"/dsl", nil, nil)
	if r = e.do("friend", http.MethodPut, app+"/dsl", putBody(withRules(t, sharedKakeibo(t))), ifMatch(r.version())); r.code != http.StatusCreated {
		t.Fatalf("participant copy: %d %s", r.code, r.raw)
	}
}

// PRO_GATE_MODE: log saves and writes but logs pro_gate.would_block; off
// does not look at all; enforce (default) answers 402.
func TestProGateModes(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	for _, mode := range []paid.Mode{paid.ModeEnforce, paid.ModeLog, paid.ModeOff} {
		buf.Reset()
		t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
		e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithPro(proSet{}), WithProMode(mode)))}
		app := "/v1/apps/PG-MODE-" + string(mode)
		e.do("free", http.MethodGet, app+"/dsl", nil, nil)
		r := e.do("free", http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), ifMatch(1))
		w := e.do("friend", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1}}`, nil)
		logged := strings.Count(buf.String(), `"msg":"pro_gate.would_block"`)
		switch mode {
		case paid.ModeEnforce:
			wantPro(t, "enforce save", r, "sharedData")
			if logged != 1 {
				t.Fatalf("enforce logs the stop: %s", buf.String())
			}
		case paid.ModeLog:
			if r.code != http.StatusCreated || w.code != http.StatusCreated {
				t.Fatalf("log: save %d / write %d %s", r.code, w.code, w.raw)
			}
			if logged != 2 || !strings.Contains(buf.String(), `"where":"appconfig.dsl_save"`) || !strings.Contains(buf.String(), `"where":"appconfig.shared_write"`) {
				t.Fatalf("log: %s", buf.String())
			}
		case paid.ModeOff:
			if r.code != http.StatusCreated || w.code != http.StatusCreated || logged != 0 {
				t.Fatalf("off: save %d / write %d / logs %d", r.code, w.code, logged)
			}
		}
	}
}
