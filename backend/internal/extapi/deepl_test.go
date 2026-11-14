package extapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const deeplTestKey = "00000000-1111-2222-3333-444444444444"

// deeplFake is a fake DeepL API: it records the request and answers.
type deeplFake struct {
	status atomic.Int64
	billed atomic.Int64 // < 0: no billed_characters in the answer
	calls  atomic.Int64
	req    atomic.Value // deeplRequest
	auth   atomic.Value // string
}

func newDeepLFake(t *testing.T) (*deeplFake, *httptest.Server) {
	t.Helper()
	f := &deeplFake{}
	f.status.Store(200)
	f.billed.Store(-1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.auth.Store(r.Header.Get("Authorization"))
		if r.Method != http.MethodPost || r.URL.Path != "/v2/translate" || r.URL.RawQuery != "" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var q deeplRequest
		_ = json.Unmarshal(b, &q)
		f.req.Store(q)
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"message":"no"}`))
			return
		}
		tr := map[string]any{"detected_source_language": "EN", "text": "こんにちは、世界"}
		if n := f.billed.Load(); n >= 0 {
			tr["billed_characters"] = n
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"translations": []any{tr}})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestDeepLTranslate(t *testing.T) {
	f, srv := newDeepLFake(t)
	ops, err := newDeepL(deeplTestKey, srv.URL, 0.01, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	out, err := ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": "Hello, world", "targetLang": "ja"}})
	if err != nil {
		t.Fatal(err)
	}
	q := f.req.Load().(deeplRequest)
	if len(q.Text) != 1 || q.Text[0] != "Hello, world" || q.TargetLang != "JA" || q.SourceLang != "" || !q.ShowBilledCharacters {
		t.Fatalf("request %+v", q)
	}
	if f.auth.Load() != "DeepL-Auth-Key "+deeplTestKey {
		t.Fatal("the key is not in the Authorization header")
	}
	want := Row{Title: "こんにちは、世界", Subtitle: "EN → JA", Value: "こんにちは、世界"}
	if out.Item == nil || *out.Item != want {
		t.Fatalf("item %+v", out.Item)
	}
	// no billed count in the answer: the estimate (12 characters)
	if !out.Usage.Priced || out.Usage.USD != 12*0.01 {
		t.Fatalf("usage %+v", out.Usage)
	}
	// the billed count wins
	f.billed.Store(30)
	out, err = ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": "Hello, world", "targetLang": "EN-US", "sourceLang": "de"}})
	if err != nil || out.Usage.USD != 30*0.01 || out.Item.Subtitle != "DE → EN-US" {
		t.Fatalf("%+v %v", out, err)
	}
	if q := f.req.Load().(deeplRequest); q.SourceLang != "DE" || q.TargetLang != "EN-US" {
		t.Fatalf("request %+v", q)
	}
	// no per-character cost: the op does not price the call
	ops, _ = newDeepL(deeplTestKey, srv.URL, -1, srv.Client())
	if out, err := ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": "a", "targetLang": "JA"}}); err != nil || out.Usage.Priced {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestDeepLRefusesBadArgs(t *testing.T) {
	f, srv := newDeepLFake(t)
	ops, _ := newDeepL(deeplTestKey, srv.URL, 0.01, srv.Client())
	for _, a := range []map[string]string{
		{"targetLang": "JA"},
		{"text": "   ", "targetLang": "JA"},
		{"text": strings.Repeat("あ", deeplMaxBytes/3+1), "targetLang": "JA"}, // over 4 KB
		{"text": "\xff", "targetLang": "JA"},
		{"text": "hi"},
		{"text": "hi", "targetLang": "japanese"},
		{"text": "hi", "targetLang": "JA", "sourceLang": "EN-US"},
		{"text": "hi", "targetLang": "J&A"},
	} {
		if _, err := ops["translate"].Do(t.Context(), Input{Args: a}); Reason(err) != ReasonBadRequest {
			t.Fatalf("%q: %s (%v)", a, Reason(err), err)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("a bad argument reached the API")
	}
	// exactly 4 KB is fine
	if _, err := ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": strings.Repeat("a", deeplMaxBytes), "targetLang": "JA"}}); err != nil {
		t.Fatal(err)
	}
}

func TestDeepLDefinition(t *testing.T) {
	p, ok := Lookup(DeepL)
	if !ok || p.Billing != BillingPaid || p.NeedsOAuth || p.Fake || p.EnvName() != "DEEPL" || len(p.Hosts) != 1 || p.Hosts[0] != deeplHost {
		t.Fatalf("%+v", p)
	}
	// no key, a free-API key (":fx"): off
	for _, k := range []string{"", "x", deeplTestKey + ":fx", "has space 00000000-1111"} {
		if _, err := newDeepL(k, deeplHost, -1, http.DefaultClient); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: deeplTestKey}); err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]float64{"": -1, "0": 0, " 0.5 ": 0.5} {
		if got, err := deeplPerChar(v); err != nil || got != want {
			t.Fatalf("%q: %v %v", v, got, err)
		}
	}
	for _, v := range []string{"-1", "abc", "NaN", "Inf"} {
		if _, err := deeplPerChar(v); err == nil {
			t.Fatalf("%q accepted", v)
		}
	}
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_DEEPL_API_KEY" {
			return deeplTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[DeepL] {
		t.Fatalf("enabled without EXT_DEEPL_ENABLED: %v", err)
	}
}

// Through Service.Call: hold → commit the billed cost → the rest back; a
// 5xx / 429 / timeout-like failure gives the run and the coins back; the
// key and the text are in no log line.
func TestDeepLThroughService(t *testing.T) {
	f, srv := newDeepLFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[DeepL] = true
		c.Secrets = map[string]Secrets{DeepL: {APIKey: deeplTestKey}}
		c.Reserve[DeepL] = 50
	}})
	if !e.svc.Enabled(DeepL) {
		t.Fatal("not enabled")
	}
	ops, _ := newDeepL(deeplTestKey, srv.URL, 0.01, srv.Client())
	e.svc.ops[DeepL] = ops
	e.fund(user, 100)
	f.billed.Store(12)
	text := "secret diary entry"
	res, err := e.call(e.req(DeepL, "translate", map[string]string{"text": text, "targetLang": "JA"}))
	// 12 × 0.01 USD × 100 coins/USD = 12 of the 50 held
	if err != nil || res.Charge.Coins != 12 || res.Charge.Balance != 88 || e.runs() != 1 {
		t.Fatalf("%+v %v runs=%d", res, err, e.runs())
	}
	if w := e.wallet(user); w.Reserved != 0 || w.Available() != 88 {
		t.Fatalf("wallet %+v", w)
	}
	for _, s := range []int64{500, 503, 429, 456, 403} {
		f.status.Store(s)
		_, err := e.call(e.req(DeepL, "translate", map[string]string{"text": text, "targetLang": "JA"}))
		if Reason(err) != ReasonUpstream {
			t.Fatalf("status %d: %v", s, err)
		}
		if w := e.wallet(user); w.Reserved != 0 || w.Available() != 88 || e.runs() != 1 {
			t.Fatalf("status %d: not given back: %+v runs=%d", s, w, e.runs())
		}
	}
	// a bad argument: nothing counted, no call
	f.status.Store(200)
	n := f.calls.Load()
	if _, err := e.call(e.req(DeepL, "translate", map[string]string{"text": text, "targetLang": "??"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("bad lang: %v", err)
	}
	if f.calls.Load() != n || e.runs() != 1 || e.wallet(user).Available() != 88 {
		t.Fatal("a refused argument was counted or sent")
	}
	if l := e.logs.String(); strings.Contains(l, deeplTestKey) || strings.Contains(l, text) {
		t.Fatal("the key or the text is in the logs")
	}
	// paid under enforce without COIN_RESERVE_EXT_DEEPL: unavailable
	e2 := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[DeepL] = true
		c.Secrets = map[string]Secrets{DeepL: {APIKey: deeplTestKey}}
	}})
	if _, err := e2.call(e2.req(DeepL, "translate", map[string]string{"text": "a", "targetLang": "JA"})); Reason(err) != ReasonUnavailable {
		t.Fatalf("no reserve: %v", err)
	}
}
