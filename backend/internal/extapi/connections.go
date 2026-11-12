package extapi

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Connection statuses (users/{uid}/connections/{provider}.status).
const (
	StatusActive = "active"
	// StatusRevoked: the person cut the connection. The tokens are gone; the
	// document is only the marker (決定 7) until its DeleteAt.
	StatusRevoked = "revoked"
	// StatusError: the provider refused the refresh token. The tokens stay
	// until the person connects again or DeleteAt.
	StatusError = "error"
)

// Connection is users/{uid}/connections/{provider}. Clients can neither read
// nor write it (no Firestore rule allows it; only the runner's SA can, and
// only the runner's SA can open Sealed).
type Connection struct {
	Provider string `firestore:"provider"`
	Status   string `firestore:"status"`
	// Sealed is the token, sealed by a Sealer with uid/provider as the
	// associated data. Empty once revoked.
	Sealed      []byte    `firestore:"sealed,omitempty"`
	Scopes      []string  `firestore:"scopes,omitempty"`
	ConnectedAt time.Time `firestore:"connectedAt,omitempty"`
	LastUsedAt  time.Time `firestore:"lastUsedAt,omitempty"`
	RevokedAt   time.Time `firestore:"revokedAt,omitempty"`
	// DeleteAt is when the document goes (Firestore TTL policy on
	// deleteAt, and ExpireUnused): last use + 90 days, or revoke + 90 days
	// for the marker.
	DeleteAt time.Time `firestore:"deleteAt"`
}

// ConnRef names one connection.
type ConnRef struct {
	UID      string
	Provider string
}

// ConnectionStore keeps connections (Firestore in production, memory in tests).
type ConnectionStore interface {
	Get(ctx context.Context, uid, provider string) (Connection, bool, error)
	Put(ctx context.Context, uid string, c Connection) error
	// Update reads uid's connection of provider and writes what fn returns
	// when its second result is true, in one transaction. fn may run more
	// than once and gets ok false when there is none.
	Update(ctx context.Context, uid, provider string, fn func(c Connection, ok bool) (Connection, bool)) error
	Delete(ctx context.Context, uid, provider string) error
	List(ctx context.Context, uid string) ([]Connection, error)
	// Due returns up to limit connections whose DeleteAt is before now.
	Due(ctx context.Context, now time.Time, limit int) ([]ConnRef, error)
}

// Sealer encrypts tokens at rest. Production uses Cloud KMS (KMSSealer):
// the key's encrypt / decrypt is granted to the runner's SA only.
type Sealer interface {
	Seal(ctx context.Context, plaintext, aad []byte) ([]byte, error)
	Open(ctx context.Context, ciphertext, aad []byte) ([]byte, error)
}

// ErrNotConnected is 409 ext_not_connected: the person has not connected the
// provider, cut it, or the provider refused the token.
var ErrNotConnected = errors.New("extapi: not connected")

// Connections stores and reads per-person tokens.
type Connections struct {
	Store  ConnectionStore
	Sealer Sealer
	// UnusedTTL: a connection not used this long is deleted (default 90 days).
	UnusedTTL time.Duration
	Now       func() time.Time
	Log       *slog.Logger
}

func (c *Connections) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *Connections) ttl() time.Duration {
	if c.UnusedTTL > 0 {
		return c.UnusedTTL
	}
	return DefaultUnusedTTL
}

func (c *Connections) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

// sealedToken is the plaintext form a Sealer seals (Token's own JSON is redacted).
type sealedToken struct {
	A string    `json:"a"`
	R string    `json:"r,omitempty"`
	E time.Time `json:"e,omitzero"`
	S []string  `json:"s,omitempty"`
}

func aad(uid, provider string) []byte { return []byte("ext/v1\x00" + uid + "\x00" + provider) }

func (c *Connections) seal(ctx context.Context, uid, provider string, t Token) ([]byte, error) {
	pt, err := json.Marshal(sealedToken{A: t.AccessToken, R: t.RefreshToken, E: t.Expiry, S: t.Scopes})
	if err != nil {
		return nil, err
	}
	return c.Sealer.Seal(ctx, pt, aad(uid, provider))
}

func (c *Connections) open(ctx context.Context, uid, provider string, sealed []byte) (Token, error) {
	pt, err := c.Sealer.Open(ctx, sealed, aad(uid, provider))
	if err != nil {
		return Token{}, err
	}
	var s sealedToken
	if err := json.Unmarshal(pt, &s); err != nil {
		return Token{}, errors.New("extapi: sealed token is not readable")
	}
	return Token{AccessToken: s.A, RefreshToken: s.R, Expiry: s.E, Scopes: s.S}, nil
}

