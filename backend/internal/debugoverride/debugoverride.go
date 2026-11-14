// Package debugoverride lets allowlisted test accounts on stg ask for the Pro
// or free answer per request (#2085), so both plans can be tried without a
// sandbox purchase. It is off unless DEBUG_OVERRIDES=1 and APP_ENV=stg, and
// the process refuses to start when the flag is set anywhere else.
//
// Only the Pro decision changes. Nothing is written: not the stored
// entitlement, not the wrapped checker's cache. Coin balances are not touched.
// docs/infra/stg-plan-override.md
package debugoverride

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const (
	// Header carries the requested plan: "pro" | "free".
	Header = "X-Debug-Plan"

	EnabledEnv = "DEBUG_OVERRIDES"     // "1" turns it on (stg only)
	UIDsEnv    = "DEBUG_OVERRIDE_UIDS" // comma-separated verified uids

	// LogEvent is logged for each changed decision.
	LogEvent = "debugoverride.plan"
)

// Plan is a requested Pro answer.
type Plan string

const (
	Pro  Plan = "pro"
	Free Plan = "free"
)

// Config is the enabled override; a nil *Config means off.
type Config struct {
	uids map[string]struct{}
}

// FromEnv reads the override settings. It returns nil (off) when
// DEBUG_OVERRIDES is unset, and an error (refuse to start) when it is set
// outside APP_ENV=stg, set to anything but "1", or set without any uid.
func FromEnv(getenv func(string) string) (*Config, error) {
	flag := strings.TrimSpace(getenv(EnabledEnv))
	if flag == "" {
		return nil, nil
	}
	if env := getenv("APP_ENV"); env != "stg" {
		return nil, fmt.Errorf("%s is set but APP_ENV=%q (allowed only on stg)", EnabledEnv, env)
	}
	if flag != "1" {
		return nil, fmt.Errorf("%s=%q: want 1 or unset", EnabledEnv, flag)
	}
	c := &Config{uids: map[string]struct{}{}}
	for _, uid := range strings.Split(getenv(UIDsEnv), ",") {
		if uid = strings.TrimSpace(uid); uid != "" {
			c.uids[uid] = struct{}{}
		}
	}
	if len(c.uids) == 0 {
		return nil, errors.New(EnabledEnv + "=1 needs " + UIDsEnv)
	}
	return c, nil
}

// Len is the number of allowlisted uids (for the startup log).
func (c *Config) Len() int {
	if c == nil {
		return 0
	}
	return len(c.uids)
}

func (c *Config) allows(uid string) bool {
	if c == nil || uid == "" {
		return false
	}
	_, ok := c.uids[uid]
	return ok
}

type planKey struct{}

// Middleware carries a valid X-Debug-Plan into the request's context when
// the override is on; off (nil), it passes requests on untouched. It decides
// nothing: Checker applies the value only to the uid auth verified.
func Middleware(c *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if c == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p, ok := parse(r.Header.Get(Header)); ok {
				r = r.WithContext(context.WithValue(r.Context(), planKey{}, p))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parse(v string) (Plan, bool) {
	switch p := Plan(strings.ToLower(strings.TrimSpace(v))); p {
	case Pro, Free:
		return p, true
	}
	return "", false
}

// Requested returns the plan Middleware carried ("" when none). The ai ->
// appconfig client forwards it; appconfig applies it under its own checks.
func Requested(ctx context.Context) Plan {
	p, _ := ctx.Value(planKey{}).(Plan)
	return p
}

// ProChecker is ratelimit / paid / budget's ProChecker.
type ProChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

// Checker answers from the request's requested plan for an allowlisted,
// verified caller and otherwise asks Next. It keeps no state.
type Checker struct {
	Next ProChecker
	cfg  *Config
	log  *slog.Logger
}

// Wrap returns next unchanged when the override is off.
func Wrap(next ProChecker, c *Config, log *slog.Logger) ProChecker {
	if c == nil {
		return next
	}
	if log == nil {
		log = slog.Default()
	}
	return &Checker{Next: next, cfg: c, log: log}
}

// IsPro implements ProChecker. The requested plan applies only when uid is
// the caller auth verified for this request (auth.UID) and is allowlisted;
// another uid (an app's owner, say) is always asked of Next.
func (c *Checker) IsPro(ctx context.Context, uid string) bool {
	if p := Requested(ctx); p != "" && uid == auth.UID(ctx) && c.cfg.allows(uid) {
		where := "?"
		if _, file, line, ok := runtime.Caller(1); ok {
			where = fmt.Sprintf("%s/%s:%d", filepath.Base(filepath.Dir(file)), filepath.Base(file), line)
		}
		c.log.InfoContext(ctx, LogEvent,
			slog.String("event", LogEvent),
			slog.String("uid_hash", httpx.UIDHash(uid)),
			slog.String("plan", string(p)),
			slog.String("caller", where))
		return p == Pro
	}
	if c.Next == nil {
		return false
	}
	return c.Next.IsPro(ctx, uid)
}
