package ai

import (
	"container/list"
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
// capped per day, per app and per (app, caller) (CreatorPaidCap). Otherwise
// the caller's own Pro decides.
//
// An aiButton prompt is compiled to an RE2 regexp (linear time): {{input}}
// is one of inputMarkers, {{field}} (a field id, the DSL id rule) any text,
// anything else (`{{ input }}` too) literal text, as the device replaces
// them (iOS AIRequestBuilder: exact "{{input}}" and "{{<field id>}}").
// A prompt is not eligible when it has more than maxScopePlaceholders field
// placeholders, or field placeholders and fewer than minScopeLiteralRunes
// runes of its own text; the field values of a request may add at most
// maxScopeFillRunes runes. The DSL has no per-field length, so only the
// total is capped.

const (
	// maxScopePlaceholders caps the field placeholders of one prompt.
	maxScopePlaceholders = 20
	// minScopeLiteralRunes: a prompt with field placeholders needs this much
	// text of its own (no whitespace, no placeholders) to be eligible.
	minScopeLiteralRunes = 20
	// maxScopeFillRunes caps the runes the field values add to the prompt.
	maxScopeFillRunes = 400
	// maxScopeTemplateRunes: a longer prompt is not eligible (the device
	// sends at most dsl.MaxAIInstructionRunes).
	maxScopeTemplateRunes = 2 * dsl.MaxAIInstructionRunes
	// maxScopeComponents caps the aiButton / aiChat components compiled per app.
	maxScopeComponents = 100
	// scopeCacheTTL / scopeCacheSize: the compiled published DSL per appId.
	scopeCacheTTL  = 60 * time.Second
	scopeCacheSize = 128
)

// inputMarkers are what the device writes for {{input}} (ja / en); the data
// itself goes in input.
var inputMarkers = []string{"下の材料", "the data below"}

var (
	placeholderRe = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
	fieldIDRe     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`) // dsl idRe
)

// wsRe is what the device trims (Swift .whitespacesAndNewlines).
const wsRe = `[\s\p{Z}\x{0B}\x{85}]`

// tmplPart is a literal, the {{input}} marker or (field) any text.
type tmplPart struct {
	lit    string
	marker bool
	field  bool
}

// parseTemplate splits an aiButton prompt into its parts.
func parseTemplate(t string) []tmplPart {
	var out []tmplPart
	last := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(t, -1) {
		var p tmplPart
		switch name := t[m[2]:m[3]]; {
		case name == "input":
			p = tmplPart{marker: true}
		case fieldIDRe.MatchString(name):
			p = tmplPart{field: true}
		default:
			continue // the device leaves it as it is
		}
		if m[0] > last {
			out = append(out, tmplPart{lit: t[last:m[0]]})
		}
		out = append(out, p)
		last = m[1]
	}
	if last < len(t) {
		out = append(out, tmplPart{lit: t[last:]})
	}
	return out
}

// template is a compiled prompt: whole matches a trimmed instruction, cut
// one the device cut at dsl.MaxAIInstructionRunes (it may stop anywhere).
// Every capture group is a field value.
type template struct {
	whole, cut *regexp.Regexp
	fields     int // field placeholders
	literal    int // runes of the prompt's own text (no whitespace)
}

func compileTemplate(t string) (*template, error) {
	parts := parseTemplate(t)
	tm := &template{}
	for _, p := range parts {
		if p.field {
			tm.fields++
		}
		for _, r := range p.lit {
			if !unicode.IsSpace(r) {
				tm.literal++
			}
		}
	}
	if tm.fields > maxScopePlaceholders {
		return tm, nil // not eligible: not compiled
	}
	var err error
	if tm.whole, err = regexp.Compile(`(?s)^` + wsRe + `*` + templateRe(parts, 0, false) + `$`); err != nil {
		return nil, err
	}
	if tm.cut, err = regexp.Compile(`(?s)^` + wsRe + `*` + templateRe(parts, 0, true)); err != nil {
		return nil, err
	}
	return tm, nil
}

// templateRe is parts[i:]. A literal's leading (trailing) whitespace is
// optional when only fields come before (after) it: they may be empty and
// the device trims the whole instruction. With cut the text may end inside
// any part.
func templateRe(parts []tmplPart, i int, cut bool) string {
	if i == len(parts) {
		if cut {
			return wsRe + `*$`
		}
		return wsRe + `*`
	}
	rest := templateRe(parts, i+1, cut)
	p := parts[i]
	if p.field {
		return `(.*?)` + rest
	}
	alts, head, tail := inputMarkers, "", ""
	if !p.marker {
		lit := p.lit
		if onlyFields(parts[:i]) {
			if t := strings.TrimLeftFunc(lit, unicode.IsSpace); t != lit {
				head, lit = wsRe+`*`, t
			}
		}
		if onlyFields(parts[i+1:]) {
			if t := strings.TrimRightFunc(lit, unicode.IsSpace); t != lit {
				tail, lit = wsRe+`*`, t
			}
		}
		alts = []string{lit}
	}
	var b strings.Builder
	b.WriteString(head + `(?:`)
	for k, a := range alts {
		if k > 0 {
			b.WriteString(`|`)
		}
		b.WriteString(regexp.QuoteMeta(a) + tail + rest)
		if cut {
			b.WriteString(`|` + prefixRe(a) + `$`)
		}
	}
	b.WriteString(`)`)
	return b.String()
}

// prefixChunk bounds the nesting of prefixRe (RE2 nests at most 1000 deep).
const prefixChunk = 64

// prefixRe matches any prefix of s (the empty one too): whole chunks, then
// a nested prefix of the next chunk.
func prefixRe(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.WriteString(`(?:`)
	for start := 0; start < len(rs); start += prefixChunk {
		if start > 0 {
			b.WriteString(`|` + regexp.QuoteMeta(string(rs[:start])))
		}
		chunk := rs[start:min(start+prefixChunk, len(rs))]
		for _, r := range chunk {
			b.WriteString(`(?:` + regexp.QuoteMeta(string(r)))
		}
		b.WriteString(strings.Repeat(`)?`, len(chunk)))
	}
	b.WriteString(`)`)
	return b.String()
}

func onlyFields(parts []tmplPart) bool {
	for _, p := range parts {
		if !p.field {
			return false
		}
	}
	return true
}

// match reports whether s is the template filled in (and, with cut, cut),
// the field values adding at most maxScopeFillRunes runes.
func (tm *template) match(s string, cut bool) bool {
	re := tm.whole
	if cut {
		re = tm.cut
	}
	if re == nil {
		return false
	}
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return false
	}
	fill := 0
	for g := 2; g+1 < len(loc); g += 2 {
		if loc[g] >= 0 {
			fill += utf8.RuneCountInString(s[loc[g]:loc[g+1]])
		}
	}
	return fill <= maxScopeFillRunes
}

// eligible: a prompt the creator may pay for (see the top of the file).
func (tm *template) eligible() bool {
	return tm.whole != nil && (tm.fields == 0 || tm.literal >= minScopeLiteralRunes)
}

// scopeComp is one aiButton / aiChat of the published DSL.
type scopeComp struct {
	chat    bool
	persona string
	task    string
	long    bool
	photo   bool
	tmpl    *template
}

// appScope is what the creator of one app pays for.
type appScope struct{ comps []scopeComp }

// newAppScope compiles the eligible components of app (nil app = none).
func newAppScope(app *dsl.App) *appScope {
	sc := &appScope{}
	if app == nil {
		return sc
	}
	for i := range app.Screens {
		eachComponent(app.Screens[i].Components, func(c *dsl.Component) {
			if len(sc.comps) >= maxScopeComponents {
				return
			}
			switch c.Type {
			case dsl.CompAiChat:
				sc.comps = append(sc.comps, scopeComp{chat: true, persona: strings.TrimSpace(c.Persona), long: true})
			case dsl.CompAiButton:
				if utf8.RuneCountInString(c.Prompt) > maxScopeTemplateRunes {
					return
				}
				tm, err := compileTemplate(c.Prompt)
				if err != nil || !tm.eligible() {
					return
				}
				task := c.Task
				if task == "" {
					task = "generate"
				}
				sc.comps = append(sc.comps, scopeComp{task: task, long: c.Length == "long", photo: c.InputPhoto != "", tmpl: tm})
			}
		})
	}
	return sc
}

// sends reports whether req is what one of the components sends.
func (sc *appScope) sends(req TextRequest) bool {
	// cut by the device at the instruction cap
	cut := utf8.RuneCountInString(req.Instruction) >= dsl.MaxAIInstructionRunes
	for _, c := range sc.comps {
		if c.sends(req, cut) {
			return true
		}
	}
	return false
}

func (c scopeComp) sends(req TextRequest, cut bool) bool {
	if req.isLong() && !c.long {
		return false
	}
	if req.isChat() {
		if !c.chat {
			return false
		}
		s := strings.TrimLeftFunc(req.Instruction, unicode.IsSpace)
		return strings.TrimRightFunc(s, unicode.IsSpace) == c.persona || (cut && strings.HasPrefix(c.persona, s))
	}
	if c.chat || (req.Image != nil && !c.photo) {
		return false
	}
	task := req.Task
	if task == "" {
		task = "generate"
	}
	return c.task == task && c.tmpl.match(req.Instruction, cut)
}

// requestInApp reports whether req is what an aiButton or aiChat of app sends.
func requestInApp(app *dsl.App, req TextRequest) bool {
	return newAppScope(app).sends(req)
}

// scopeCache keeps the compiled published DSL per appId for scopeCacheTTL,
// the least recently used out past scopeCacheSize.
type scopeCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order list.List // front = most recent
}

type scopeItem struct {
	appID string
	scope *appScope
	at    time.Time
}

func (c *scopeCache) get(appID string, now time.Time) *appScope {
	c.mu.Lock()
	defer c.mu.Unlock()
	el := c.items[appID]
	if el == nil {
		return nil
	}
	it := el.Value.(*scopeItem)
	if now.Sub(it.at) >= scopeCacheTTL || now.Before(it.at) {
		c.order.Remove(el)
		delete(c.items, appID)
		return nil
	}
	c.order.MoveToFront(el)
	return it.scope
}

func (c *scopeCache) put(appID string, sc *appScope, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]*list.Element{}
	}
	if el := c.items[appID]; el != nil {
		el.Value = &scopeItem{appID: appID, scope: sc, at: now}
		c.order.MoveToFront(el)
		return
	}
	c.items[appID] = c.order.PushFront(&scopeItem{appID: appID, scope: sc, at: now})
	for c.order.Len() > scopeCacheSize {
		old := c.order.Back()
		c.order.Remove(old)
		delete(c.items, old.Value.(*scopeItem).appID)
	}
}

// appScopeOf is appID's compiled published DSL (cached briefly); nil when
// the lookup failed.
func (s *Service) appScopeOf(ctx context.Context, appID string) *appScope {
	now := time.Now()
	if sc := s.scopes.get(appID, now); sc != nil {
		return sc
	}
	app, ok := s.publishedApp(ctx, appID)
	if !ok {
		return nil
	}
	sc := newAppScope(app)
	s.scopes.put(appID, sc, now)
	return sc
}

// publishedApp decodes appID's published DSL. ok is false when the lookup
// failed; app is nil when there is none or it does not decode. Decode only:
// an older document that today's rules would reject still counts.
func (s *Service) publishedApp(ctx context.Context, appID string) (app *dsl.App, ok bool) {
	pub, err := s.Members.GetPublicApp(ctx, appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, true
		}
		s.logger().WarnContext(ctx, "ai: published app lookup failed", slog.Any("error", err))
		return nil, false
	}
	var a dsl.App
	if err := json.Unmarshal(pub.DSL, &a); err != nil {
		s.logger().WarnContext(ctx, "ai: published app does not decode", slog.Any("error", err))
		return nil, true
	}
	return &a, true
}

// Defaults of AI_CREATOR_PAID_APP_DAILY / AI_CREATOR_PAID_MEMBER_DAILY.
const (
	DefaultCreatorPaidDaily       = 300
	DefaultCreatorPaidMemberDaily = 30
)

// scopeAppMember is details.scope of the per-(app, caller) cap.
const scopeAppMember = "app_member"

// CreatorPaidCap caps the calls a creator pays for, per app and per (app,
// caller), per day (JST). Both counters are taken in one step. Counters
// share the rate limiter's store:
//
//	rateLimits/{env}_{yyyy-mm-dd}_creator_paid_{appId}            {count, scope: app, ...}
//	rateLimits/{env}_{yyyy-mm-dd}_creator_paid_{appId}_uid_{uid}  {count, scope: app_member, ...}
type CreatorPaidCap struct {
	Daily       int             // calls per app and day; 0 = not counted
	MemberDaily int             // calls per (app, caller) and day; 0 = not counted
	Store       ratelimit.Store // nil = in memory per instance
	Env         string          // APP_ENV (counter id prefix)
	Now         func() time.Time

	once     sync.Once
	fallback *ratelimit.Memory
}

// CreatorPaidDailyFromEnv reads AI_CREATOR_PAID_APP_DAILY (unset = default, 0 = off).
func CreatorPaidDailyFromEnv(getenv func(string) string) (int, error) {
	return nonNegativeEnv(getenv, "AI_CREATOR_PAID_APP_DAILY", DefaultCreatorPaidDaily)
}

// CreatorPaidMemberDailyFromEnv reads AI_CREATOR_PAID_MEMBER_DAILY (unset = default, 0 = off).
func CreatorPaidMemberDailyFromEnv(getenv func(string) string) (int, error) {
	return nonNegativeEnv(getenv, "AI_CREATOR_PAID_MEMBER_DAILY", DefaultCreatorPaidMemberDaily)
}

func nonNegativeEnv(getenv func(string) string, key string, def int) (int, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", key, raw)
	}
	return n, nil
}

// take counts one creator-paid call of uid in appID; a 429 error when
// today's cap of the app or of uid in the app is reached (nothing counted
// then). refund gives the call back (Claude failed with 5xx). A store
// failure counts in memory instead.
func (c *CreatorPaidCap) take(ctx context.Context, log *slog.Logger, appID, uid string) (refund func(context.Context), err error) {
	refund = func(context.Context) {}
	if c == nil {
		return refund, nil
	}
	c.once.Do(func() { c.fallback = ratelimit.NewMemory() })
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	t := now().In(ratelimit.JST)
	y, m, d := t.Date()
	reset := time.Date(y, m, d+1, 0, 0, 0, 0, ratelimit.JST)
	expireAt := reset.Add(24 * time.Hour)
	env := c.Env
	if env == "" {
		env = "local"
	}
	id := env + "_" + t.Format("2006-01-02") + "_creator_paid_" + appID
	var counters []ratelimit.Counter
	if c.Daily > 0 {
		counters = append(counters, ratelimit.Counter{ID: id, Scope: "app", Limit: c.Daily})
	}
	if c.MemberDaily > 0 && uid != "" {
		counters = append(counters, ratelimit.Counter{ID: id + "_uid_" + strings.ReplaceAll(uid, "/", "_"), Scope: scopeAppMember, Limit: c.MemberDaily})
	}
	if len(counters) == 0 {
		return refund, nil
	}
	var st ratelimit.Store = c.fallback
	if c.Store != nil {
		st = c.Store
	}
	u, err := st.Consume(ctx, counters, 1, expireAt)
	if err != nil && st != ratelimit.Store(c.fallback) {
		log.WarnContext(ctx, "ai: creator-paid count failed; counting in memory", slog.Any("error", err))
		st = c.fallback
		u, err = st.Consume(ctx, counters, 1, expireAt)
	}
	if err != nil {
		return refund, nil
	}
	if u.Blocked < 0 {
		return func(ctx context.Context) {
			if err := st.Refund(ctx, counters, 1, expireAt); err != nil {
				log.WarnContext(ctx, "ai: creator-paid refund failed", slog.Any("error", err))
			}
		}, nil
	}
	blocked := counters[u.Blocked]
	msg := "このアプリの今日の AI の利用は上限に達しました。明日また使えます。"
	if blocked.Scope == scopeAppMember {
		msg = "このアプリでのあなたの今日の AI の利用は上限に達しました。明日また使えます。"
	}
	wait := int(reset.Sub(t).Seconds())
	return refund, &Error{Status: http.StatusTooManyRequests, Code: httpx.CodeRateLimited, Message: msg,
		Details: map[string]any{"scope": blocked.Scope, "limit": blocked.Limit, "used": u.Counts[u.Blocked], "remaining": 0,
			"resetAt": reset.UTC().Format(time.RFC3339), "retryAfterSec": wait}}
}

type creatorRefundKey struct{}

// withCreatorRefund keeps the refund of the creator-paid count in ctx.
func withCreatorRefund(ctx context.Context, fn func(context.Context)) context.Context {
	return context.WithValue(ctx, creatorRefundKey{}, fn)
}

// refundCreatorPaid gives the creator-paid count of ctx back when the
// request ended with a 5xx from upstream (502 / 503 / 504), like the rate
// limiter (ratelimit refund.go). A caller that went away gets nothing back.
func refundCreatorPaid(ctx context.Context, status int) {
	fn, _ := ctx.Value(creatorRefundKey{}).(func(context.Context))
	if fn == nil || ctx.Err() != nil {
		return
	}
	if status != http.StatusBadGateway && status != http.StatusServiceUnavailable && status != http.StatusGatewayTimeout {
		return
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	fn(rctx)
}
