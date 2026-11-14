package extapi

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// egressServerS7003 counts hits; h answers them (nil: 200 "ok").
type egressServerS7003 struct {
	*httptest.Server
	hits atomic.Int32
}

func newEgressServerS7003(t *testing.T, h http.HandlerFunc) *egressServerS7003 {
	t.Helper()
	s := &egressServerS7003{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if h == nil {
			_, _ = w.Write([]byte("ok"))
			return
		}
		h(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// dialLoopbackS7003 swaps only the dialer so a loopback httptest server is
// reachable (the real dialer refuses non-public addresses). The host list,
// redirect rule and timeouts stay those of NewHTTPClient.
func dialLoopbackS7003(t *testing.T, c *http.Client) *http.Client {
	t.Helper()
	at, ok := c.Transport.(*allowTransport)
	if !ok {
		t.Fatalf("transport = %T, want *allowTransport", c.Transport)
	}
	tr, ok := at.next.(*http.Transport)
	if !ok {
		t.Fatalf("next = %T, want *http.Transport", at.next)
	}
	tr.DialContext = (&net.Dialer{Timeout: time.Second}).DialContext
	return c
}

func TestNewHTTPClient_S7Backend003_ListedHostReached(t *testing.T) {
	t.Parallel()
	s := newEgressServerS7003(t, nil)
	c := dialLoopbackS7003(t, NewHTTPClient([]string{s.URL}, time.Second))
	resp, err := c.Get(s.URL + "/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	b, err := ReadBody(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(b) != "ok" || resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d %q %v, want 200 ok", resp.StatusCode, b, err)
	}
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", s.hits.Load())
	}
}

func TestNewHTTPClient_S7Backend003_UnlistedHostNotSent(t *testing.T) {
	t.Parallel()
	s := newEgressServerS7003(t, nil)
	c := dialLoopbackS7003(t, NewHTTPClient([]string{"https://allowed.example"}, time.Second))
	for _, raw := range []string{
		s.URL + "/x",
		strings.Replace(s.URL, "http://", "http://user@", 1) + "/x", // user info is never allowed
	} {
		if _, err := c.Get(raw); !errors.Is(err, ErrEgress) {
			t.Errorf("Get(%s) err = %v, want ErrEgress", raw, err)
		}
	}
	if s.hits.Load() != 0 {
		t.Fatalf("hits = %d, want 0 (nothing sent)", s.hits.Load())
	}
}

// Without the test dialer a listed loopback host is still refused at dial.
func TestNewHTTPClient_S7Backend003_PrivateAddrRefused(t *testing.T) {
	t.Parallel()
	s := newEgressServerS7003(t, nil)
	c := NewHTTPClient([]string{s.URL}, time.Second)
	if _, err := c.Get(s.URL + "/x"); !errors.Is(err, ErrEgress) {
		t.Fatalf("err = %v, want ErrEgress", err)
	}
	if s.hits.Load() != 0 {
		t.Fatalf("hits = %d, want 0", s.hits.Load())
	}
}

func TestNewHTTPClient_S7Backend003_Redirects(t *testing.T) {
	t.Parallel()
	other := newEgressServerS7003(t, nil)
	var self string
	s := newEgressServerS7003(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/done", http.StatusFound)
		case "/other":
			http.Redirect(w, r, other.URL+"/x", http.StatusFound)
		case "/https":
			http.Redirect(w, r, strings.Replace(self, "http://", "https://", 1)+"/done", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			_, _ = w.Write([]byte("done"))
		}
	})
	self = s.URL
	https := strings.Replace(s.URL, "http://", "https://", 1)
	c := dialLoopbackS7003(t, NewHTTPClient([]string{s.URL, https}, time.Second))

	resp, err := c.Get(s.URL + "/same")
	if err != nil {
		t.Fatalf("same host redirect: %v", err)
	}
	_ = resp.Body.Close()
	if resp.Request.URL.Path != "/done" {
		t.Fatalf("ended at %s, want /done", resp.Request.URL.Path)
	}

	for _, path := range []string{"/other", "/https", "/loop"} {
		if _, err := c.Get(s.URL + path); !errors.Is(err, ErrEgress) {
			t.Errorf("Get(%s) err = %v, want ErrEgress", path, err)
		}
	}
	if other.hits.Load() != 0 {
		t.Fatalf("unlisted redirect target hits = %d, want 0", other.hits.Load())
	}
}

func TestNewHTTPClient_S7Backend003_Timeout(t *testing.T) {
	t.Parallel()
	s := newEgressServerS7003(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	c := dialLoopbackS7003(t, NewHTTPClient([]string{s.URL}, 50*time.Millisecond))
	start := time.Now()
	_, err := c.Get(s.URL + "/slow")
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v, want the 50ms timeout to stop it", d)
	}
}
