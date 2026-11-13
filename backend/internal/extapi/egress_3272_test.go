package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// openClient3272 returns a NewHTTPClient whose inner transport is swapped for a
// plain one so a loopback httptest server can be reached (the real dialer
// refuses non-public addresses).
func openClient3272(hosts []string, timeout time.Duration) *http.Client {
	c := NewHTTPClient(hosts, timeout)
	c.Transport.(*allowTransport).next = http.DefaultTransport
	return c
}

func TestNewHTTPClient_ListedHostReachable_3272(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer s.Close()
	resp, err := openClient3272([]string{s.URL}, time.Second).Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestNewHTTPClient_UnlistedHostNotSent_3272(t *testing.T) {
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer s.Close()
	_, err := openClient3272([]string{"https://listed.example"}, time.Second).Get(s.URL)
	if !errors.Is(err, ErrEgress) {
		t.Fatalf("err = %v, want ErrEgress", err)
	}
	if hits.Load() != 0 {
		t.Fatal("request reached the server")
	}
}

func TestNewHTTPClient_NonPublicAddrBlockedByDialer_3272(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer s.Close()
	// Listed host, but loopback: the real dialer must refuse it.
	_, err := NewHTTPClient([]string{s.URL}, time.Second).Get(s.URL)
	if !errors.Is(err, ErrEgress) {
		t.Fatalf("err = %v, want ErrEgress", err)
	}
}

func TestNewHTTPClient_Timeout_3272(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer s.Close()
	_, err := openClient3272([]string{s.URL}, 50*time.Millisecond).Get(s.URL)
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestNewHTTPClient_Redirects_3272(t *testing.T) {
	var other atomic.Int32
	otherSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { other.Add(1) }))
	defer otherSrv.Close()
	var s *httptest.Server
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("done"))
		case "/in":
			http.Redirect(w, r, s.URL+"/ok", http.StatusFound)
		case "/out":
			http.Redirect(w, r, otherSrv.URL, http.StatusFound)
		default: // /loop
			http.Redirect(w, r, s.URL+"/loop", http.StatusFound)
		}
	}))
	defer s.Close()
	c := openClient3272([]string{s.URL}, time.Second)

	resp, err := c.Get(s.URL + "/in")
	if err != nil {
		t.Fatalf("same-host redirect: %v", err)
	}
	resp.Body.Close()

	if _, err := c.Get(s.URL + "/out"); !errors.Is(err, ErrEgress) {
		t.Fatalf("off-list redirect err = %v, want ErrEgress", err)
	}
	if other.Load() != 0 {
		t.Fatal("redirect target was contacted")
	}
	if _, err := c.Get(s.URL + "/loop"); !errors.Is(err, ErrEgress) {
		t.Fatalf("redirect loop err = %v, want ErrEgress", err)
	}
}
