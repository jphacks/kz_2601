// Package calls is the voice / video calls of a shared app (#447,
// docs/backend/features/calls.md). The media goes through LiveKit Cloud;
// this package only issues the room tokens and charges the minutes.
//
//	POST /v1/apps/{appId}/calls/token  (appconfig, authenticated)  -> a LiveKit token for the app's room
//	POST /v1/calls/webhook             (public, LiveKit-signed)    -> participant joined / left, room finished
//	callSessions/{sessionId}           one person's stay in a room: payer, coin holds, join / leave, charges
//
// Coins (#630): every token reserves a block of minutes from the caller's
// own wallet (COIN_PER_CALL_MINUTE × CALL_BLOCK_MINUTES); the app renews the
// block before it runs out. The webhook settles the actual minutes over the
// holds once (idempotent); a hold nobody settles expires and goes back
// (the coin ledger's TTL), so a lost webhook never over-charges.
//
// Everything is behind CALLS_ENABLED (default off: no routes) and
// LIVEKIT_URL / LIVEKIT_API_KEY / LIVEKIT_API_SECRET (never in the repo).
package calls

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults. Prices are never here: COIN_PER_CALL_MINUTE defaults to 0.
const (
	DefaultBlockMinutes    = 30
	DefaultHoldGrace       = 10 * time.Minute
	DefaultTokenTTL        = 10 * time.Minute
	DefaultMaxParticipants = 8
	DefaultTokensPerHour   = 30
)

// Config is CALLS_* / LIVEKIT_* / COIN_PER_CALL_MINUTE.
type Config struct {
	Enabled   bool
	URL       string // wss://<project>.livekit.cloud
	APIKey    string
	APISecret string
	// PerMinute is the coins one call minute costs (0 = not counted).
	PerMinute int64
	// BlockMinutes is how many minutes one token reserves.
	BlockMinutes int
	// HoldGrace is added to a block's hold TTL (the webhook's delay).
	HoldGrace time.Duration
	// TokenTTL is how long a token can be used to join.
	TokenTTL        time.Duration
	MaxParticipants int
	// TokensPerHour caps token requests per user (0 = off).
	TokensPerHour int
}

// ConfigFromEnv reads:
//
//	CALLS_ENABLED           1 / true turns calls on (default off)
//	LIVEKIT_URL             wss URL of the LiveKit project (required when on)
//	LIVEKIT_API_KEY         API key (required when on)
//	LIVEKIT_API_SECRET      API secret (required when on; signs tokens, verifies webhooks)
//	COIN_PER_CALL_MINUTE    coins per minute in a call (default 0 = not counted)
//	CALL_BLOCK_MINUTES      minutes reserved per token (default 30, 1..240)
//	CALL_HOLD_GRACE         extra hold lifetime after a block, e.g. 10m (default 10m)
//	CALL_TOKEN_TTL          token lifetime to join, e.g. 10m (default 10m, 1m..1h)
//	CALL_MAX_PARTICIPANTS   people in one room (default 8, 2..50)
//	CALL_TOKENS_PER_HOUR    token requests per user per hour (default 30, 0 = off)
//
// With CALLS_ENABLED off the rest is not read and nothing is an error.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	c := Config{BlockMinutes: DefaultBlockMinutes, HoldGrace: DefaultHoldGrace, TokenTTL: DefaultTokenTTL,
		MaxParticipants: DefaultMaxParticipants, TokensPerHour: DefaultTokensPerHour}
	switch strings.ToLower(strings.TrimSpace(getenv("CALLS_ENABLED"))) {
	case "", "0", "false", "off":
		return c, nil
	case "1", "true", "on":
		c.Enabled = true
	default:
		return c, fmt.Errorf("CALLS_ENABLED must be true or false, got %q", getenv("CALLS_ENABLED"))
	}
	c.URL = strings.TrimSpace(getenv("LIVEKIT_URL"))
	c.APIKey = strings.TrimSpace(getenv("LIVEKIT_API_KEY"))
	c.APISecret = strings.TrimSpace(getenv("LIVEKIT_API_SECRET"))
	if c.URL == "" || c.APIKey == "" || c.APISecret == "" {
		return c, fmt.Errorf("CALLS_ENABLED needs LIVEKIT_URL, LIVEKIT_API_KEY and LIVEKIT_API_SECRET")
	}
	if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "wss" && u.Scheme != "https") || u.Host == "" {
		return c, fmt.Errorf("LIVEKIT_URL must be a wss:// (or https://) URL")
	}
	var err error
	intIn := func(key string, dst *int, lo, hi int) {
		v := strings.TrimSpace(getenv(key))
		if err != nil || v == "" {
			return
		}
		n, e := strconv.Atoi(v)
		if e != nil || n < lo || n > hi {
			err = fmt.Errorf("%s must be an integer %d..%d, got %q", key, lo, hi, v)
			return
		}
		*dst = n
	}
	durIn := func(key string, dst *time.Duration, lo, hi time.Duration) {
		v := strings.TrimSpace(getenv(key))
		if err != nil || v == "" {
			return
		}
		d, e := time.ParseDuration(v)
		if e != nil || d < lo || d > hi {
			err = fmt.Errorf("%s must be a duration %s..%s, got %q", key, lo, hi, v)
			return
		}
		*dst = d
	}
	intIn("CALL_BLOCK_MINUTES", &c.BlockMinutes, 1, 240)
	intIn("CALL_MAX_PARTICIPANTS", &c.MaxParticipants, 2, 50)
	intIn("CALL_TOKENS_PER_HOUR", &c.TokensPerHour, 0, 10000)
	durIn("CALL_HOLD_GRACE", &c.HoldGrace, 0, 24*time.Hour)
	durIn("CALL_TOKEN_TTL", &c.TokenTTL, time.Minute, time.Hour)
	if err != nil {
		return c, err
	}
	if v := strings.TrimSpace(getenv("COIN_PER_CALL_MINUTE")); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 0 {
			return c, fmt.Errorf("COIN_PER_CALL_MINUTE must be an integer ≥ 0, got %q", v)
		}
		c.PerMinute = n
	}
	return c, nil
}

// BlockCoins is the hold of one block (0 = calls are not charged).
func (c Config) BlockCoins() int64 { return c.PerMinute * int64(c.BlockMinutes) }

// BlockDuration is the minutes one token reserves.
func (c Config) BlockDuration() time.Duration { return time.Duration(c.BlockMinutes) * time.Minute }
