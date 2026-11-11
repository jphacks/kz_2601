package extapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Operation is one op of one provider. A provider's file implements it
// against the real API (child issues of #1493); fake.go has fixed answers.
type Operation interface {
	Do(ctx context.Context, in Input) (Output, error)
}

// OperationFunc adapts a function to Operation.
type OperationFunc func(ctx context.Context, in Input) (Output, error)

// Do implements Operation.
func (f OperationFunc) Do(ctx context.Context, in Input) (Output, error) { return f(ctx, in) }

// Input is what an op gets: the checked arguments and, for a NeedsOAuth
// provider, the access token of the person the call acts for.
type Input struct {
	Args map[string]string
	// Token is nil for a provider without OAuth. Never log it.
	Token *Token
	// RequestID is the idempotency key a write op passes on when the
	// provider has one (X-Request-Id).
	RequestID string
}

// Row is one display row (externalList: title / subtitle / value / link).
type Row struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Value    string `json:"value,omitempty"`
	URL      string `json:"url,omitempty"`
}

// Output is the op's answer in the fixed shape the app shows; raw provider
// JSON never goes back to the device.
type Output struct {
	Rows []Row `json:"rows,omitempty"`
	// Cells are Rows' cells in column order (Cells[i] is Rows[i]) for
	// server-side readers only (the scheduled import, #1712); an op that
	// has no columns leaves it nil. Never sent to a device.
	Cells [][]string `json:"-"`
	Item  *Row       `json:"item,omitempty"`
	// ResultID is the id of what a write op made (resultIdField).
	ResultID string `json:"resultId,omitempty"`
	// Usage is the call's measured cost (paid providers).
	Usage Usage `json:"-"`
	// ValidUntil, when set, is when the first row stops being valid (ODPT
	// dct:valid): the answer cache keeps the answer no longer than this.
	ValidUntil time.Time `json:"-"`
}

// Usage is the measured cost of a paid call. Priced false: the op did not
// know (no usage in the response); EXT_COST_USD_<PROVIDER>_<OP> is used, or
// the hold is charged when that is not set either.
type Usage struct {
	USD    float64
	Priced bool
}

// ProviderError is a failed call to the provider. Billable: the provider
// charged it anyway (a 4xx that cost); the call is committed, not given back.
type ProviderError struct {
	Status   int
	Billable bool
	Usage    Usage
	// Retryable: a 429 / 5xx a read op may try once more.
	Retryable bool
	Err       error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider status %d: %v", e.Status, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// Secrets are a provider's keys, from env (Secret Manager on Cloud Run):
// EXT_<PROVIDER>_API_KEY, EXT_<PROVIDER>_CLIENT_ID, EXT_<PROVIDER>_CLIENT_SECRET.
type Secrets struct {
	APIKey       string
	ClientID     string
	ClientSecret string
}

// LogValue keeps the secrets out of logs.
func (Secrets) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// String keeps the secrets out of %v.
func (Secrets) String() string { return "[redacted]" }

// Token is a person's OAuth token of one provider. It is sealed at rest and
// never logged, printed or sent to a device.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
}

// LogValue keeps the token out of logs.
func (Token) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// String keeps the token out of %v.
func (Token) String() string { return "[redacted]" }

// GoString keeps the token out of %#v.
func (Token) GoString() string { return "[redacted]" }

// MarshalJSON keeps the token out of JSON (sealing uses its own form).
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// expired reports whether the access token is past (or within skew of) its expiry.
func (t Token) expired(now time.Time) bool {
	return !t.Expiry.IsZero() && !now.Add(time.Minute).Before(t.Expiry)
}

// ErrInvalidGrant: the provider refused the refresh token (revoked on the
// provider's side). The connection turns to error; the person connects again.
var ErrInvalidGrant = errors.New("extapi: invalid grant")

// ErrNoRevoke: the provider has no revoke endpoint (Spotify ...); only the
// stored token is deleted.
var ErrNoRevoke = errors.New("extapi: provider has no revoke endpoint")

// OAuthClient is a NeedsOAuth provider's OAuth 2.0 (authorization code +
// PKCE S256). The child issues implement it per provider.
type OAuthClient interface {
	// AuthURL is the consent page to open in ASWebAuthenticationSession.
	AuthURL(state, codeChallenge, redirectURI string, scopes []string) string
	// Exchange turns the callback's code into a token.
	Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error)
	// Refresh renews the access token (ErrInvalidGrant when refused).
	Refresh(ctx context.Context, t Token) (Token, error)
	// Revoke revokes t at the provider (ErrNoRevoke when it cannot).
	Revoke(ctx context.Context, t Token) error
}
