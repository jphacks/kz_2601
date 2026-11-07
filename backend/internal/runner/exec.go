package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
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
// repeating what already happened) and charge is what the failed rule
// counted (refunded if the event gives up).
func (r *Runner) process(ctx context.Context, ev store.OutboxEvent) (map[string]string, quota.Charge, error) {
	var none quota.Charge
	stopLevel := ""
	all := func(outcome, reason string) map[string]string {
		out := make(map[string]string, len(ev.RuleIDs))
		for _, id := range ev.RuleIDs {
			out[id] = outcome
			r.logRun(ctx, ev, runLog{ruleID: id, outcome: outcome, reason: reason, stopLevel: stopLevel})
			r.cfg.Quota.LogUsage(ctx, quota.Usage{AppID: ev.AppID, ActorUID: ev.ActorUID, RuleID: id, Trigger: triggerName(ev.Kind), Outcome: outcome})
		}
		return out
	}
	// stopping takes effect at once, whatever the version (#337)
	if r.cfg.Paused() {
		return all(quota.OutcomeSkippedPaused, ""), none, nil
	}
	st, err := r.stopsFor(ctx, ev)
	if err != nil {
		return nil, none, err
	}
	if st.app != nil {
		stopLevel = st.app.Level // StopLevelApp | StopLevelGlobal
		return all(OutcomeSkippedStopped, ReasonOperatorStop), none, nil
	}
	if ev.Kind == store.OutboxSchedule && schedule.IsLate(ev.DueAt, r.cfg.Now(), r.cfg.MaxLate) {
		return all(OutcomeSkippedLate, ""), none, nil
	}
	if r.cfg.Now().Sub(ev.CreatedAt) > r.cfg.MaxAge {
		return all(OutcomeSkippedStale, ""), none, nil
	}
	h, err := scope.New(r.cfg.Store, r.cfg.Env, ev, r.cfg.Logger)
	if err != nil {
		return all(OutcomeSkippedStopped, ReasonBadEvent), none, nil
	}

	current, creator, reason, err := r.appState(ctx, h, ev)
	if err != nil {
		return nil, none, err
	}
	if reason != "" {
		if ev.Kind == store.OutboxSchedule && (reason == ReasonAppDeleted || reason == ReasonCreatorDeleted) {
			r.dropSchedule(ctx, ev)
		}
		return all(OutcomeSkippedStopped, reason), none, nil
	}
	if ev.Kind == store.OutboxSchedule {
		idle, err := r.inactive(ctx, h, ev)
		if err != nil {
			return nil, none, err
		}
		if idle {
			return all(OutcomeSkippedInactive, ""), none, nil
		}
	}
	v, err := h.Version(ctx, ev.Version)
	if errors.Is(err, store.ErrNotFound) {
		return all(OutcomeSkippedStopped, ReasonVersionMissing), none, nil
	}
	if err != nil {
		return nil, none, err
	}
	app, err := decodeDSL(v.DSL)
	if err != nil {
		return all(OutcomeSkippedStopped, ReasonVersionMissing), none, nil
	}
	h.Bind(app, ev.Entity, ev.RecordID, ev.Data)

	trig, err := r.trigger(ctx, h, ev, app)
	if err != nil {
		return nil, none, err
	}
	out := make(map[string]string, len(ev.RuleIDs))
	for _, id := range ev.RuleIDs {
		rule, now := ruleByID(app, id), ruleByID(current, id)
		if rule == nil || now == nil {
			out[id] = OutcomeSkippedStopped
			r.logRun(ctx, ev, runLog{ruleID: id, outcome: OutcomeSkippedStopped, reason: ReasonRuleRemoved})
			continue
		}
		if st.rules[id] {
			out[id] = r.ruleStopped(ctx, ev, rule)
			continue
		}
		o, charge, err := r.runRule(ctx, h, ev, app, rule, trig, creator, st.notify)
		if err != nil {
			return nil, charge, fmt.Errorf("rule %s: %w", id, err)
		}
		out[id] = o
	}
	return out, none, nil
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
	// shared; for a time trigger, which has no entity, no entity is shared
	// any more): queued shared triggers are dropped, whatever the version (#337)
	if h.Shared() && !stillShared(current, ev) {
		return nil, "", ReasonRuleRemoved, nil
	}
	return current, creator, "", nil
}

