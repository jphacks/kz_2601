// Package plan is the one table of what each plan may use (#317): the free
// plan and Tailor It Pro. docs/backend/rate-limit.md "プランごとの上限"
//
// The AI units per day are enforced by internal/ratelimit (RATE_LIMIT_*);
// they are read into the table so every limit is listed in one place.
// Apps, records and photos are enforced by appconfig when it saves (apps:
// 402 app_limit_reached, appconfig/app_slots.go).
//
// The plan is the payer's: the caller for its own apps, the app's creator for
// an app someone else made (the creator pays, participants use it for free:
// docs/00_decisions.md "課金・Pro").
package plan

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Plan names (details.plan of plan_limit_reached).
type Plan string

// Plans.
const (
	Free Plan = "free"
	Pro  Plan = "pro"
)

// Resources (details.resource of plan_limit_reached).
const (
	Apps          = "apps"
	RecordsPerApp = "recordsPerApp"
	PhotosPerApp  = "photosPerApp"
)

// Limits are one plan's column. 0 = not counted.
type Limits struct {
	// AIUnitsDaily is the ratelimit units per day (signed-in user / Pro).
	// Shown here; ratelimit enforces it.
	AIUnitsDaily int
	// Apps a user may hold (#630 app slots): the apps in their list, both the
	// ones they created and the shared ones they joined (users/{uid}/apps past
	// version 1; a blank, never-generated app is not counted). Coin-bought
	// slots (coins.Wallet.AppSlots) are added on top.
	Apps int
	// RecordsPerApp is the records one user sees in one app: its own records
	// plus the app's shared ones, across every entity.
	RecordsPerApp int
	// PhotosPerApp is the photo values in the same records. One photo is a
	// data URL of at most 300 KB (an 800px JPEG), so this bounds the bytes.
	PhotosPerApp int
}

// Of returns the limit of resource (0 for an unknown one).
func (l Limits) Of(resource string) int {
	switch resource {
	case Apps:
		return l.Apps
	case RecordsPerApp:
		return l.RecordsPerApp
	case PhotosPerApp:
		return l.PhotosPerApp
	}
	return 0
}

// Table is every plan's limits and how hard they are enforced.
type Table struct {
	Free, Pro Limits
	Mode      Mode
}

// For returns p's column.
func (t Table) For(p Plan) Limits {
	if p == Pro {
		return t.Pro
	}
	return t.Free
}

// Allows reports whether one more of resource fits when used are in use.
func (t Table) Allows(p Plan, resource string, used int) bool {
	limit := t.For(p).Of(resource)
	return limit <= 0 || used < limit
}

// Default: the held apps are the owner's decision of 2026-11-10 (free 2, Pro
// 5; more with coin-bought slots). The rest is loose on purpose: it only
// stops runaway use until the values are set from the measured cost (#298)
// and the price (#325).
var Default = Table{
	Free: Limits{AIUnitsDaily: ratelimit.DefaultLimits.SignedIn, Apps: 2, RecordsPerApp: 5000, PhotosPerApp: 500},
	Pro:  Limits{AIUnitsDaily: ratelimit.DefaultLimits.Pro, Apps: 5, RecordsPerApp: 50000, PhotosPerApp: 5000},
	Mode: ModeEnforce,
}

