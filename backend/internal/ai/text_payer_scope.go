package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A call the creator pays for (text_payer.go) is matched against the app's
// published DSL (apps_public/{appId}): it must be what one of the app's
// aiButton / aiChat components sends, and the app's creator-paid calls are
// capped per day (CreatorPaidCap). Otherwise the caller's own Pro decides.

// requestInApp reports whether req is what an aiButton (same task, its prompt
// as the device fills it in) or an aiChat (its persona) of app sends.
func requestInApp(app *dsl.App, req TextRequest) bool {
	found := false
	for i := range app.Screens {
		eachComponent(app.Screens[i].Components, func(c *dsl.Component) {
			found = found || componentSends(c, req)
		})
	}
	return found
}

func componentSends(c *dsl.Component, req TextRequest) bool {
	if req.isLong() && c.Length != "long" {
		return false
	}
	// cut by the device at the instruction cap
	cut := utf8.RuneCountInString(req.Instruction) >= dsl.MaxAIInstructionRunes
	if req.isChat() {
		return c.Type == dsl.CompAiChat &&
			matchTemplate([]tmplPart{{alts: []string{strings.TrimSpace(c.Persona)}}}, req.Instruction, cut)
	}
	if c.Type != dsl.CompAiButton || (req.Image != nil && c.InputPhoto == "") {
		return false
	}
	task := req.Task
	if task == "" {
		task = "generate"
	}
	return c.Task == task && matchTemplate(parseTemplate(c.Prompt), req.Instruction, cut)
}

// tmplPart is a literal (one of alts) or, with field, any text.
type tmplPart struct {
	alts  []string
	field bool
}

var placeholderRe = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// inputMarkers are what the device writes for {{input}} (ja / en); the data
// itself goes in input.
var inputMarkers = []string{"下の材料", "the data below"}

// parseTemplate splits an aiButton prompt: {{input}} is one of inputMarkers,
// any other {{field}} is the field's value (any text, or the placeholder
// itself when the form has no such field).
func parseTemplate(t string) []tmplPart {
	var out []tmplPart
	last := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(t, -1) {
		if m[0] > last {
			out = append(out, tmplPart{alts: []string{t[last:m[0]]}})
		}
		if strings.TrimSpace(t[m[2]:m[3]]) == "input" {
			out = append(out, tmplPart{alts: inputMarkers})
		} else {
			out = append(out, tmplPart{field: true})
		}
		last = m[1]
	}
	if last < len(t) {
		out = append(out, tmplPart{alts: []string{t[last:]}})
	}
	return out
}

// matchTemplate reports whether s is parts filled in and trimmed (the device
// trims the whole instruction); cut = s may stop anywhere in the template.
func matchTemplate(parts []tmplPart, s string, cut bool) bool {
	failed := map[[2]int]bool{}
	onlyFields := func(i int) bool {
		for _, p := range parts[i:] {
			if !p.field {
				return false
			}
		}
		return true
	}
	var match func(i, pos int) bool
	match = func(i, pos int) bool {
		rest := s[pos:]
		if i == len(parts) {
			return strings.TrimSpace(rest) == ""
		}
		if failed[[2]int{i, pos}] {
			return false
		}
		ok := false
		if p := parts[i]; p.field {
			ok = cut // a value may run to the cut
			for j := pos; !ok && j <= len(s); j++ {
				if j < len(s) && !utf8.RuneStart(s[j]) {
					continue
				}
				ok = match(i+1, j)
			}
		} else {
			for _, a := range p.alts {
				if pos == 0 {
					a = strings.TrimLeftFunc(a, unicode.IsSpace)
				}
				switch {
				case strings.HasPrefix(rest, a) && match(i+1, pos+len(a)):
					ok = true
				case cut && strings.HasPrefix(a, rest):
					ok = true
				case rest == strings.TrimRightFunc(a, unicode.IsSpace) && onlyFields(i+1):
					ok = true
				}
				if ok {
					break
				}
			}
		}
		if !ok {
			failed[[2]int{i, pos}] = true
		}
		return ok
	}
	return match(0, 0)
}

// publishedApp decodes appID's published DSL; nil when it has none. Decode
// only: an older document that today's rules would reject still counts.
func (s *Service) publishedApp(ctx context.Context, appID string) *dsl.App {
	pub, err := s.Members.GetPublicApp(ctx, appID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.logger().WarnContext(ctx, "ai: published app lookup failed", slog.Any("error", err))
		}
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(pub.DSL, &app); err != nil {
		s.logger().WarnContext(ctx, "ai: published app does not decode", slog.Any("error", err))
		return nil
	}
	return &app
}

// DefaultCreatorPaidDaily is the default of AI_CREATOR_PAID_APP_DAILY.
const DefaultCreatorPaidDaily = 300

// CreatorPaidCap caps the calls a creator pays for, per app and day (JST).
// Counters share the rate limiter's store:
//
//	rateLimits/{env}_{yyyy-mm-dd}_creator_paid_{appId}   {count, scope, expireAt, updatedAt}
type CreatorPaidCap struct {
	Daily int             // calls per app and day; 0 = not counted
	Store ratelimit.Store // nil = in memory per instance
	Env   string          // APP_ENV (counter id prefix)
	Now   func() time.Time

	once     sync.Once
	fallback *ratelimit.Memory
}

// CreatorPaidDailyFromEnv reads AI_CREATOR_PAID_APP_DAILY (unset = default, 0 = off).
func CreatorPaidDailyFromEnv(getenv func(string) string) (int, error) {
	raw := strings.TrimSpace(getenv("AI_CREATOR_PAID_APP_DAILY"))
	if raw == "" {
		return DefaultCreatorPaidDaily, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("AI_CREATOR_PAID_APP_DAILY must be a non-negative integer, got %q", raw)
	}
	return n, nil
}

// take counts one creator-paid call of appID; a 429 error when today's cap
// is reached. A store failure counts in memory instead.
func (c *CreatorPaidCap) take(ctx context.Context, log *slog.Logger, appID string) error {
	if c == nil || c.Daily <= 0 {
		return nil
	}
	c.once.Do(func() { c.fallback = ratelimit.NewMemory() })
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	t := now().In(ratelimit.JST)
	y, m, d := t.Date()
	reset := time.Date(y, m, d+1, 0, 0, 0, 0, ratelimit.JST)
	env := c.Env
	if env == "" {
		env = "local"
	}
	counter := []ratelimit.Counter{{ID: env + "_" + t.Format("2006-01-02") + "_creator_paid_" + appID, Scope: "app", Limit: c.Daily}}
	var st ratelimit.Store = c.fallback
	if c.Store != nil {
		st = c.Store
	}
	u, err := st.Consume(ctx, counter, 1, reset.Add(24*time.Hour))
	if err != nil && st != ratelimit.Store(c.fallback) {
		log.WarnContext(ctx, "ai: creator-paid count failed; counting in memory", slog.Any("error", err))
		u, err = c.fallback.Consume(ctx, counter, 1, reset.Add(24*time.Hour))
	}
	if err != nil || u.Blocked < 0 {
		return nil
	}
	wait := int(reset.Sub(t).Seconds())
	return &Error{Status: http.StatusTooManyRequests, Code: httpx.CodeRateLimited,
		Message: "このアプリの今日の AI の利用は上限に達しました。明日また使えます。",
		Details: map[string]any{"scope": "app", "limit": c.Daily, "used": u.Counts[0], "remaining": 0,
			"resetAt": reset.UTC().Format(time.RFC3339), "retryAfterSec": wait}}
}
