package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func newTestHue(t *testing.T, srv *httptest.Server) *hue {
	t.Helper()
	c, err := newHue(Secrets{ClientID: hueTestID, ClientSecret: hueTestSecret}, hueWire{
		AuthURL:  "https://api.meethue.com/v2/oauth2/authorize",
		TokenURL: srv.URL + "/v2/oauth2/token",
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
