package extapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Defaults that reveal nothing about prices.
const (
	// DefaultTimeout is one call a person pressed (8 s in the design).
	DefaultTimeout = 8 * time.Second
	// DefaultMaxConcurrent is the calls one runner instance runs at once,
	// apart from the tick (a full semaphore is ext_busy).
	DefaultMaxConcurrent = 8
	// DefaultUnusedTTL deletes a connection not used for 90 days (決定 7).
	DefaultUnusedTTL = 90 * 24 * time.Hour
	// MaxBody is the most a provider's response may be (1 MB).
	MaxBody = 1 << 20
)

// Config is the external API settings. Prices come from env set by the
// owner; the defaults are 0 (= not known).
type Config struct {
	// Disabled is EXT_DISABLED: every provider is off.
	Disabled bool
	// Production: APP_ENV=production. Fake providers never run there.
	Production bool
	// Enabled are the providers switched on (EXT_<PROVIDER>_ENABLED; default off).
	Enabled map[string]bool
	// Secrets per provider (EXT_<PROVIDER>_API_KEY / _CLIENT_ID / _CLIENT_SECRET).
	Secrets map[string]Secrets
	// CostUSD per "provider.op" (EXT_COST_USD_<PROVIDER>_<OP>): the cost of
	// one call when the op does not measure it.
	CostUSD map[string]float64
	// Reserve per provider (COIN_RESERVE_EXT_<PROVIDER>): the coins a paid
	// call holds. 0 under COIN_BILLING_MODE=enforce keeps the provider off.
	Reserve map[string]int64
	// Timeout of one call (EXT_TIMEOUT, default 8s).
	Timeout time.Duration
	// MaxConcurrent calls per instance (EXT_MAX_CONCURRENT, default 8).
	MaxConcurrent int
	// UnusedTTL deletes a connection not used for this long (90 days).
	UnusedTTL time.Duration
	// CallbackBase is the runner's https origin; the OAuth redirect URI is
	// CallbackBase + /oauth/callback/{provider} (EXT_OAUTH_CALLBACK_BASE).
	// "" keeps OAuth providers off.
	CallbackBase string
	// AppReturnURL is where the callback sends the device back
	// (EXT_OAUTH_APP_RETURN, default tailorit://connected).
	AppReturnURL string
	// Limits per provider (EXT_LIMIT_* / EXT_GLOBAL_DAILY*, limits.go).
	// A provider missing here is not limited (tests); ConfigFromEnv fills
	// every provider with DefaultLimits.
	Limits map[string]Limits
	// UpstreamPause is how long a provider is not called after it answered
	// 429 (ConfigFromEnv: UpstreamPause, 1 minute; 0 = no pause).
	UpstreamPause time.Duration
	// Env prefixes the limit counters (APP_ENV: stg and prod share Firestore).
	Env string
}

