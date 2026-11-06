package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// errSendRetry: a notification failed in a way a retry may fix.
var errSendRetry = errors.New("runner: notification send failed")

// RunEvent is the message (and `event` attr) of the per-run audit log.
// Cloud Logging filter: jsonPayload.event="runner.run". It carries ids,
// counts and outcomes only: no record values, no notification text, no raw uid.
const RunEvent = "runner.run"

// process runs every rule of ev. It returns the outcome of each rule; an
// error means the event should be retried (marks keep the retry from
// repeating what already happened).
func (r *Runner) process(ctx context.Context, ev store.OutboxEvent) (map[string]string, error) {
	all := func(outcome, reason string) map[string]string {
		out := make(map[string]string, len(ev.RuleIDs))
		for _, id := range ev.RuleIDs {
			out[id] = outcome
			r.logRun(ctx, ev, runLog{ruleID: id, outcome: outcome, reason: reason})
			r.cfg.Quota.LogUsage(ctx, quota.Usage{AppID: ev.AppID, ActorUID: ev.ActorUID, RuleID: id, Trigger: triggerName(ev.Kind), Outcome: outcome})
		}
		return out
	}
	// stopping takes effect at once, whatever the version (#337)
	if r.cfg.Paused() {
		return all(quota.OutcomeSkippedPaused, ""), nil
	}
	if r.cfg.Now().Sub(ev.CreatedAt) > r.cfg.MaxAge {
		return all(OutcomeSkippedStale, ""), nil
	}
	h, err := scope.New(r.cfg.Store, r.cfg.Env, ev, r.cfg.Logger)
	if err != nil {
		return all(OutcomeSkippedStopped, ReasonBadEvent), nil
	}

	current, creator, reason, err := r.appState(ctx, h, ev)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return all(OutcomeSkippedStopped, reason), nil
	}
	v, err := h.Version(ctx, ev.Version)
	if errors.Is(err, store.ErrNotFound) {
		return all(OutcomeSkippedStopped, ReasonVersionMissing), nil
	}
	if err != nil {
		return nil, err
	}
	app, err := decodeDSL(v.DSL)
	if err != nil {
		return all(OutcomeSkippedStopped, ReasonVersionMissing), nil
	}
	h.Bind(app, ev.Entity, ev.RecordID, ev.Data)

	trig, err := r.trigger(ctx, h, ev, app)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(ev.RuleIDs))
	for _, id := range ev.RuleIDs {
		rule, now := ruleByID(app, id), ruleByID(current, id)
		if rule == nil || now == nil {
			out[id] = OutcomeSkippedStopped
			r.logRun(ctx, ev, runLog{ruleID: id, outcome: OutcomeSkippedStopped, reason: ReasonRuleRemoved})
			continue
		}
		o, err := r.runRule(ctx, h, ev, app, rule, trig, creator)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", id, err)
		}
		out[id] = o
	}
	return out, nil
}

// appState checks right before running that the app may still run and
// returns the current DSL (whose rules still count) and the creator.
// reason != "" means stopped.
func (r *Runner) appState(ctx context.Context, h *scope.Handle, ev store.OutboxEvent) (current *dsl.App, creator, reason string, err error) {
	var raw json.RawMessage
	if h.Shared() {
		pub, err := h.PublicApp(ctx)
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", ReasonAppDeleted, nil
		}
		if err != nil {
			return nil, "", "", err
		}
		if pub.Hidden {
			return nil, "", ReasonAppHidden, nil
		}
		owner, err := h.Owner(ctx)
		if err != nil {
			return nil, "", "", err
		}
		if owner == "" || pub.OwnerUID != owner {
			return nil, "", ReasonNotCreator, nil
		}
		raw, creator = pub.DSL, owner
	} else {
		a, err := h.PrivateApp(ctx)
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", ReasonAppDeleted, nil
		}
		if err != nil {
			return nil, "", "", err
		}
		raw, creator = a.DSL, ev.UID
	}
	deleted, err := h.Deleted(ctx, creator)
	if err != nil {
		return nil, "", "", err
	}
	if deleted {
		return nil, "", ReasonCreatorDeleted, nil
	}
	current, err = decodeDSL(raw)
	if err != nil {
		return nil, "", ReasonAppDeleted, nil
	}
	// sharing withdrawn (the entity left the published DSL or is no longer
	// shared): queued shared triggers are dropped, whatever the version (#337)
	if ent := current.EntityByID(ev.Entity); h.Shared() && (ent == nil || !ent.IsShared()) {
		return nil, "", ReasonRuleRemoved, nil
	}
	return current, creator, "", nil
}

// decodeDSL decodes without validating (an older stored DSL that today's
// rules would reject still decodes, like appconfig's publishedDSL).
func decodeDSL(raw json.RawMessage) (*dsl.App, error) {
	a := &dsl.App{}
	if err := json.Unmarshal(raw, a); err != nil {
		return nil, err
	}
	return a, nil
}

