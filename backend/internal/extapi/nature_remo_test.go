package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const remoDevBedroom = "11111111-1111-1111-1111-111111111111"

// fakeRemo is Nature's Cloud API v1 on httptest (no live calls).
type fakeRemo struct {
	mu       sync.Mutex
	calls    int
	lastAuth string
	lastURL  string
}

// remoKeys is the per-user key store's stand-in: the token is the user id.
func remoKeys(_ context.Context, in Input) (natureRemoKey, error) {
	tok := in.Args["_who"]
	if tok == "" {
		return natureRemoKey{}, ErrNotConnected
	}
	return natureRemoKey{Token: tok}, nil
}

func newFakeRemo(t *testing.T) (*fakeRemo, map[string]Operation) {
	f := &fakeRemo{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.calls++
		f.lastAuth, f.lastURL = r.Header.Get("Authorization"), r.URL.String()
		f.mu.Unlock()
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "ok":
			return true
		case "forbidden":
			w.WriteHeader(403)
		case "busy":
			w.WriteHeader(503)
		case "limited":
			writeJSON(w, 429, map[string]any{"code": 429001, "message": "Too Many Requests"})
		case "garbage":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("<html>"))
		default:
			writeJSON(w, 401, map[string]any{"code": 401001, "message": "Unauthorized"})
		}
		return false
	}
	mux.HandleFunc("GET /1/devices", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, []map[string]any{
			{"id": remoDevBedroom, "name": "寝室", "serial_number": "secret-serial", "mac_address": "aa:bb:cc",
				"newest_events": map[string]any{
					"te": map[string]any{"val": 23.5, "created_at": "2026-11-12T00:00:00Z"},
					"hu": map[string]any{"val": 45},
					"il": map[string]any{"val": 777},
					"mo": map[string]any{"val": 1, "created_at": "2026-11-12T00:01:00Z"}}},
			{"id": "66666666-6666-6666-6666-666666666666", "name": "リビング(mini)",
				"newest_events": map[string]any{"te": map[string]any{"val": 20.1}}},
		})
	})
	mux.HandleFunc("GET /1/appliances", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, []map[string]any{
			{"id": "22222222-2222-2222-2222-222222222222", "type": "AC", "nickname": "寝室のエアコン",
				"settings": map[string]any{"temp": "26", "mode": "cool", "button": "power-off"},
				"signals":  []map[string]any{{"id": "44444444-4444-4444-4444-444444444444", "name": "secret-signal"}}},
			{"id": "33333333-3333-3333-3333-333333333333", "type": "IR", "nickname": "テレビ"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ops, err := newNatureRemo(remoKeys, srv.URL+"/1", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return f, ops
}

func remoIn(who string, args map[string]string) Input {
	a := map[string]string{"_who": who}
	for k, v := range args {
		a[k] = v
	}
	return Input{Args: a}
}

func TestNatureRemoDevices(t *testing.T) {
	f, ops := newFakeRemo(t)
	ctx := context.Background()
	out, err := ops["devices"].Do(ctx, remoIn("ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{{Title: "寝室", Value: "23.5", Subtitle: "湿度 45%"}, {Title: "リビング(mini)", Value: "20.1"}}
	if len(out.Rows) != 2 || out.Rows[0] != want[0] || out.Rows[1] != want[1] {
		t.Fatalf("rows %+v", out.Rows)
	}
	if f.lastAuth != "Bearer ok" || strings.Contains(f.lastURL, "ok") {
		t.Fatalf("auth %q url %q", f.lastAuth, f.lastURL)
	}
	// only the allowlist goes back: no motion, illuminance, id, serial, MAC
	b, _ := json.Marshal(out)
	for _, s := range []string{"secret-serial", "aa:bb:cc", remoDevBedroom, "777", "照度", "00:01:00"} {
		if strings.Contains(string(b), s) || strings.Contains(fmt.Sprint(out.Cells), s) {
			t.Fatalf("leaked %q in %s", s, b)
		}
	}
	for _, d := range []string{"寝室", remoDevBedroom} {
		out, err := ops["devices"].Do(ctx, remoIn("ok", map[string]string{"device": d}))
		if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "寝室" {
			t.Fatalf("%s: %+v %v", d, out, err)
		}
	}
	if _, err := ops["devices"].Do(ctx, remoIn("ok", map[string]string{"device": "台所"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("unknown device: %v", err)
	}
}

func TestNatureRemoAppliances(t *testing.T) {
	_, ops := newFakeRemo(t)
	ctx := context.Background()
	out, err := ops["appliances"].Do(ctx, remoIn("ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 2 || out.Rows[0] != (Row{Title: "寝室のエアコン", Subtitle: "エアコン"}) || out.Rows[1] != (Row{Title: "テレビ", Subtitle: "赤外線"}) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// no on / off state, no signals, no ids
	b, _ := json.Marshal(out)
	for _, s := range []string{"停止", "power-off", "secret-signal", "2222"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("leaked %q in %s", s, b)
		}
	}
	out, err = ops["appliances"].Do(ctx, remoIn("ok", map[string]string{"appliance": "テレビ"}))
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("by name: %+v %v", out, err)
	}
}

func TestNatureRemoErrors(t *testing.T) {
	f, ops := newFakeRemo(t)
	ctx := context.Background()
	if _, err := ops["devices"].Do(ctx, Input{}); !errors.Is(err, ErrNotConnected) || f.calls != 0 {
		t.Fatalf("no key: %v calls=%d", err, f.calls)
	}
	for _, tok := range []string{"dead", "forbidden"} {
		if _, err := ops["devices"].Do(ctx, remoIn(tok, nil)); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	for _, tok := range []string{"busy", "limited"} {
		_, err := ops["appliances"].Do(ctx, remoIn(tok, nil))
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if _, err := ops["devices"].Do(ctx, remoIn("garbage", nil)); Reason(err) != ReasonUpstream {
		t.Fatalf("bad JSON: %v", err)
	}
	if _, err := newNatureRemo(nil, natureRemoHost+"/1", nil); err == nil {
		t.Fatal("built without a key store")
	}
}

// Check refuses bad arguments before anything is counted.
func TestNatureRemoCheck(t *testing.T) {
	p, _ := Lookup(NatureRemo)
	bad := map[string][]map[string]string{
		"devices":    {{"device": strings.Repeat("あ", 65)}, {"device": "a\nb"}, {"limit": "1"}, {"device": "https://evil.example.com/x"}},
		"appliances": {{"appliance": "a\x00"}, {"x": "1"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{"devices": {}, "appliances": {"appliance": "寝室のエアコン"}} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
	if _, ok := p.Op("sendSignal"); ok {
		t.Fatal("a write op before the owner's decision")
	}
}

// Only api.nature.global is reachable; nothing writes or is cached; off by
// default; and until #1671 it stays off even when enabled.
func TestNatureRemoDefinition(t *testing.T) {
	p, ok := Lookup(NatureRemo)
	if !ok || p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "NATURE_REMO" || len(p.Hosts) != 1 {
		t.Fatalf("%+v", p)
	}
	u, _ := url.Parse(natureRemoHost + "/1/devices")
	if !AllowedURL(p.Hosts, u) {
		t.Fatal("api not on the allowlist")
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 || o.CacheTTLFor != nil {
			t.Errorf("%s write=%v ttl=%v", o.Name, o.Write, o.CacheTTL)
		}
	}
	// the production client refuses any other host
	if _, err := NewHTTPClient(p.Hosts, DefaultTimeout).Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[NatureRemo] {
		t.Fatal("on by default")
	}
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[NatureRemo] = true }})
	if e.svc.Enabled(NatureRemo) {
		t.Fatal("on before the per-user key store (#1671)")
	}
}