// Save stores t as uid's active connection of provider (a new connection
// from the OAuth callback).
func (c *Connections) Save(ctx context.Context, uid, provider string, t Token) error {
	if c.Sealer == nil {
		return errors.New("extapi: no sealer")
	}
	sealed, err := c.seal(ctx, uid, provider, t)
	if err != nil {
		return fmt.Errorf("seal: %w", err)
	}
	now := c.now()
	return c.Store.Update(ctx, uid, provider, func(old Connection, ok bool) (Connection, bool) {
		conn := Connection{Provider: provider, Status: StatusActive, Sealed: sealed, Scopes: t.Scopes, ConnectedAt: now, LastUsedAt: now, DeleteAt: now.Add(c.ttl())}
		if ok && old.Status == StatusActive && !old.ConnectedAt.IsZero() {
			conn.ConnectedAt = old.ConnectedAt
		}
		return conn, true
	})
}

// SaveRefreshed stores t, renewed from the token sealed as from, and
// returns its sealed form. Nothing is written and the error is
// ErrNotConnected when the connection no longer holds from (cut, refused or
// connected again meanwhile): a cut connection is never brought back.
func (c *Connections) SaveRefreshed(ctx context.Context, uid, provider string, from []byte, t Token) ([]byte, error) {
	if c.Sealer == nil {
		return nil, errors.New("extapi: no sealer")
	}
	sealed, err := c.seal(ctx, uid, provider, t)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	now := c.now()
	saved := false
	err = c.Store.Update(ctx, uid, provider, func(cur Connection, ok bool) (Connection, bool) {
		saved = ok && cur.Status == StatusActive && bytes.Equal(cur.Sealed, from)
		if !saved {
			return cur, false
		}
		cur.Sealed, cur.Scopes, cur.LastUsedAt, cur.DeleteAt = sealed, t.Scopes, now, now.Add(c.ttl())
		return cur, true
	})
	if err != nil {
		return nil, err
	}
	if !saved {
		return nil, ErrNotConnected
	}
	return sealed, nil
}

// Token returns uid's token of provider and notes its use (DeleteAt moves
// to 90 days from now). ErrNotConnected when there is none, it was cut or
// refused, or it was unused past its DeleteAt (then it is deleted now).
func (c *Connections) Token(ctx context.Context, uid, provider string) (Token, error) {
	t, _, err := c.read(ctx, uid, provider)
	return t, err
}

// read is Token with the sealed form it was read from.
func (c *Connections) read(ctx context.Context, uid, provider string) (Token, []byte, error) {
	if c.Sealer == nil {
		return Token{}, nil, ErrNotConnected
	}
	conn, ok, err := c.Store.Get(ctx, uid, provider)
	if err != nil {
		return Token{}, nil, err
	}
	if !ok || conn.Status != StatusActive || len(conn.Sealed) == 0 {
		return Token{}, nil, ErrNotConnected
	}
	now := c.now()
	if !conn.DeleteAt.IsZero() && !now.Before(conn.DeleteAt) {
		if err := c.Store.Delete(ctx, uid, provider); err != nil {
			c.log().WarnContext(ctx, "extapi: deleting an unused connection failed", slog.String("provider", provider), slog.Any("error", err))
		}
		return Token{}, nil, ErrNotConnected
	}
	t, err := c.open(ctx, uid, provider, conn.Sealed)
	if err != nil {
		return Token{}, nil, fmt.Errorf("open: %w", err)
	}
	if now.Sub(conn.LastUsedAt) >= time.Hour { // one write an hour at most
		// only the dates of the connection as it is now (a token renewed or
		// a connection cut meanwhile stays as it is)
		err := c.Store.Update(ctx, uid, provider, func(cur Connection, ok bool) (Connection, bool) {
			if !ok || cur.Status != StatusActive {
				return cur, false
			}
			cur.LastUsedAt, cur.DeleteAt = now, now.Add(c.ttl())
			return cur, true
		})
		if err != nil {
			c.log().WarnContext(ctx, "extapi: noting a connection's use failed", slog.String("provider", provider), slog.Any("error", err))
		}
	}
	return t, conn.Sealed, nil
}

// MarkError turns uid's connection to error (the provider refused the
// token sealed as sealed): it is not used until the person connects again.
// A connection that no longer holds that token (renewed, cut or connected
// again meanwhile) is left as it is.
func (c *Connections) MarkError(ctx context.Context, uid, provider string, sealed []byte) error {
	return c.Store.Update(ctx, uid, provider, func(cur Connection, ok bool) (Connection, bool) {
		if !ok || cur.Status != StatusActive || len(sealed) == 0 || !bytes.Equal(cur.Sealed, sealed) {
			return cur, false
		}
		cur.Status = StatusError
		return cur, true
	})
}

