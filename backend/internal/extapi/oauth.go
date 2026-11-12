package extapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// StateTTL is how long a started connection waits for its callback.
const StateTTL = 10 * time.Minute

// OAuthState is oauthStates/{state}: one started connection (one use,
// expireAt is a Firestore TTL).
type OAuthState struct {
	UID      string    `firestore:"uid"`
	Provider string    `firestore:"provider"`
	Verifier string    `firestore:"verifier"`
	Scopes   []string  `firestore:"scopes,omitempty"`
	ExpireAt time.Time `firestore:"expireAt"`
	// CreatedAt is when Start made it (a connection cut after it is not saved).
	CreatedAt time.Time `firestore:"createdAt,omitempty"`
}

// startedAt is CreatedAt, or for a state written without it ExpireAt − StateTTL.
func (s OAuthState) startedAt() time.Time {
	if !s.CreatedAt.IsZero() {
		return s.CreatedAt
	}
	return s.ExpireAt.Add(-StateTTL)
}

// StateStore keeps OAuth states.
type StateStore interface {
	PutState(ctx context.Context, state string, s OAuthState) error
	// TakeState returns the state and deletes it (one use).
	TakeState(ctx context.Context, state string) (OAuthState, bool, error)
	// DeleteStates deletes every state of uid (account deletion).
	DeleteStates(ctx context.Context, uid string) error
}

// ErrBadState: the callback's state is unknown, used, expired or for another provider.
var ErrBadState = errors.New("extapi: bad oauth state")

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RedirectURI is the provider's callback on the runner.
func (s *Service) RedirectURI(provider string) string {
	return s.cfg.CallbackBase + "/oauth/callback/" + url.PathEscape(provider)
}

// Start begins uid's connection of provider and returns the consent URL
// (opened with ASWebAuthenticationSession). scopes are the ones asked for;
// empty asks for the read ops' scopes only (more on the first write op).
func (s *Service) Start(ctx context.Context, uid, provider string, scopes []string) (string, error) {
	p, ok := Lookup(provider)
	if !ok || !p.NeedsOAuth {
		return "", ErrUnknown
	}
	cl := s.oauth[provider]
	if cl == nil || !s.Enabled(provider) || s.States == nil {
		return "", ErrUnavailable
	}
	if uid == "" {
		return "", ErrUnknown
	}
	all, read := []string{}, []string{}
	for _, o := range p.Ops {
		for _, sc := range o.Scopes {
			if !slices.Contains(all, sc) {
				all = append(all, sc)
			}
			if !o.Write && !slices.Contains(read, sc) {
				read = append(read, sc)
			}
		}
	}
	if len(scopes) == 0 {
		scopes = read
	}
	for _, sc := range scopes {
		if !slices.Contains(all, sc) {
			return "", ErrUnknown
		}
	}
	state, err := randomString(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomString(48)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	now := s.now().UTC()
	if err := s.States.PutState(ctx, state, OAuthState{UID: uid, Provider: provider, Verifier: verifier, Scopes: scopes, ExpireAt: now.Add(StateTTL), CreatedAt: now}); err != nil {
		return "", fmt.Errorf("state: %w", err)
	}
	return cl.AuthURL(state, challenge, s.RedirectURI(provider), scopes), nil
}

// Callback finishes a connection: the state is taken once and checked, the
// code is exchanged and the token sealed. It returns where to send the
// device (AppReturnURL?provider=…&status=ok|error); the URL never has a
// token or a code.
func (s *Service) Callback(ctx context.Context, provider, state, code string) string {
	back := func(status string) string {
		v := url.Values{"provider": {provider}, "status": {status}}
		return s.cfg.AppReturnURL + "?" + v.Encode()
	}
	err := s.callback(ctx, provider, state, code)
	result := "ok"
	if err != nil {
		result = "error"
		s.Log.WarnContext(ctx, "extapi: oauth callback failed", slog.String("provider", provider), slog.String("reason", Reason(err)), slog.Any("error", err))
	}
	return back(result)
}

func (s *Service) callback(ctx context.Context, provider, state, code string) error {
	cl := s.oauth[provider]
	if cl == nil || !s.Enabled(provider) || s.States == nil {
		return ErrUnavailable
	}
	if state == "" || code == "" {
		return ErrBadState
	}
	st, ok, err := s.States.TakeState(ctx, state)
	if err != nil {
		return err
	}
	if !ok || st.Provider != provider || !s.now().Before(st.ExpireAt) {
		return ErrBadState
	}
	t, err := cl.Exchange(ctx, code, st.Verifier, s.RedirectURI(provider))
	if err != nil {
		return fmt.Errorf("exchange: %w", err)
	}
	if len(t.Scopes) == 0 {
		t.Scopes = st.Scopes
	}
	if err := s.Conns.SaveSince(ctx, st.UID, provider, t, st.startedAt()); err != nil {
		if errors.Is(err, ErrCutSinceStart) {
			// the person cut it (or left) while consenting: the new grant goes too
			if rerr := cl.Revoke(context.WithoutCancel(ctx), t); rerr != nil && !errors.Is(rerr, ErrNoRevoke) {
				s.Log.WarnContext(ctx, "extapi: provider revoke failed", slog.String("provider", provider), slog.Any("error", rerr))
			}
		}
		return err
	}
	s.Log.InfoContext(ctx, "extapi: connected", slog.String("provider", provider), slog.String("uid_hash", httpx.UIDHash(st.UID)))
	return nil
}

// Revoke cuts uid's connection of provider (the provider's revoke when it
// has one, then the tokens are deleted; the revoked marker stays).
func (s *Service) Revoke(ctx context.Context, uid, provider string) error {
	if _, ok := Lookup(provider); !ok || uid == "" {
		return ErrUnknown
	}
	if s.Conns == nil {
		return ErrUnavailable
	}
	unlock, err := s.lockConn(ctx, uid, provider)
	if err != nil {
		return err
	}
	defer unlock()
	return s.Conns.Revoke(ctx, uid, provider, s.oauth[provider])
}

// lockConn takes the refresh lock of uid's connection of provider: a revoke
// waits for a refresh on this instance (and the other way round), so it
// revokes the token the refresh stored, not the one it replaced.
func (s *Service) lockConn(ctx context.Context, uid, provider string) (func(), error) {
	return s.refreshing.lock(ctx, connKey(uid, provider))
}

// connKey is the refresh lock's key of uid's connection of provider.
func connKey(uid, provider string) string { return uid + "\x00" + provider }

// RevokeAll cuts every connection of uid (DELETE /v1/me): uid's started
// connections go first, then every connection is cut and every OAuth
// provider gets the revoked marker, so a callback still on its way saves
// nothing.
func (s *Service) RevokeAll(ctx context.Context, uid string) (int, error) {
	if s.Conns == nil || uid == "" {
		return 0, nil
	}
	var serr error
	if s.States != nil {
		if serr = s.States.DeleteStates(ctx, uid); serr != nil {
			serr = fmt.Errorf("states: %w", serr)
		}
	}
	n, err := s.Conns.revokeAll(ctx, uid, func(p string) OAuthClient { return s.oauth[p] }, s.lockConn)
	return n, errors.Join(err, serr)
}
