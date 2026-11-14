package extapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// 偽の鍵(gitleaks に掛からない値)。
var googleTranslateTestKey = strings.Repeat("g", 32)

// googleTranslateFake is a fake Translation v2 API on httptest (no live calls).
type googleTranslateFake struct {
	status atomic.Int64
	calls  atomic.Int64
	body   atomic.Value // map[string]any
	query  atomic.Value // string
}

func newGoogleTranslateFake(t *testing.T) (*googleTranslateFake, map[string]Operation) {
	t.Helper()
	f := &googleTranslateFake{}
	f.status.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("X-Goog-Api-Key") != googleTranslateTestKey || strings.Contains(r.URL.RawQuery, "key=") {
			w.WriteHeader(403)
			return
		}
		f.query.Store(r.URL.RawQuery)
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		f.body.Store(m)
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Invalid Value"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /language/translate/v2":
			_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"こんにちは、世界","detectedSourceLanguage":"en"}]}}`))
		case "POST /language/translate/v2/detect":
			_, _ = w.Write([]byte(`{"data":{"detections":[[{"language":"en","isReliable":false,"confidence":0.98}]]}}`))
		case "GET /language/translate/v2/languages":
			_, _ = w.Write([]byte(`{"data":{"languages":[{"language":"en","name":"英語"},{"language":"ja","name":"日本語"},{"language":""}]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	ops, err := newGoogleTranslate(googleTranslateTestKey, srv.URL, 0.001, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return f, ops
}

func TestGoogleTranslateTranslate(t *testing.T) {
	f, ops := newGoogleTranslateFake(t)
	out, err := ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": "Hello, world", "targetLang": "JA"}})
	if err != nil {
		t.Fatal(err)
	}
	b := f.body.Load().(map[string]any)
	if b["target"] != "ja" || b["format"] != "text" || b["source"] != nil {
		t.Fatalf("body %+v", b)
	}
	want := Row{Title: "こんにちは、世界", Subtitle: "en → ja", Value: "こんにちは、世界"}
	if out.Item == nil || *out.Item != want {
		t.Fatalf("item %+v", out.Item)
	}
	if !out.Usage.Priced || out.Usage.USD != 12*0.001 {
		t.Fatalf("usage %+v", out.Usage)
	}
	out, err = ops["translate"].Do(t.Context(), Input{Args: map[string]string{"text": "你好", "targetLang": "ja", "sourceLang": "ZH-CN"}})
	if err != nil {
		t.Fatal(err)
	}
	if b := f.body.Load().(map[string]any); b["source"] != "zh-CN" || out.Item.Subtitle != "zh-CN → ja" {
		t.Fatalf("body %+v item %+v", b, out.Item)
	}
}

func TestGoogleTranslateDetectAndLanguages(t *testing.T) {
	f, ops := newGoogleTranslateFake(t)
	out, err := ops["detect"].Do(t.Context(), Input{Args: map[string]string{"text": "Hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Item == nil || out.Item.Value != "en" || out.Item.Subtitle != "確からしさ 98%" || !out.Usage.Priced {
		t.Fatalf("detect %+v %+v", out.Item, out.Usage)
	}
	out, err = ops["languages"].Do(t.Context(), Input{Args: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if f.query.Load() != "target=ja" || len(out.Rows) != 2 || out.Rows[1] != (Row{Title: "日本語", Value: "ja"}) || out.Usage.USD != 0 || !out.Usage.Priced {
		t.Fatalf("languages %q %+v", f.query.Load(), out)
	}
	p, _ := Lookup(GoogleTranslate)
	if o, _ := p.Op("languages"); o.CacheTTL == 0 {
		t.Fatal("languages should be cached")
	}
	for _, name := range []string{"translate", "detect"} {
		if o, _ := p.Op(name); o.CacheTTL != 0 {
			t.Fatalf("%s must not be cached (texts)", name)
		}
	}
}

func TestGoogleTranslateBadArgs(t *testing.T) {
	f, ops := newGoogleTranslateFake(t)
	p, _ := Lookup(GoogleTranslate)
	for _, c := range []struct {
		op   string
		args map[string]string
	}{
		{"translate", map[string]string{"text": "", "targetLang": "ja"}},
		{"translate", map[string]string{"text": "  ", "targetLang": "ja"}},
		{"translate", map[string]string{"text": strings.Repeat("a", 4097), "targetLang": "ja"}},
		{"translate", map[string]string{"text": "hi"}},
		{"translate", map[string]string{"text": "hi", "targetLang": "japanese"}},
		{"translate", map[string]string{"text": "hi", "targetLang": "ja", "sourceLang": "e1"}},
		{"translate", map[string]string{"text": "hi", "targetLang": "ja", "url": "x"}},
		{"detect", map[string]string{"text": ""}},
		{"languages", map[string]string{"displayLang": "日本"}},
	} {
		if _, err := p.CheckArgs(c.op, c.args); !errors.Is(err, ErrUnknown) {
			t.Errorf("CheckArgs %s %v: %v", c.op, c.args, err)
		}
		if _, ok := c.args["url"]; ok {
			continue
		}
		if _, err := ops[c.op].Do(t.Context(), Input{Args: c.args}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("Do %s %v: %v", c.op, c.args, err)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatalf("bad args reached the API %d times", f.calls.Load())
	}
}

func TestGoogleTranslateUpstreamErrors(t *testing.T) {
	f, ops := newGoogleTranslateFake(t)
	args := map[string]string{"text": "hi", "targetLang": "ja"}
	f.status.Store(400)
	if _, err := ops["translate"].Do(t.Context(), Input{Args: args}); Reason(err) != ReasonBadRequest {
		t.Fatalf("400: %v", err)
	}
	for _, s := range []int{403, 429, 500, 503} {
		f.status.Store(int64(s))
		_, err := ops["translate"].Do(t.Context(), Input{Args: args})
		if Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
			t.Fatalf("%d: %v", s, err)
		}
	}
}

func TestGoogleTranslateConfig(t *testing.T) {
	if _, err := newGoogleTranslate("", "https://x", -1, nil); err == nil {
		t.Fatal("empty key accepted")
	}
	if v, err := googleTranslatePerChar(""); err != nil || v != -1 {
		t.Fatal(v, err)
	}
	if _, err := googleTranslatePerChar("-1"); err == nil {
		t.Fatal("negative cost accepted")
	}
	p, ok := Lookup(GoogleTranslate)
	if !ok || p.Billing != BillingPaid || p.NeedsOAuth || len(p.Hosts) != 1 || p.Hosts[0] != "https://translation.googleapis.com" {
		t.Fatalf("provider %+v", p)
	}
}