func ruleByID(a *dsl.App, id string) *dsl.ServerRule {
	if a == nil {
		return nil
	}
	for i := range a.ServerRules {
		if a.ServerRules[i].ID == id {
			return &a.ServerRules[i]
		}
	}
	return nil
}

// trigger builds the engine trigger from the event's snapshot (the values
// at the time of the write). The poster's display name of a shared record
// is read from the record itself (the event does not carry it).
func (r *Runner) trigger(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, app *dsl.App) (engine.Trigger, error) {
	scopeOf := dsl.EntityScope(ev.Scope)
	if scopeOf == "" {
		scopeOf = dsl.ScopePrivate
	}
	t := engine.Trigger{
		AppID: ev.AppID, Scope: scopeOf, Entity: ev.Entity, Actor: ev.ActorUID, Version: ev.Version, At: ev.CreatedAt,
		Record: expreval.Record{ID: ev.RecordID, Data: orEmpty(ev.Data), Author: ev.Author},
	}
	switch ev.Kind {
	case store.OutboxRecordCreated:
		t.Kind = engine.KindCreated
	case store.OutboxRecordUpdated:
		t.Kind = engine.KindUpdated
		t.Prev = &expreval.Record{ID: ev.RecordID, Data: orEmpty(ev.Prev), Author: ev.Author}
	}
	if ent := app.EntityByID(ev.Entity); ent != nil && ent.IsShared() {
		rec, found, err := h.Record(ctx, ev.Entity, ev.RecordID)
		if err != nil {
			return t, err
		}
		if found {
			t.Record.AuthorName = rec.AuthorName
			if t.Prev != nil {
				t.Prev.AuthorName = rec.AuthorName
			}
		}
	}
	return t, nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// runRule counts, plans, writes and notifies one rule.
func (r *Runner) runRule(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, app *dsl.App, rule *dsl.ServerRule, trig engine.Trigger, creator string) (string, error) {
	start := time.Now()
	run := quota.Run{
		AppID: ev.AppID, CreatorUID: creator, Shared: h.Shared(), ActorUID: ev.ActorUID, Attempt: ev.Attempts,
		// private data: the creator is the person who wrote
		CreatorAnonymous: !h.Shared() && ev.ActorAnonymous,
	}
	usage := quota.Usage{AppID: ev.AppID, CreatorUID: creator, ActorUID: ev.ActorUID, RuleID: rule.ID, Trigger: triggerName(ev.Kind)}
	rl := runLog{ruleID: rule.ID}
	finish := func(outcome string) string {
		usage.Outcome, rl.outcome = outcome, outcome
		usage.CPUMs, rl.took = time.Since(start).Milliseconds(), time.Since(start)
		r.cfg.Quota.LogUsage(ctx, usage)
		r.logRun(ctx, ev, rl)
		return outcome
	}
	dec, err := r.cfg.Quota.Begin(ctx, run)
	if err != nil {
		return "", err
	}
	if !dec.Allowed {
		if dec.Block != nil {
			usage.Limit = dec.Block.Scope
		}
		return finish(dec.Outcome), nil
	}
	plan, err := engine.PlanRule(ctx, app, rule, trig, h)
	if err != nil {
		return "", err
	}

	grant, err := r.cfg.Quota.Writes(ctx, dec, len(plan.Writes))
	if err != nil {
		return "", err
	}
	if grant.Block != nil {
		usage.Limit = grant.Block.Scope
	}
	usage.WritesDropped = grant.Dropped
	for i, w := range plan.Writes {
		if i >= grant.Allowed {
			break
		}
		wrote, err := r.write(ctx, h, ev, rule, w)
		if err != nil {
			return "", err
		}
		if wrote {
			usage.Writes++
		} else {
			rl.writesSkipped++
		}
	}

	resolver, err := notify.NewResolver(notify.Config{Directory: h, Accounts: h, Consent: r.cfg.Consent, Age: r.cfg.Age,
		Limiter: recipientLimiter{q: r.cfg.Quota, dec: dec}})
	if err != nil {
		return "", err
	}
	disp := notify.Dispatcher{Tokens: r.cfg.Tokens, Sender: r.cfg.Sender}
	for _, n := range plan.Notifies {
		key := markKey(ev, rule, n.Action)
		done, err := h.Marked(ctx, key)
		if err != nil {
			return "", err
		}
		if done {
			continue
		}
		res, err := resolver.Resolve(ctx, trig, n)
		if err != nil {
			return "", err
		}
		out, err := disp.Dispatch(ctx, res, notify.Content{Key: ev.EventID + "/" + rule.ID, AppID: ev.AppID, AppName: app.App.Name, Title: n.Title, Body: n.Body})
		if err != nil {
			return "", err
		}
		if out.Retryable {
			return "", errSendRetry
		}
		if err := h.Mark(ctx, key); err != nil {
			return "", err
		}
		usage.NotifyRecipients += len(out.Sent)
		for _, s := range out.Skipped {
			if s.Reason == notify.ReasonRecipientLimit {
				usage.RecipientsDropped++
			}
		}
		rl.notifies = append(rl.notifies, out.Summary())
	}
	return finish(quota.OutcomeOK), nil
}

// markKey names the done mark of one action of one rule run.
func markKey(ev store.OutboxEvent, rule *dsl.ServerRule, action int) string {
	return ev.EventID + "_" + rule.ID + "_" + strconv.Itoa(action)
}

// write carries out one planned write once. wrote=false: already done by
// an earlier attempt, or the target record is gone.
func (r *Runner) write(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, rule *dsl.ServerRule, w engine.PlannedWrite) (bool, error) {
	switch w.Kind {
	case engine.WriteCreate:
		id := store.IDFromClientID(ev.EventID + "/" + rule.ID + "/" + strconv.Itoa(w.Action))
		origin := store.RecordOrigin{RuleID: rule.ID, Version: ev.Version, EventID: ev.EventID, ActorUID: ev.ActorUID}
		return h.CreateRecord(ctx, w.Entity, id, w.Values, origin)
	case engine.WriteUpdate, engine.WriteIncrement:
		applied, err := h.ApplyOnce(ctx, w.Entity, w.RecordID, markKey(ev, rule, w.Action), func(d map[string]any) map[string]any {
			if d == nil {
				d = map[string]any{}
			}
			if w.Kind == engine.WriteIncrement {
				d[w.Field] = addNumber(d[w.Field], w.By)
				return d
			}
			for k, v := range w.Values {
				if v == nil {
					delete(d, k)
				} else {
					d[k] = v
				}
			}
			return d
		})
		if errors.Is(err, store.ErrNotFound) {
			return false, nil // the record was deleted in the meantime
		}
		return applied, err
	}
	return false, fmt.Errorf("runner: unknown write kind %q", w.Kind)
}

// addNumber adds by to a stored number (missing / not a number counts as 0).
// A whole result stays an integer.
func addNumber(v any, by float64) any {
	var cur float64
	switch n := v.(type) {
	case int64:
		cur = float64(n)
	case int:
		cur = float64(n)
	case float64:
		cur = n
	case json.Number:
		cur, _ = n.Float64()
	}
	sum := cur + by
	if sum == math.Trunc(sum) && math.Abs(sum) < 1<<53 {
		return int64(sum)
	}
	return sum
}

func triggerName(kind string) string {
	if kind == store.OutboxRecordUpdated {
		return quota.TriggerRecordUpdated
	}
	return quota.TriggerRecordCreated
}

// recipientLimiter adapts quota.Recipients to notify.RecipientLimiter.
type recipientLimiter struct {
	q   *quota.Quota
	dec quota.Decision
}

func (l recipientLimiter) Limit(ctx context.Context, uids []string) ([]string, []notify.LimitDrop, error) {
	g, err := l.q.Recipients(ctx, l.dec, uids)
	if err != nil {
		return nil, nil, err
	}
	drops := make([]notify.LimitDrop, 0, len(g.Dropped))
	for _, d := range g.Dropped {
		drops = append(drops, notify.LimitDrop{UID: d.UID, Scope: d.Block.Scope})
	}
	return g.Allowed, drops, nil
}

type runLog struct {
	ruleID        string
	outcome       string
	reason        string
	writesSkipped int
	notifies      []notify.Summary
	took          time.Duration
}

// logRun writes one runner.run line.
func (r *Runner) logRun(ctx context.Context, ev store.OutboxEvent, l runLog) {
	attrs := []slog.Attr{
		slog.String("event", RunEvent),
		slog.String("event_id", ev.EventID),
		slog.String("app_id", ev.AppID),
		slog.String("rule_id", l.ruleID),
		slog.Int("version", ev.Version),
		slog.String("trigger", triggerName(ev.Kind)),
		slog.String("scope", ev.Scope),
		slog.Int("attempt", ev.Attempts),
		slog.String("outcome", l.outcome),
		slog.Int64("duration_ms", l.took.Milliseconds()),
	}
	if l.reason != "" {
		attrs = append(attrs, slog.String("reason", l.reason))
	}
	if l.writesSkipped > 0 {
		attrs = append(attrs, slog.Int("writes_skipped", l.writesSkipped))
	}
	if len(l.notifies) > 0 {
		attrs = append(attrs, slog.Any("notifies", l.notifies))
	}
	r.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, RunEvent, attrs...)
}