// Revoke cuts uid's connection: the provider's revoke endpoint is called
// (best effort; client nil or ErrNoRevoke skips it), then the tokens are
// deleted at once and only the revoked marker stays (決定 7).
//
// Providers on one OAuth client (ClientFrom: Google Calendar, Tasks and
// Sheets) hold one grant at the provider, and revoking any of their tokens
// ends all of them. So the revoke endpoint is skipped while another of them
// is still connected for uid, and called when the last one is cut; until
// then the grant stays listed in the person's Google account (only our
// stored tokens are gone).
func (c *Connections) Revoke(ctx context.Context, uid, provider string, client OAuthClient) error {
	conn, ok, err := c.Store.Get(ctx, uid, provider)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if client != nil && c.grantInUse(ctx, uid, provider) {
		client = nil
	}
	if client != nil && len(conn.Sealed) > 0 && c.Sealer != nil {
		if t, err := c.open(ctx, uid, provider, conn.Sealed); err == nil {
			if err := client.Revoke(ctx, t); err != nil && !errors.Is(err, ErrNoRevoke) {
				c.log().WarnContext(ctx, "extapi: provider revoke failed; the stored token is deleted anyway",
					slog.String("provider", provider), slog.String("uid_hash", httpx.UIDHash(uid)), slog.Any("error", err))
			}
		}
	}
	now := c.now()
	marker := Connection{Provider: provider, Status: StatusRevoked, ConnectedAt: conn.ConnectedAt, RevokedAt: now, DeleteAt: now.Add(c.ttl())}
	return c.Store.Put(ctx, uid, marker)
}

// grantInUse: another provider on provider's OAuth client is still connected
// for uid (not revoked). A failed read counts as in use: not revoking
// leaves a grant at the provider; revoking would break the others.
func (c *Connections) grantInUse(ctx context.Context, uid, provider string) bool {
	group := oauthClientOf(provider)
	shared := false
	for _, p := range Providers() {
		if p.ID != provider && oauthClientOf(p.ID) == group {
			shared = true
		}
	}
	if !shared {
		return false
	}
	list, err := c.Store.List(ctx, uid)
	if err != nil {
		c.log().WarnContext(ctx, "extapi: listing connections failed; the provider revoke is skipped",
			slog.String("provider", provider), slog.Any("error", err))
		return true
	}
	return slices.ContainsFunc(list, func(o Connection) bool {
		return o.Provider != provider && o.Status != StatusRevoked && oauthClientOf(o.Provider) == group
	})
}

// oauthClientOf is the provider whose OAuth client id uses (its ClientFrom,
// or itself).
func oauthClientOf(id string) string {
	if p, ok := Lookup(id); ok && p.ClientFrom != "" {
		return p.ClientFrom
	}
	return id
}

// RevokeAll cuts every connection of uid (account deletion).
func (c *Connections) RevokeAll(ctx context.Context, uid string, clients func(provider string) OAuthClient) (int, error) {
	list, err := c.Store.List(ctx, uid)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, conn := range list {
		if conn.Status == StatusRevoked {
			continue
		}
		var cl OAuthClient
		if clients != nil {
			cl = clients(conn.Provider)
		}
		if err := c.Revoke(ctx, uid, conn.Provider, cl); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ExpireUnused deletes up to limit connections past their DeleteAt (unused
// for 90 days, or revoked markers 90 days old). Firestore's TTL policy on
// deleteAt does the same within about a day; this is the sure path.
func (c *Connections) ExpireUnused(ctx context.Context, limit int) (int, error) {
	refs, err := c.Store.Due(ctx, c.now(), limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range refs {
		if err := c.Store.Delete(ctx, r.UID, r.Provider); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ConnectionView is what GET /v1/me/connections shows: never a token.
type ConnectionView struct {
	Provider    string    `json:"provider"`
	Status      string    `json:"status"`
	Scopes      []string  `json:"scopes,omitempty"`
	ConnectedAt time.Time `json:"connectedAt,omitzero"`
	LastUsedAt  time.Time `json:"lastUsedAt,omitzero"`
}

// List returns uid's connections without tokens (revoked markers left out).
func (c *Connections) List(ctx context.Context, uid string) ([]ConnectionView, error) {
	list, err := c.Store.List(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := []ConnectionView{}
	for _, conn := range list {
		if conn.Status == StatusRevoked {
			continue
		}
		out = append(out, ConnectionView{Provider: conn.Provider, Status: conn.Status, Scopes: conn.Scopes, ConnectedAt: conn.ConnectedAt, LastUsedAt: conn.LastUsedAt})
	}
	return out, nil
}

// LocalSealer is AES-256-GCM with a key in memory: tests and the local
// emulator only. Production uses KMSSealer.
type LocalSealer struct{ aead cipher.AEAD }

// NewLocalSealer takes a 32-byte key; nil makes a random one (tokens are
// unreadable after a restart, which is fine locally).
func NewLocalSealer(key []byte) (*LocalSealer, error) {
	if key == nil {
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, err
		}
	}
	if len(key) != 32 {
		return nil, errors.New("extapi: local sealer key must be 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &LocalSealer{aead: a}, nil
}

// Seal implements Sealer.
func (s *LocalSealer) Seal(_ context.Context, plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Open implements Sealer.
func (s *LocalSealer) Open(_ context.Context, ciphertext, aad []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("extapi: sealed value too short")
	}
	return s.aead.Open(nil, ciphertext[:n], ciphertext[n:], aad)
}