// stillShared reports whether the published DSL still shares the data of ev.
func stillShared(current *dsl.App, ev store.OutboxEvent) bool {
	if ev.Kind == store.OutboxSchedule {
		return slices.ContainsFunc(current.Entities, func(e dsl.Entity) bool { return e.IsShared() })
	}
	ent := current.EntityByID(ev.Entity)
	return ent != nil && ent.IsShared()
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
// at the time of the write), read in the creator's time zone. The poster's
// display name of a shared record is read from the record itself (the event
// does not carry it).
func (r *Runner) trigger(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, app *dsl.App) (engine.Trigger, error) {
	scopeOf := dsl.EntityScope(ev.Scope)
	if scopeOf == "" {
		scopeOf = dsl.ScopePrivate
	}
	loc, err := r.location(ctx, h, ev)
	if err != nil {
		return engine.Trigger{}, err
	}
	t := engine.Trigger{
		AppID: ev.AppID, Scope: scopeOf, Entity: ev.Entity, Actor: ev.ActorUID, Version: ev.Version, At: ev.CreatedAt,
		Record: expreval.Record{ID: ev.RecordID, Data: orEmpty(ev.Data), Author: ev.Author}, Location: loc,
	}
	switch ev.Kind {
	case store.OutboxSchedule:
		// the run is "at" its due time: today / now read the scheduled day
		t.Kind, t.At, t.Record = engine.KindScheduled, ev.DueAt, expreval.Record{}
		return t, nil
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

// runRule counts, plans, writes and notifies one rule. On error, charge is
// what this run counted so far (kept with the event for a refund).
// notifyPaused (NotifyPaused*, "" = not paused): the rule runs and writes
// but sends nothing; each skipped notification is marked done so a retry
// does not send it after the pause.
func (r *Runner) runRule(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, app *dsl.App, rule *dsl.ServerRule, trig engine.Trigger, creator, notifyPaused string) (outcome string, charge quota.Charge, err error) {
	start := time.Now()
	run := quota.Run{
		AppID: ev.AppID, CreatorUID: creator, Shared: h.Shared(), ActorUID: ev.ActorUID, Attempt: ev.Attempts,
		// private data: the creator is the person who wrote
		CreatorAnonymous: !h.Shared() && ev.ActorAnonymous,
	}
	if ev.Kind == store.OutboxSchedule {
		run.CreatorAnonymous = ev.ActorAnonymous // the schedule keeps the creator's
	}
	charged := &quota.Charge{}
	fail := func(err error) (string, quota.Charge, error) { return "", *charged, err }
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
		return fail(err)
	}
	charged.Parts = append(charged.Parts, dec.Charge.Parts...)
	rl.quota, rl.quotaDay = dec.AppExec, dec.Day
	if !dec.Allowed {
		if dec.Block != nil {
			usage.Limit = dec.Block.Scope
		}
		return finish(dec.Outcome), quota.Charge{}, nil
	}
	reader := &ruleReader{h: h, q: r.cfg.Quota, dec: dec, maxReads: r.cfg.Quota.RunReadCap(dec.Plan), charged: charged, usage: &usage}
	rows, err := planRows(ctx, app, rule, trig, reader)
	if errors.Is(err, engine.ErrReadLimit) {
		// stopped, never computed on part of the records (#338: stop, do not delay)
		var rle *engine.ReadLimitError
		if errors.As(err, &rle) {
			usage.Limit = rle.Scope
		}
		return finish(quota.OutcomeSkippedQuota), quota.Charge{}, nil
	}
	if err != nil {
		return fail(err)
	}

	nWrites := 0
	for _, row := range rows {
		nWrites += len(row.Plan.Writes)
	}
	grant, err := r.cfg.Quota.Writes(ctx, dec, nWrites)
	if err != nil {
		return fail(err)
	}
	charged.Parts = append(charged.Parts, grant.Charge.Parts...)
	if grant.Block != nil {
		usage.Limit = grant.Block.Scope
	}
	usage.WritesDropped = grant.Dropped
	written := 0
	for _, row := range rows {
		for _, w := range row.Plan.Writes {
			if written >= grant.Allowed {
				break
			}
			written++
			wrote, err := r.write(ctx, h, ev, rule, row.key, w)
			if err != nil {
				return fail(err)
			}
			if wrote {
				usage.Writes++
			} else {
				rl.writesSkipped++
			}
		}
	}

	var consent notify.Consent = h
	if r.cfg.Consent != nil {
		consent = r.cfg.Consent
	}
	resolver, err := notify.NewResolver(notify.Config{Directory: h, Accounts: h, Consent: consent, Age: r.cfg.Age,
		Limiter: recipientLimiter{q: r.cfg.Quota, dec: dec, charged: charged}})
	if err != nil {
		return fail(err)
	}
	var tokens notify.TokenReader = handleTokens{h: h, env: r.cfg.PushEnvironment}
	if r.cfg.Tokens != nil {
		tokens = r.cfg.Tokens
	}
	disp := notify.Dispatcher{Tokens: tokens, Sender: r.cfg.Sender}
	for _, row := range rows {
		for _, n := range row.Plan.Notifies {
			key := markKey(ev, rule, row.key, n.Action)
			done, err := h.Marked(ctx, key)
			if err != nil {
				return fail(err)
			}
			if done {
				continue
			}
			if notifyPaused != "" {
				if err := h.Mark(ctx, key); err != nil {
					return fail(err)
				}
				rl.notifyPaused, rl.notifiesSkipped = notifyPaused, rl.notifiesSkipped+1
				continue
			}
			res, err := resolver.Resolve(ctx, row.trig, n)
			if err != nil {
				return fail(err)
			}
			contentKey := ev.EventID + "/" + rule.ID
			if row.key != "" {
				contentKey += "/" + row.key
			}
			out, err := disp.Dispatch(ctx, res, notify.Content{Key: contentKey, AppID: ev.AppID, AppName: app.App.Name, Title: n.Title, Body: n.Body})
			if err != nil {
				return fail(err)
			}
			r.dropUnregistered(ctx, h, out.Unregistered)
			if out.Retryable {
				return fail(errSendRetry)
			}
			if err := h.Mark(ctx, key); err != nil {
				return fail(err)
			}
			usage.NotifyRecipients += len(out.Sent)
			for _, s := range out.Skipped {
				if s.Reason == notify.ReasonRecipientLimit {
					usage.RecipientsDropped++
				}
			}
			rl.notifies = append(rl.notifies, out.Summary())
		}
	}
	return finish(quota.OutcomeOK), quota.Charge{}, nil
}

// planRow is the plan for one row: the trigger record, or one record of
// on.each (key = its id, so marks and created ids differ per record).
type planRow struct {
	key  string
	trig engine.Trigger
	Plan engine.Plan
}

// planRows plans the rule: one row, or one per record for on.each.
func planRows(ctx context.Context, app *dsl.App, rule *dsl.ServerRule, trig engine.Trigger, reader *ruleReader) ([]planRow, error) {
	if rule.On.Each == "" {
		p, err := engine.PlanRule(ctx, app, rule, trig, reader)
		if err != nil {
			return nil, err
		}
		return []planRow{{trig: trig, Plan: p}}, nil
	}
	rps, err := engine.PlanEach(ctx, app, rule, trig, reader)
	if err != nil {
		return nil, err
	}
	out := make([]planRow, len(rps))
	for i, rp := range rps {
		out[i] = planRow{key: rp.Trigger.Record.ID, trig: rp.Trigger, Plan: rp.Plan}
	}
	return out, nil
}

// ruleReader is the run's engine.RefReader + engine.Lister: reads go
// through the appId-bound handle; lists are capped per run (maxReads records in
// all) and counted against the app's daily reads, all or nothing.
type ruleReader struct {
	h        *scope.Handle
	q        *quota.Quota
	dec      quota.Decision
	maxReads int // records one run may list; 0 = no cap
	read     int // records listed so far
	charged  *quota.Charge
	usage    *quota.Usage
}

// Record implements engine.RefReader.
func (rr *ruleReader) Record(ctx context.Context, entity, id string) (expreval.Record, bool, error) {
	return rr.h.Record(ctx, entity, id)
}

// List implements engine.Lister.
func (rr *ruleReader) List(ctx context.Context, entity string) ([]expreval.Record, error) {
	limit := 0
	if rr.maxReads > 0 {
		if limit = rr.maxReads - rr.read; limit <= 0 {
			return nil, &engine.ReadLimitError{Scope: quota.ScopeRunReads, Limit: rr.maxReads, Used: rr.read + 1}
		}
	}
	recs, err := rr.h.List(ctx, entity, limit)
	if errors.Is(err, scope.ErrTooMany) {
		rr.usage.Reads += limit + 1 // what the query read
		return nil, &engine.ReadLimitError{Scope: quota.ScopeRunReads, Limit: rr.maxReads, Used: rr.read + limit + 1}
	}
	if err != nil {
		return nil, err
	}
	rr.read += len(recs)
	rr.usage.Reads += len(recs)
	ch, blk, err := rr.q.Reads(ctx, rr.dec, len(recs))
	if err != nil {
		return nil, err
	}
	rr.charged.Parts = append(rr.charged.Parts, ch.Parts...)
	if blk != nil {
		return nil, &engine.ReadLimitError{Scope: blk.Scope, Limit: blk.Limit, Used: blk.Used}
	}
	return recs, nil
}

// markKey names the done mark of one action of one rule run (row: the
// on.each record id, "" otherwise).
func markKey(ev store.OutboxEvent, rule *dsl.ServerRule, row string, action int) string {
	k := ev.EventID + "_" + rule.ID + "_" + strconv.Itoa(action)
	if row != "" {
		k += "_" + row
	}
	return k
}

// write carries out one planned write once. wrote=false: already done by
// an earlier attempt, or the target record is gone.
func (r *Runner) write(ctx context.Context, h *scope.Handle, ev store.OutboxEvent, rule *dsl.ServerRule, row string, w engine.PlannedWrite) (bool, error) {
	switch w.Kind {
	case engine.WriteCreate:
		client := ev.EventID + "/" + rule.ID + "/" + strconv.Itoa(w.Action)
		if row != "" {
			client += "/" + row
		}
		id := store.IDFromClientID(client)
		origin := store.RecordOrigin{RuleID: rule.ID, Version: ev.Version, EventID: ev.EventID, ActorUID: ev.ActorUID}
		return h.CreateRecord(ctx, w.Entity, id, w.Values, origin)
	case engine.WriteUpdate, engine.WriteIncrement:
		applied, err := h.ApplyOnce(ctx, w.Entity, w.RecordID, markKey(ev, rule, row, w.Action), func(d map[string]any) map[string]any {
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
	switch kind {
	case store.OutboxRecordUpdated:
		return quota.TriggerRecordUpdated
	case store.OutboxSchedule:
		return quota.TriggerSchedule
	}
	return quota.TriggerRecordCreated
}

// handleTokens reads device tokens through the run's handle (#337), keeping
// those of the APNs environment the runner sends to.
type handleTokens struct {
	h   *scope.Handle
	env string
}

// Tokens implements notify.TokenReader.
func (t handleTokens) Tokens(ctx context.Context, uid string) ([]string, error) {
	return t.h.DeviceTokens(ctx, uid, t.env)
}

// dropUnregistered deletes the tokens APNs rejected as unregistered (410). A
// failure is logged only: the next send to that token gets 410 again.
func (r *Runner) dropUnregistered(ctx context.Context, h *scope.Handle, toks []notify.DeviceToken) {
	for _, t := range toks {
		if err := h.DropDeviceToken(ctx, t.UID, t.Token); err != nil {
			r.cfg.Logger.WarnContext(ctx, "runner: unregistered device token not deleted", slog.Any("error", err))
		}
	}
}

// recipientLimiter adapts quota.Recipients to notify.RecipientLimiter.
type recipientLimiter struct {
	q       *quota.Quota
	dec     quota.Decision
	charged *quota.Charge // what the grants counted (refund on give-up)
}

func (l recipientLimiter) Limit(ctx context.Context, uids []string) ([]string, []notify.LimitDrop, error) {
	g, err := l.q.Recipients(ctx, l.dec, uids)
	if err != nil {
		return nil, nil, err
	}
	if l.charged != nil {
		l.charged.Parts = append(l.charged.Parts, g.Charge.Parts...)
	}
	drops := make([]notify.LimitDrop, 0, len(g.Dropped))
	for _, d := range g.Dropped {
		drops = append(drops, notify.LimitDrop{UID: d.UID, Scope: d.Block.Scope})
	}
	return g.Allowed, drops, nil
}

type runLog struct {
	ruleID       string
	outcome      string
	reason       string
	stopLevel    string // StopLevel* of an operator_stop
	notifyPaused string // NotifyPaused* when notifications were not sent
	// notifiesSkipped: notify actions not sent because of notifyPaused
	notifiesSkipped int
	writesSkipped   int
	notifies        []notify.Summary
	took            time.Duration
	// quota: the app's execution counter of quotaDay (rule status, #347)
	quota    *quota.Block
	quotaDay string
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
	if l.stopLevel != "" {
		attrs = append(attrs, slog.String("stop_level", l.stopLevel))
	}
	if l.notifyPaused != "" {
		attrs = append(attrs, slog.String("notify_paused", l.notifyPaused), slog.Int("notifies_skipped", l.notifiesSkipped))
	}
	if l.writesSkipped > 0 {
		attrs = append(attrs, slog.Int("writes_skipped", l.writesSkipped))
	}
	if len(l.notifies) > 0 {
		attrs = append(attrs, slog.Any("notifies", l.notifies))
	}
	r.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, RunEvent, attrs...)
	r.putStatus(ctx, ev, l)
}

// putStatus overwrites the rule's status for its creator's screen (#347).
// Best effort: a failed write is logged; the run's result stands.
func (r *Runner) putStatus(ctx context.Context, ev store.OutboxEvent, l runLog) {
	if l.ruleID == "" || l.outcome == "" {
		return
	}
	uid := "" // shared / public: one status per rule
	if ev.Scope == "" || ev.Scope == string(dsl.ScopePrivate) {
		uid = ev.UID
	}
	now := r.cfg.Now().UTC()
	s := store.RuleStatus{ID: store.RuleStatusID(ev.AppID, uid, l.ruleID), AppID: ev.AppID, RuleID: l.ruleID,
		At: now, Outcome: l.outcome, Reason: l.reason, NotifyPaused: l.notifyPaused != "", ExpireAt: now.Add(store.RuleStatusTTL)}
	if l.outcome == quota.OutcomeOK {
		s.LastOKAt = now
	}
	if l.quota != nil && l.quotaDay != "" {
		s.QuotaDay, s.QuotaUsed, s.QuotaLimit = l.quotaDay, l.quota.Used, l.quota.Limit
	}
	if err := r.cfg.Store.PutRuleStatus(ctx, r.cfg.Env, s); err != nil {
		r.cfg.Logger.WarnContext(ctx, "runner: writing a rule status failed",
			slog.String("app_id", ev.AppID), slog.String("rule_id", l.ruleID), slog.Any("error", err))
	}
}