// FromEnv reads the table (unset = Default; 0 = not counted):
//
//	PLAN_FREE_APPS / PLAN_PRO_APPS                       (2 / 5)
//	PLAN_FREE_RECORDS_PER_APP / PLAN_PRO_RECORDS_PER_APP (5000 / 50000)
//	PLAN_FREE_PHOTOS_PER_APP / PLAN_PRO_PHOTOS_PER_APP   (500 / 5000)
//	PLAN_LIMITS_MODE                                     enforce | log | off
//	RATE_LIMIT_USER_DAILY / RATE_LIMIT_PRO_DAILY         AIUnitsDaily (ratelimit)
func FromEnv(getenv func(string) string) (Table, error) {
	t := Default
	rl, err := ratelimit.ConfigFromEnv(getenv)
	if err != nil {
		return t, err
	}
	t.Free.AIUnitsDaily, t.Pro.AIUnitsDaily = rl.Limits.SignedIn, rl.Limits.Pro
	fields := []struct {
		key string
		dst *int
	}{
		{"PLAN_FREE_APPS", &t.Free.Apps},
		{"PLAN_PRO_APPS", &t.Pro.Apps},
		{"PLAN_FREE_RECORDS_PER_APP", &t.Free.RecordsPerApp},
		{"PLAN_PRO_RECORDS_PER_APP", &t.Pro.RecordsPerApp},
		{"PLAN_FREE_PHOTOS_PER_APP", &t.Free.PhotosPerApp},
		{"PLAN_PRO_PHOTOS_PER_APP", &t.Pro.PhotosPerApp},
	}
	for _, f := range fields {
		raw := strings.TrimSpace(getenv(f.key))
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return t, fmt.Errorf("%s must be a non-negative integer, got %q", f.key, raw)
		}
		*f.dst = n
	}
	t.Mode, err = modeFromEnv(getenv)
	return t, err
}

// Mode is PLAN_LIMITS_MODE, the same steps as PRO_GATE_MODE (#316).
type Mode string

// Modes.
const (
	ModeEnforce Mode = "enforce" // 403 plan_limit_reached
	ModeLog     Mode = "log"     // nothing is stopped; plan_limit.would_block is logged
	ModeOff     Mode = "off"     // not counted (the behavior before #317)
)

// ModeEnv is the environment variable of Mode.
const ModeEnv = "PLAN_LIMITS_MODE"

// WouldBlockEvent is logged for every stop (blocked=false in log mode).
const WouldBlockEvent = "plan_limit.would_block"

func modeFromEnv(getenv func(string) string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(getenv(ModeEnv)))); m {
	case "":
		return ModeEnforce, nil
	case ModeEnforce, ModeLog, ModeOff:
		return m, nil
	default:
		return ModeEnforce, fmt.Errorf("%s must be enforce|log|off, got %q", ModeEnv, getenv(ModeEnv))
	}
}

// Checks reports whether the limits are counted at all.
func (m Mode) Checks() bool { return m != ModeOff }

// Stop logs plan_limit.would_block (payer = whose plan ran out) and reports
// whether to really stop (enforce; the zero Mode counts as enforce).
func (m Mode) Stop(ctx context.Context, p Plan, resource, payer string, used, limit int) bool {
	if m == ModeOff {
		return false
	}
	enforce := m != ModeLog
	slog.InfoContext(ctx, WouldBlockEvent,
		slog.String("event", WouldBlockEvent),
		slog.Bool("blocked", enforce),
		slog.String("plan", string(p)),
		slog.String("resource", resource),
		slog.String("uid_hash", httpx.UIDHash(payer)),
		slog.Int("used", used),
		slog.Int("limit", limit),
	)
	return enforce
}

// Message is the Japanese reason of plan_limit_reached. byCreator: the
// caller uses someone else's app, whose creator's plan ran out.
func Message(resource string, limit int, upgrade, byCreator bool) string {
	what := map[string]string{
		Apps:          fmt.Sprintf("持てるアプリの数(%d 個)", limit),
		RecordsPerApp: fmt.Sprintf("このアプリに保存できるデータの件数(%d 件)", limit),
		PhotosPerApp:  fmt.Sprintf("このアプリに保存できる写真の枚数(%d 枚)", limit),
	}[resource]
	switch {
	case byCreator && upgrade:
		return "このアプリを作った人のプランの上限に達しました: " + what + "。作った人が Tailor It Pro にすると上限が上がります。"
	case byCreator:
		return "このアプリを作った人のプランの上限に達しました: " + what + "。古いデータを消すと続けて保存できます。"
	case upgrade:
		return "無料プランの上限に達しました: " + what + "。使わないものを消すか、Tailor It Pro にすると上限が上がります。"
	}
	return "プランの上限に達しました: " + what + "。使わないものを消すと続けて使えます。"
}
