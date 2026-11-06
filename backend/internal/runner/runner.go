// Package runner runs server rules (#341): it takes record triggers from the
// outbox, checks right before running that the app and the rule may still
// run, evaluates the rule with the DSL version of the trigger (engine),
// counts it (quota), writes once per action (idempotent marks) and sends the
// notifications (notify). Everything app-specific goes through an appId-bound
// scope.Handle.
package runner

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/queue"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Outcomes of a run besides quota's (ok / error / skipped_quota /
// skipped_global / skipped_paused / skipped_plan) and queue.OutcomeGaveUp.
const (
	// OutcomeSkippedStopped: the app or rule may no longer run (Reason* says why).
	OutcomeSkippedStopped = "skipped_stopped"
	// OutcomeSkippedStale: the trigger is older than Config.MaxAge.
	OutcomeSkippedStale = "skipped_stale"
	// OutcomeMixed: the rules of one event ended differently (event level only).
	OutcomeMixed = "mixed"
)

// Reasons of OutcomeSkippedStopped.
const (
	ReasonAppDeleted     = "app_deleted"
	ReasonAppHidden      = "app_hidden"      // unpublished by the operator
	ReasonNotCreator     = "not_creator"     // the published DSL is not the creator's
	ReasonCreatorDeleted = "creator_deleted" // the creator deleted their account
	ReasonVersionMissing = "version_missing" // the trigger's DSL version is gone
	ReasonRuleRemoved    = "rule_removed"    // not in the trigger's version or not in the current DSL
	ReasonBadEvent       = "bad_event"
)

// DefaultMaxAge: triggers older than this are not run (#337: after a
// restore or a long pause, old triggers must not all run at once).
const DefaultMaxAge = time.Hour

// Store is what the runner needs from the store.
type Store interface {
	scope.Backend
	store.OutboxQueue
}

// Config configures a Runner. Env, Store and Quota are required.
type Config struct {
	// Env is APP_ENV as the outbox names it (runner/{env}/outbox).
	Env   string
	Store Store
	Quota *quota.Quota
	// Sender: nil = notify.LogSender.
	Sender notify.Sender
	// Tokens: nil = the devices in the store (users/{uid}/devices, #269)
	// whose environment is PushEnvironment, read through the run's handle.
	Tokens notify.TokenReader
	// PushEnvironment: the APNs environment Sender sends to ("" = production).
	PushEnvironment string
	// Consent: nil = the consents in the store (users/{uid}/notifyConsent,
	// #339) through the run's handle; none stored = not agreed.
	Consent notify.Consent
	// Age: nil = notify's default (age unknown).
	Age notify.AccountAge
	// Paused (RUNNER_PAUSED): events are finished as skipped_paused, never kept for later.
	Paused func() bool
	// MaxAge: 0 = DefaultMaxAge.
	MaxAge time.Duration
	// Batch: events claimed at a time; 0 = queue.DefaultBatch.
	Batch  int
	Now    func() time.Time
	Logger *slog.Logger
}

// Runner runs the outbox of one environment.
type Runner struct {
	cfg   Config
	queue *queue.Queue
}

// New builds a Runner.
func New(cfg Config) (*Runner, error) {
	if cfg.Env == "" || cfg.Store == nil || cfg.Quota == nil {
		return nil, errors.New("runner: Env, Store and Quota are required")
	}
	if cfg.Sender == nil {
		cfg.Sender = notify.LogSender{Logger: cfg.Logger}
	}
	if cfg.PushEnvironment == "" {
		cfg.PushEnvironment = store.DeviceProduction
	}
	if cfg.Paused == nil {
		cfg.Paused = func() bool { return false }
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultMaxAge
	}
	if cfg.Batch <= 0 {
		cfg.Batch = queue.DefaultBatch
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	q := queue.New(cfg.Store, cfg.Env)
	q.Now = cfg.Now
	return &Runner{cfg: cfg, queue: q}, nil
}

// Queue is the runner's queue (tests tune its delays).
func (r *Runner) Queue() *queue.Queue { return r.queue }

// TickResult counts what one Tick did.
type TickResult struct {
	Claimed int            `json:"claimed"`
	Done    int            `json:"done"`
	Retried int            `json:"retried"`
	GaveUp  int            `json:"gaveUp"`
	Events  map[string]int `json:"outcomes,omitempty"` // event outcome -> count
	// More: the budget ran out with events possibly left.
	More bool `json:"more"`
}

// Tick drains due events until the queue is empty or budget has passed
// (the batch in hand is always finished). Errors of single events are
// retried by the queue; err is a failure to claim or to store a result.
func (r *Runner) Tick(ctx context.Context, budget time.Duration) (TickResult, error) {
	deadline := time.Now().Add(budget)
	var res TickResult
	for {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if !time.Now().Before(deadline) {
			res.More = true
			return res, nil
		}
		evs, err := r.queue.Claim(ctx, r.cfg.Batch)
		if err != nil {
			return res, err
		}
		if len(evs) == 0 {
			return res, nil
		}
		res.Claimed += len(evs)
		for _, ev := range evs {
			kind, outcome, err := r.handle(ctx, ev)
			if err != nil {
				return res, err
			}
			switch kind {
			case store.OutboxDone:
				res.Done++
			case store.OutboxRetry:
				res.Retried++
			case store.OutboxGaveUp:
				res.GaveUp++
			}
			if res.Events == nil {
				res.Events = map[string]int{}
			}
			res.Events[outcome]++
		}
	}
}

// handle runs one claimed event and stores its result.
func (r *Runner) handle(ctx context.Context, ev store.OutboxEvent) (status, outcome string, err error) {
	if r.queue.Exhausted(ev) {
		for _, id := range ev.RuleIDs {
			r.logRun(ctx, ev, runLog{ruleID: id, outcome: queue.OutcomeGaveUp})
		}
		return store.OutboxGaveUp, queue.OutcomeGaveUp, r.queue.GiveUp(ctx, ev, "")
	}
	rules, perr := r.process(ctx, ev)
	if perr != nil {
		code := errorCode(perr)
		r.cfg.Logger.WarnContext(ctx, "runner: event failed; will retry",
			slog.String("event_id", ev.EventID), slog.String("app_id", ev.AppID),
			slog.Int("attempt", ev.Attempts), slog.String("error_code", code), slog.Any("error", perr))
		return store.OutboxRetry, quota.OutcomeError, r.queue.Retry(ctx, ev, code)
	}
	outcome = summarize(rules)
	return store.OutboxDone, outcome, r.queue.Done(ctx, ev, outcome, rules)
}

func summarize(rules map[string]string) string {
	out := ""
	for _, o := range rules {
		switch {
		case out == "":
			out = o
		case out != o:
			return OutcomeMixed
		}
	}
	if out == "" {
		return quota.OutcomeOK
	}
	return out
}

// errorCode is a short class of err for the outbox document (never content).
func errorCode(err error) string {
	switch {
	case errors.Is(err, errSendRetry):
		return "send_failed"
	case errors.Is(err, scope.ErrViolation):
		return "scope_violation"
	case errors.Is(err, scope.ErrNotReadable):
		return "not_readable"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	}
	return "error"
}
