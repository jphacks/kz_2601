package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	hueTestID     = "hue-client"
	hueTestSecret = "hue-secret"
)

var hueTestNow = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

// fakeHue is Hue's OAuth (and later the remote API) on httptest (no live calls).
type fakeHue struct {
	mu         sync.Mutex
	rotateNext int
	tokenNext  int // non-zero: the next token answer is this status

	dir     string // testdata/hue/<dir> answers the resource lists
	links   int    // remote links made (application keys handed out)
	apiNext int    // non-zero: the next remote API answer is this status
	gets    []string
	puts    []string // "<light id> <body>" of each light change
}

// remoteOK checks the bearer and, for CLIP v2, the application key. It
// answers the forced status first.
func (f *fakeHue) remoteOK(w http.ResponseWriter, r *http.Request, needKey bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.apiNext != 0 {
		w.WriteHeader(f.apiNext)
		f.apiNext = 0
		return false
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer hue-access-") {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	if needKey && r.Header.Get("hue-application-key") != "app-key-"+strconv.Itoa(f.links) {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	return true
}

func newFakeHue(t *testing.T) (*fakeHue, *httptest.Server) {
	t.Helper()
	f := &fakeHue{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v2/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.tokenNext != 0 {
			w.WriteHeader(f.tokenNext)
			f.tokenNext = 0
			return
		}
		id, secret, ok := r.BasicAuth()
		_ = r.ParseForm()
		if !ok || id != hueTestID || secret != hueTestSecret || r.URL.RawQuery != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		answer := func(n int) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "hue-access-" + strconv.Itoa(n), "refresh_token": "hue-refresh-" + strconv.Itoa(n), "expires_in": 604800, "token_type": "bearer"})
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "hue-code" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			answer(0)
		case "refresh_token":
			if r.Form.Get("refresh_token") != "hue-refresh-"+strconv.Itoa(f.rotateNext) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.rotateNext++
			answer(f.rotateNext)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	mux.HandleFunc("PUT /route/api/0/config", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]bool
		if !f.remoteOK(w, r, false) || json.NewDecoder(r.Body).Decode(&b) != nil || !b["linkbutton"] {
			return
		}
		_, _ = w.Write([]byte(`[{"success":{"/config/linkbutton":true}}]`))
	})
	mux.HandleFunc("POST /route/api", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		if !f.remoteOK(w, r, false) || json.NewDecoder(r.Body).Decode(&b) != nil || b["devicetype"] == "" {
			return
		}
		f.mu.Lock()
		f.links++
		n := f.links
		f.mu.Unlock()
		_, _ = w.Write([]byte(`[{"success":{"username":"app-key-` + strconv.Itoa(n) + `"}}]`))
	})
	mux.HandleFunc("GET /route/clip/v2/resource/{kind}", func(w http.ResponseWriter, r *http.Request) {
		if !f.remoteOK(w, r, true) {
			return
		}
		f.mu.Lock()
		f.gets = append(f.gets, r.PathValue("kind"))
		dir := f.dir
		f.mu.Unlock()
		b, err := os.ReadFile(filepath.Join("testdata", "hue", dir, r.PathValue("kind")+".json"))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	})
	mux.HandleFunc("PUT /route/clip/v2/resource/light/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !f.remoteOK(w, r, true) {
			return
		}
		b, _ := io.ReadAll(r.Body)
		// the lights of testdata/hue/normal
		if !strings.HasPrefix(r.PathValue("id"), "3f2a1c5e-0000-4000-8000-00000000000") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"description":"Not Found"}],"data":[]}`))
			return
		}
		f.mu.Lock()
		f.puts = append(f.puts, r.PathValue("id")+" "+string(b))
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"errors":[],"data":[{"rid":"` + r.PathValue("id") + `","rtype":"light"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func newTestHue(t *testing.T, srv *httptest.Server) *hue {
	t.Helper()
	c, err := newHue(Secrets{ClientID: hueTestID, ClientSecret: hueTestSecret}, hueWire{
		AuthURL:  "https://api.meethue.com/v2/oauth2/authorize",
		TokenURL: srv.URL + "/v2/oauth2/token",
		APIBase:  srv.URL,
		Client:   srv.Client(),
		Now:      func() time.Time { return hueTestNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHueNeedsClient(t *testing.T) {
	if _, err := newHue(Secrets{ClientID: hueTestID}, hueWireDefault); err == nil {
		t.Fatal("built without a secret")
	}
}

func TestHueAuthURL(t *testing.T) {
	_, srv := newFakeHue(t)
	c := newTestHue(t, srv)
	u, err := url.Parse(c.AuthURL("st", "challenge", "https://runner.example/oauth/callback/hue", nil))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "api.meethue.com" || q.Get("client_id") != hueTestID || q.Get("response_type") != "code" || q.Get("state") != "st" {
		t.Fatalf("auth url %s", u)
	}
	if q.Has("client_secret") || q.Has("code_challenge") {
		t.Fatalf("auth url carries more than it should: %s", u)
	}
}

func TestHueExchangeAndRefresh(t *testing.T) {
	_, srv := newFakeHue(t)
	c := newTestHue(t, srv)
	ctx := context.Background()
	tok, err := c.Exchange(ctx, "hue-code", "verifier", "https://runner.example/oauth/callback/hue")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "hue-access-0" || tok.RefreshToken != "hue-refresh-0" || !tok.Expiry.Equal(hueTestNow.Add(7*24*time.Hour)) {
		t.Fatalf("exchange: %+v %v", tok.AccessToken, tok.Expiry)
	}
	tok, err = c.Refresh(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "hue-access-1" || tok.RefreshToken != "hue-refresh-1" {
		t.Fatalf("refresh: %s %s", tok.AccessToken, tok.RefreshToken)
	}
	// the rotated-out refresh token is refused
	if _, err := c.Refresh(ctx, Token{RefreshToken: "hue-refresh-0"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("old refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("no refresh token: %v", err)
	}
	if _, err := c.Exchange(ctx, "other", "", ""); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if err := c.Revoke(ctx, tok); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
}

func TestHueTokenErrors(t *testing.T) {
	f, srv := newFakeHue(t)
	c := newTestHue(t, srv)
	ctx := context.Background()
	for _, tc := range []struct {
		status    int
		grant     bool
		retryable bool
	}{
		{http.StatusUnauthorized, true, false},
		{http.StatusTooManyRequests, false, true},
		{http.StatusServiceUnavailable, false, true},
		{http.StatusForbidden, false, false},
	} {
		f.mu.Lock()
		f.tokenNext = tc.status
		f.mu.Unlock()
		_, err := c.Refresh(ctx, Token{RefreshToken: "hue-refresh-0"})
		if errors.Is(err, ErrInvalidGrant) != tc.grant {
			t.Errorf("%d: %v", tc.status, err)
			continue
		}
		var pe *ProviderError
		if !tc.grant && (!errors.As(err, &pe) || pe.Status != tc.status || pe.Retryable != tc.retryable) {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
	// a wrong client secret is a 401 too
	bad, err := newHue(Secrets{ClientID: hueTestID, ClientSecret: "wrong"}, hueWire{TokenURL: srv.URL + "/v2/oauth2/token", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Exchange(ctx, "hue-code", "", ""); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("wrong secret: %v", err)
	}
}

func TestHueOffByDefault(t *testing.T) {
	if os.Getenv("EXT_HUE_ENABLED") != "" {
		t.Skip("EXT_HUE_ENABLED is set")
	}
	if _, ok := Lookup(Hue); ok {
		t.Fatal("hue is registered without EXT_HUE_ENABLED")
	}
	p := hueProvider()
	if err := p.check(); err != nil {
		t.Fatal(err)
	}
	if !p.NeedsOAuth || !slices.Equal(p.Hosts, []string{"https://api.meethue.com"}) {
		t.Fatalf("%+v", p)
	}
	if _, err := p.CheckArgs("lights", map[string]string{"x": "1"}); err == nil {
		t.Fatal("lights took an argument")
	}
}

func TestHueLights(t *testing.T) {
	tok := &Token{AccessToken: "hue-access-0"}
	for _, tc := range []struct {
		dir   string
		rows  []Row
		cells [][]string
	}{
		{"normal", []Row{
			{Title: "リビング", Subtitle: "オン・明るさ 75%", Value: "3f2a1c5e-0000-4000-8000-000000000001"},
			{Title: "寝室", Subtitle: "オフ", Value: "3f2a1c5e-0000-4000-8000-000000000002"},
			{Title: "玄関のプラグ", Subtitle: "オン", Value: "3f2a1c5e-0000-4000-8000-000000000003"},
		}, [][]string{
			{"3f2a1c5e-0000-4000-8000-000000000001", "リビング", "true", "75", "true"},
			{"3f2a1c5e-0000-4000-8000-000000000002", "寝室", "false", "40", "true"},
			{"3f2a1c5e-0000-4000-8000-000000000003", "玄関のプラグ", "true", "", "true"},
		}},
		{"empty", []Row{}, [][]string{}},
		{"unreachable", []Row{
			{Title: "リビング", Subtitle: "オン・明るさ 100%", Value: "3f2a1c5e-0000-4000-8000-000000000001"},
			{Title: "書斎", Subtitle: "つながっていません", Value: "3f2a1c5e-0000-4000-8000-000000000004"},
		}, [][]string{
			{"3f2a1c5e-0000-4000-8000-000000000001", "リビング", "true", "100", "true"},
			{"3f2a1c5e-0000-4000-8000-000000000004", "書斎", "true", "0", "false"},
		}},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			f, srv := newFakeHue(t)
			f.dir = tc.dir
			c := newTestHue(t, srv)
			out, err := c.lights(context.Background(), Input{Token: tok})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(out.Rows, tc.rows) {
				t.Fatalf("rows %+v", out.Rows)
			}
			if !slices.EqualFunc(out.Cells, tc.cells, slices.Equal) {
				t.Fatalf("cells %v", out.Cells)
			}
		})
	}
}

func TestHueLightsKeepsTheApplicationKey(t *testing.T) {
	f, srv := newFakeHue(t)
	f.dir = "normal"
	c := newTestHue(t, srv)
	ctx := context.Background()
	for range 2 {
		if _, err := c.lights(ctx, Input{Token: &Token{AccessToken: "hue-access-0"}}); err != nil {
			t.Fatal(err)
		}
	}
	if f.links != 1 {
		t.Fatalf("linked %d times", f.links)
	}
	// another person's token gets its own key
	if _, err := c.lights(ctx, Input{Token: &Token{AccessToken: "hue-access-9"}}); err != nil {
		t.Fatal(err)
	}
	if f.links != 2 || len(f.gets) != 6 {
		t.Fatalf("links %d gets %v", f.links, f.gets)
	}
}

func TestHueLightsErrors(t *testing.T) {
	ctx := context.Background()
	f, srv := newFakeHue(t)
	f.dir = "normal"
	c := newTestHue(t, srv)
	if _, err := c.lights(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.lights(ctx, Input{Token: &Token{AccessToken: "other"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	f.apiNext = http.StatusTooManyRequests
	var pe *ProviderError
	if _, err := c.lights(ctx, Input{Token: &Token{AccessToken: "hue-access-0"}}); !errors.As(err, &pe) || !pe.Retryable || pe.Status != 429 {
		t.Fatalf("429: %v", err)
	}
	f.apiNext = http.StatusForbidden
	if _, err := c.lights(ctx, Input{Token: &Token{AccessToken: "hue-access-0"}}); !errors.As(err, &pe) || pe.Retryable || pe.Status != 403 {
		t.Fatalf("403: %v", err)
	}
}

func TestHueSetLightIsAWriteOp(t *testing.T) {
	o, ok := hueProvider().Op("setLight")
	if !ok || !o.Write {
		t.Fatalf("setLight must be a write op (confirmation): %+v", o)
	}
}

func TestHueSetLightArgs(t *testing.T) {
	const id = "3f2a1c5e-0000-4000-8000-000000000001"
	p := hueProvider()
	for _, tc := range []struct {
		args map[string]string
		ok   bool
	}{
		{map[string]string{"id": id, "on": "true"}, true},
		{map[string]string{"id": id, "on": "false"}, true},
		{map[string]string{"id": id, "on": "true", "brightness": "1"}, true},
		{map[string]string{"id": id, "on": "true", "brightness": "100"}, true},
		{map[string]string{"id": id, "on": "true", "brightness": "0"}, false},
		{map[string]string{"id": id, "on": "true", "brightness": "101"}, false},
		{map[string]string{"id": id, "on": "true", "brightness": "50.5"}, false},
		{map[string]string{"id": id, "on": "false", "brightness": "50"}, false},
		{map[string]string{"id": id, "on": "yes"}, false},
		{map[string]string{"id": id}, false},
		{map[string]string{"id": "1", "on": "true"}, false},
		{map[string]string{"id": "3F2A1C5E-0000-4000-8000-000000000001", "on": "true"}, false},
		{map[string]string{"id": id + "/../x", "on": "true"}, false},
		{map[string]string{"on": "true"}, false},
		{map[string]string{"id": id, "on": "true", "color": "red"}, false},
	} {
		if _, err := p.CheckArgs("setLight", tc.args); (err == nil) != tc.ok {
			t.Errorf("%v: %v", tc.args, err)
		}
	}
}

func TestHueSetLight(t *testing.T) {
	const id = "3f2a1c5e-0000-4000-8000-000000000001"
	ctx := context.Background()
	tok := &Token{AccessToken: "hue-access-0"}
	f, srv := newFakeHue(t)
	c := newTestHue(t, srv)
	for _, tc := range []struct {
		args  map[string]string
		title string
		body  string
	}{
		{map[string]string{"id": id, "on": "true", "brightness": "60"}, "オン・明るさ 60%", `{"dimming":{"brightness":60},"on":{"on":true}}`},
		{map[string]string{"id": id, "on": "true"}, "オン", `{"on":{"on":true}}`},
		{map[string]string{"id": id, "on": "false"}, "オフ", `{"on":{"on":false}}`},
	} {
		out, err := c.setLight(ctx, Input{Args: tc.args, Token: tok})
		if err != nil {
			t.Fatal(err)
		}
		if out.ResultID != id || out.Item == nil || out.Item.Title != tc.title || out.Item.Value != id {
			t.Fatalf("%v: %+v", tc.args, out)
		}
		if got := f.puts[len(f.puts)-1]; got != id+" "+tc.body {
			t.Fatalf("sent %s", got)
		}
	}
	if f.links != 1 {
		t.Fatalf("linked %d times", f.links)
	}

	// 404: a light the bridge does not have
	_, err := c.setLight(ctx, Input{Args: map[string]string{"id": "aaaaaaaa-0000-4000-8000-000000000001", "on": "true"}, Token: tok})
	var pe *ProviderError
	if !errors.Is(err, ErrUnknown) || !errors.As(err, &pe) || pe.Status != 404 || pe.Retryable {
		t.Fatalf("404: %v", err)
	}
	// 429
	f.apiNext = http.StatusTooManyRequests
	if _, err := c.setLight(ctx, Input{Args: map[string]string{"id": id, "on": "true"}, Token: tok}); !errors.As(err, &pe) || pe.Status != 429 {
		t.Fatalf("429: %v", err)
	}
	// nothing is sent with bad arguments or without a connection
	n := len(f.puts)
	if _, err := c.setLight(ctx, Input{Args: map[string]string{"id": "x", "on": "true"}, Token: tok}); !errors.Is(err, ErrBadArgs) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := c.setLight(ctx, Input{Args: map[string]string{"id": id, "on": "true"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if len(f.puts) != n {
		t.Fatal("sent with bad arguments")
	}
}
