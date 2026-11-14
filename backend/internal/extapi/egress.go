package extapi

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
)

// ErrEgress: a request or redirect to a host the provider does not list, or
// an address that is not public.
var ErrEgress = errors.New("extapi: egress blocked")

// NewHTTPClient is the client a provider's operations use: only hosts
// (scheme://host, Provider.Hosts), every connection to a public address
// (feeds.PublicAddr: no metadata server, no private range), at most 3
// redirects that stay on those hosts, no proxy from env.
func NewHTTPClient(hosts []string, timeout time.Duration) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil || !feeds.PublicAddr(ap.Addr()) {
			return fmt.Errorf("%w: %s", ErrEgress, address)
		}
		return nil
	}}
	t := &http.Transport{Proxy: nil, DialContext: d.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: timeout, MaxIdleConns: 10, IdleConnTimeout: 90 * time.Second, MaxResponseHeaderBytes: 32 << 10}
	return &http.Client{Transport: &allowTransport{hosts: hosts, next: t}, Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !AllowedURL(hosts, req.URL) || req.URL.Scheme != via[0].URL.Scheme {
				return fmt.Errorf("%w: redirect to %s", ErrEgress, req.URL.Host)
			}
			return nil
		}}
}

// allowTransport refuses a request outside hosts before it is sent.
type allowTransport struct {
	hosts []string
	next  http.RoundTripper
}

func (a *allowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !AllowedURL(a.hosts, req.URL) {
		return nil, fmt.Errorf("%w: %s", ErrEgress, req.URL.Host)
	}
	return a.next.RoundTrip(req)
}

// AllowedURL reports whether u is on one of hosts (no user info).
func AllowedURL(hosts []string, u *url.URL) bool {
	return u != nil && u.User == nil && slices.Contains(hosts, u.Scheme+"://"+u.Host)
}

// ReadBody reads at most MaxBody of a response (more is an error).
func ReadBody(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxBody {
		return nil, errors.New("extapi: response body over 1 MB")
	}
	return b, nil
}
