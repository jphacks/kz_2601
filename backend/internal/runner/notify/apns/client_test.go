package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
)

// Test keys are generated here; no real .p8 is read.
func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

type seen struct {
	proto   int
	path    string
	header  http.Header
	payload map[string]any
}

// fakeAPNs is a TLS + HTTP/2 server answering by device token.
func fakeAPNs(t *testing.T, answer func(token string) (int, string)) (*httptest.Server, func() []seen) {
	t.Helper()
	var mu sync.Mutex
	var got []seen
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		mu.Lock()
		got = append(got, seen{proto: r.ProtoMajor, path: r.URL.Path, header: r.Header.Clone(), payload: p})
		mu.Unlock()
		status, reason := answer(strings.TrimPrefix(r.URL.Path, "/3/device/"))
		w.Header().Set("apns-id", "00000000-0000-0000-0000-000000000000")
		w.WriteHeader(status)
		if reason != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"reason": reason})
		}
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, func() []seen {
		mu.Lock()
		defer mu.Unlock()
		return append([]seen(nil), got...)
	}
}

func newClient(t *testing.T, srv *httptest.Server, key *ecdsa.PrivateKey, now func() time.Time) *Client {
	t.Helper()
	c, err := New(Config{KeyID: "KEY123", TeamID: "TEAM456", Topic: "jp.example.app", Key: key,
		Host: srv.URL, HTTPClient: srv.Client(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func answers(token string) (int, string) {
	switch token {
	case "gone":
		return http.StatusGone, "Unregistered"
	case "busy":
		return http.StatusServiceUnavailable, "ServiceUnavailable"
	case "many":
		return http.StatusTooManyRequests, "TooManyRequests"
	case "bad":
		return http.StatusBadRequest, "BadDeviceToken"
	case "expired":
		return http.StatusForbidden, "ExpiredProviderToken"
	}
	return http.StatusOK, ""
}

func TestSendSuccess(t *testing.T) {
	srv, got := fakeAPNs(t, answers)
	key := testKey(t)
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	c := newClient(t, srv, key, func() time.Time { return at })
	collapse := notify.CollapseID("outbox/1", 0)
	r, err := c.Send(context.Background(), notify.Push{Token: "tok1", AppID: "app1", AppName: "部活", Title: "題", Body: "本文", CollapseID: collapse})
	if err != nil || r.Status != 200 || r.Retryable || r.Unregistered {
		t.Fatalf("got %+v %v", r, err)
	}
	s := got()[0]
	if s.proto != 2 {
		t.Errorf("HTTP/%d, want HTTP/2", s.proto)
	}
	if s.path != "/3/device/tok1" {
		t.Errorf("path %s", s.path)
	}
	h := s.header
	for k, want := range map[string]string{"apns-topic": "jp.example.app", "apns-push-type": "alert", "apns-priority": "10", "apns-collapse-id": collapse} {
		if h.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, h.Get(k), want)
		}
	}
	if len(h.Get("apns-collapse-id")) > 64 {
		t.Error("collapse id over 64 bytes")
	}
	aps, _ := s.payload["aps"].(map[string]any)
	alert, _ := aps["alert"].(map[string]any)
	if alert["title"] != "題" || alert["body"] != "本文" || alert["subtitle"] != "部活" || s.payload["appId"] != "app1" {
		t.Errorf("payload %v", s.payload)
	}

	// JWT: header {alg ES256, kid}, claims {iss, iat}, signature verifies with the key
	auth := h.Get("authorization")
	if !strings.HasPrefix(auth, "bearer ") {
		t.Fatalf("authorization %q", auth)
	}
	parts := strings.Split(strings.TrimPrefix(auth, "bearer "), ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	enc := base64.RawURLEncoding
	var hdr map[string]string
	var claims map[string]any
	hb, _ := enc.DecodeString(parts[0])
	cb, _ := enc.DecodeString(parts[1])
	if err := json.Unmarshal(hb, &hdr); err != nil || hdr["alg"] != "ES256" || hdr["kid"] != "KEY123" || len(hdr) != 2 {
		t.Errorf("jwt header %s", hb)
	}
	if err := json.Unmarshal(cb, &claims); err != nil || claims["iss"] != "TEAM456" || claims["iat"] != float64(at.Unix()) || len(claims) != 2 {
		t.Errorf("jwt claims %s", cb)
	}
	sig, _ := enc.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature %d bytes", len(sig))
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("signature does not verify")
	}
}

func TestSendClassifies(t *testing.T) {
	srv, _ := fakeAPNs(t, answers)
	c := newClient(t, srv, testKey(t), nil)
	for _, tc := range []struct {
		token                   string
		status                  int
		unregistered, retryable bool
	}{
		{"gone", 410, true, false},
		{"busy", 503, false, true},
		{"many", 429, false, true},
		{"bad", 400, false, false},
		{"expired", 403, false, true},
	} {
		r, err := c.Send(context.Background(), notify.Push{Token: tc.token})
		if !errors.Is(err, ErrRejected) || r.Status != tc.status || r.Unregistered != tc.unregistered || r.Retryable != tc.retryable {
			t.Errorf("%s: got %+v %v", tc.token, r, err)
		}
	}
}

func TestProviderTokenCachedAndRefreshed(t *testing.T) {
	srv, got := fakeAPNs(t, answers)
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	c := newClient(t, srv, testKey(t), func() time.Time { return at })
	send := func(tok string) string {
		_, _ = c.Send(context.Background(), notify.Push{Token: tok})
		s := got()
		return s[len(s)-1].header.Get("authorization")
	}
	first := send("t")
	if send("t") != first {
		t.Error("token not cached")
	}
	at = at.Add(tokenTTL)
	second := send("t")
	if second == first {
		t.Error("token not refreshed after its TTL")
	}
	send("expired") // APNs refused it → the next send signs a new one
	if send("t") == second {
		t.Error("token not dropped after ExpiredProviderToken")
	}
}

func TestSendNetworkErrorIsRetryable(t *testing.T) {
	srv, _ := fakeAPNs(t, answers)
	c := newClient(t, srv, testKey(t), nil)
	srv.Close()
	r, err := c.Send(context.Background(), notify.Push{Token: "t"})
	if err == nil || !r.Retryable {
		t.Errorf("got %+v %v", r, err)
	}
}

func TestParseKey(t *testing.T) {
	k := testKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil || !got.Equal(k) {
		t.Errorf("ParseKey: %v", err)
	}
	if _, err := ParseKey([]byte("not pem")); err == nil {
		t.Error("want error for non-PEM")
	}
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der, _ = x509.MarshalPKCS8PrivateKey(p384)
	if _, err := ParseKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err == nil {
		t.Error("want error for a P-384 key")
	}
}

func TestNewRequires(t *testing.T) {
	if _, err := New(Config{KeyID: "k", TeamID: "t", Topic: "x"}); err == nil {
		t.Error("want error without a key")
	}
	if _, err := (&Client{cfg: Config{Key: testKey(t)}}).Send(context.Background(), notify.Push{}); err == nil {
		t.Error("want error for an empty token")
	}
}
