// Package apns sends notify.Push to Apple Push Notification service with
// token-based authentication (ES256 JWT) over HTTP/2. Only the runner holds
// the key (#337); this package never reads a key file itself.
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
)

// APNs hosts.
const (
	HostProduction  = "https://api.push.apple.com"
	HostDevelopment = "https://api.sandbox.push.apple.com"
)

// tokenTTL: Apple accepts a provider token for an hour and rejects
// refreshing more often than every 20 minutes.
const tokenTTL = 50 * time.Minute

// ErrRejected wraps a non-200 answer.
var ErrRejected = errors.New("apns: push rejected")

// Config configures a Client.
type Config struct {
	KeyID  string
	TeamID string
	// Topic is the app's bundle id (apns-topic).
	Topic string
	// Key is the ES256 (P-256) signing key; see ParseKey.
	Key *ecdsa.PrivateKey
	// Host: "" = HostProduction.
	Host string
	// HTTPClient: nil = an HTTP/2 client with a 10s timeout.
	HTTPClient *http.Client
	// Now: nil = time.Now.
	Now func() time.Time
}

// Client implements notify.Sender.
type Client struct {
	cfg  Config
	http *http.Client
	now  func() time.Time

	mu       sync.Mutex
	token    string
	issuedAt time.Time
}

var _ notify.Sender = (*Client)(nil)

// ParseKey parses a PKCS#8 PEM ES256 key (the .p8 format). The caller reads
// it from the secret store.
func ParseKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("apns: key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns: parse key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, errors.New("apns: key is not an ES256 (P-256) key")
	}
	return ec, nil
}

// New builds a Client.
func New(cfg Config) (*Client, error) {
	if cfg.KeyID == "" || cfg.TeamID == "" || cfg.Topic == "" || cfg.Key == nil {
		return nil, errors.New("apns: KeyID, TeamID, Topic and Key are required")
	}
	if cfg.Key.Curve != elliptic.P256() {
		return nil, errors.New("apns: key is not an ES256 (P-256) key")
	}
	if cfg.Host == "" {
		cfg.Host = HostProduction
	}
	c := &Client{cfg: cfg, http: cfg.HTTPClient, now: cfg.Now}
	if c.http == nil {
		c.http = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		}}
	}
	if c.now == nil {
		c.now = time.Now
	}
	return c, nil
}

type alert struct {
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body,omitempty"`
}

type aps struct {
	Alert    alert  `json:"alert"`
	Sound    string `json:"sound,omitempty"`
	ThreadID string `json:"thread-id,omitempty"`
}

type payload struct {
	APS   aps    `json:"aps"`
	AppID string `json:"appId,omitempty"`
}

// Send posts p. err is nil only for 200; Result says whether the token is
// unregistered (410, or 400 BadDeviceToken: the runner sends only tokens of
// its own environment, so the token itself is bad) or a retry may help (429, 5xx, network, a provider
// token APNs refused).
func (c *Client) Send(ctx context.Context, p notify.Push) (notify.Result, error) {
	if p.Token == "" {
		return notify.Result{}, errors.New("apns: empty device token")
	}
	body, err := json.Marshal(payload{
		APS:   aps{Alert: alert{Title: p.Title, Subtitle: p.AppName, Body: p.Body}, Sound: "default", ThreadID: p.AppID},
		AppID: p.AppID,
	})
	if err != nil {
		return notify.Result{}, fmt.Errorf("apns: payload: %w", err)
	}
	jwt, err := c.providerToken()
	if err != nil {
		return notify.Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Host, "/")+"/3/device/"+p.Token, bytes.NewReader(body))
	if err != nil {
		return notify.Result{}, fmt.Errorf("apns: request: %w", err)
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", c.cfg.Topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("content-type", "application/json")
	if p.CollapseID != "" {
		req.Header.Set("apns-collapse-id", p.CollapseID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return notify.Result{Retryable: true}, fmt.Errorf("apns: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return notify.Result{Status: resp.StatusCode}, nil
	}
	var e struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
	r := notify.Result{Status: resp.StatusCode, Reason: e.Reason}
	switch {
	case resp.StatusCode == http.StatusGone,
		resp.StatusCode == http.StatusBadRequest && e.Reason == "BadDeviceToken":
		r.Unregistered = true
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		r.Retryable = true
	case resp.StatusCode == http.StatusForbidden && (e.Reason == "ExpiredProviderToken" || e.Reason == "InvalidProviderToken"):
		c.dropToken(jwt)
		r.Retryable = true
	}
	return r, fmt.Errorf("%w: %d %s", ErrRejected, resp.StatusCode, e.Reason)
}

// providerToken returns the cached JWT or signs a new one.
func (c *Client) providerToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.token != "" && now.Sub(c.issuedAt) < tokenTTL {
		return c.token, nil
	}
	tok, err := signJWT(c.cfg.Key, c.cfg.KeyID, c.cfg.TeamID, now)
	if err != nil {
		return "", err
	}
	c.token, c.issuedAt = tok, now
	return tok, nil
}

func (c *Client) dropToken(tok string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == tok {
		c.token = ""
	}
}

// signJWT makes the APNs provider token: {"alg":"ES256","kid"} . {"iss","iat"},
// signed as raw r||s (JWS, RFC 7518 §3.4).
func signJWT(key *ecdsa.PrivateKey, keyID, teamID string, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	h, err := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	if err != nil {
		return "", err
	}
	cl, err := json.Marshal(map[string]any{"iss": teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	signing := enc.EncodeToString(h) + "." + enc.EncodeToString(cl)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("apns: sign: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig), nil
}