// ConfigFromEnv reads:
//
//	EXT_DISABLED=1                   every provider off (the kill switch)
//	EXT_<PROVIDER>_ENABLED=1         a provider on (default off)
//	EXT_<PROVIDER>_API_KEY           the operator's key (Secret Manager)
//	EXT_<PROVIDER>_CLIENT_ID / _CLIENT_SECRET  OAuth client (Secret Manager; unset: Provider.ClientFrom's)
//	EXT_COST_USD_<PROVIDER>_<OP>     USD of one call when the op does not measure it (default 0 = unknown)
//	COIN_RESERVE_EXT_<PROVIDER>      coins a paid call holds (default 0)
//	EXT_TIMEOUT                      one call, e.g. 8s (default 8s, 1s-60s)
//	EXT_MAX_CONCURRENT               calls per instance (default 8)
//	EXT_OAUTH_CALLBACK_BASE          https origin of the runner's /oauth/callback (default none = OAuth off)
//	EXT_OAUTH_APP_RETURN             where the callback returns the device (default tailorit://connected)
//	EXT_LIMIT_<SCOPE> / EXT_LIMIT_<PROVIDER>_<SCOPE>   calls per user / app per hour / day (limits.go)
//	EXT_GLOBAL_DAILY / EXT_GLOBAL_DAILY_<PROVIDER>     a provider's calls per day, everyone together
//	APP_ENV                          production never runs fake providers
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	c := Config{
		Disabled:      flag(getenv("EXT_DISABLED")),
		Production:    strings.EqualFold(strings.TrimSpace(getenv("APP_ENV")), "production"),
		Enabled:       map[string]bool{},
		Secrets:       map[string]Secrets{},
		CostUSD:       map[string]float64{},
		Reserve:       map[string]int64{},
		Timeout:       DefaultTimeout,
		MaxConcurrent: DefaultMaxConcurrent,
		UnusedTTL:     DefaultUnusedTTL,
		CallbackBase:  strings.TrimRight(strings.TrimSpace(getenv("EXT_OAUTH_CALLBACK_BASE")), "/"),
		AppReturnURL:  strings.TrimSpace(getenv("EXT_OAUTH_APP_RETURN")),
		Env:           strings.TrimSpace(getenv("APP_ENV")),
		UpstreamPause: UpstreamPause,
	}
	limits, err := limitsFromEnv(getenv)
	if err != nil {
		return c, err
	}
	c.Limits = limits
	if c.AppReturnURL == "" {
		c.AppReturnURL = "tailorit://connected"
	}
	if c.CallbackBase != "" && !strings.HasPrefix(c.CallbackBase, "https://") && !strings.HasPrefix(c.CallbackBase, "http://localhost") {
		return c, fmt.Errorf("EXT_OAUTH_CALLBACK_BASE must be https://…, got %q", c.CallbackBase)
	}
	for _, p := range Providers() {
		n := p.EnvName()
		c.Enabled[p.ID] = flag(getenv("EXT_" + n + "_ENABLED"))
		c.Secrets[p.ID] = Secrets{
			APIKey:       strings.TrimSpace(getenv("EXT_" + n + "_API_KEY")),
			ClientID:     strings.TrimSpace(getenv("EXT_" + n + "_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(getenv("EXT_" + n + "_CLIENT_SECRET")),
		}
		for _, o := range p.Ops {
			key := "EXT_COST_USD_" + n + "_" + envName(o.Name)
			if v := strings.TrimSpace(getenv(key)); v != "" {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
					return c, fmt.Errorf("%s must be a number ≥ 0, got %q", key, v)
				}
				c.CostUSD[p.ID+"."+o.Name] = f
			}
		}
		key := "COIN_RESERVE_EXT_" + n
		if v := strings.TrimSpace(getenv(key)); v != "" {
			r, err := strconv.ParseInt(v, 10, 64)
			if err != nil || r < 0 {
				return c, fmt.Errorf("%s must be an integer ≥ 0, got %q", key, v)
			}
			c.Reserve[p.ID] = r
		}
	}
	for _, p := range Providers() {
		if s := c.Secrets[p.ID]; p.ClientFrom != "" && s.ClientID == "" && s.ClientSecret == "" {
			from := c.Secrets[p.ClientFrom]
			s.ClientID, s.ClientSecret = from.ClientID, from.ClientSecret
			c.Secrets[p.ID] = s
		}
	}
	if v := strings.TrimSpace(getenv("EXT_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Second || d > time.Minute {
			return c, fmt.Errorf("EXT_TIMEOUT must be 1s-60s, got %q", v)
		}
		c.Timeout = d
	}
	if v := strings.TrimSpace(getenv("EXT_MAX_CONCURRENT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 256 {
			return c, fmt.Errorf("EXT_MAX_CONCURRENT must be 1-256, got %q", v)
		}
		c.MaxConcurrent = n
	}
	return c, nil
}

func flag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on":
		return true
	}
	return false
}
