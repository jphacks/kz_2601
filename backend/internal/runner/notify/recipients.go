// Package notify turns a planned notification (engine.PlannedNotify) into
// the people who receive it and sends it (#341, the base of #269).
//
// The run-time checks of #339 live here: the recipient kinds a scope allows,
// participants at run time, blocks (either way), deleted accounts, consent,
// new accounts and the per-recipient limits. Reads go through interfaces that
// the runner's appId-bound handle implements (#337); this package does not
// import internal/store.
package notify

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Reason says why a person did not receive a notification (or why a whole
// notification was refused). Values are stable: they go to the creator and
// to runner.usage.
type Reason string

// Reasons.
const (
	// ReasonScope: the recipient kind is not allowed for the app's scope
	// (public: owner and authorOf only). The whole notification is refused.
	ReasonScope Reason = "scope_not_allowed"
	// ReasonNoRecipient: the kind resolved to nobody (no owner, empty ref,
	// record gone, record without an author).
	ReasonNoRecipient Reason = "no_recipient"
	ReasonActor       Reason = "actor"      // the person who caused the trigger (shared / public)
	ReasonDeleted     Reason = "deleted"    // the account is deleted
	ReasonNotMember   Reason = "not_member" // not a participant at run time
	ReasonBlocked     Reason = "blocked"    // a block between the recipient and the actor (either way)
	// ReasonNewAccount: the actor's account is younger than NewAccountWindow
	// (or its age is unknown).
	ReasonNewAccount Reason = "new_account"
	ReasonNoConsent  Reason = "no_consent"
	// ReasonRecipientLimit: a per-recipient / per-run / per-app limit
	// (LimitDrop.Scope says which). Not sent later.
	ReasonRecipientLimit Reason = "recipient_limit"
	ReasonNoDevice       Reason = "no_device"    // no device token
	ReasonUnregistered   Reason = "unregistered" // every token was rejected as unregistered (410)
	ReasonSendFailed     Reason = "send_failed"  // sending failed (Outcome.Retryable says whether to retry)
)

// NewAccountWindow: an account this young cannot notify others (#339).
const NewAccountWindow = 24 * time.Hour

// Directory reads who is who in one app. The runner's appId-bound handle
// implements it, so the appId is not an argument.
type Directory interface {
	// Owner is the app's creator; "" when none is recorded.
	Owner(ctx context.Context) (string, error)
	// Members are the participants at run time (share-link members). The
	// owner may or may not be included.
	Members(ctx context.Context) ([]string, error)
	// RecordAuthor is the author of entity/id; found=false when the record
	// does not exist.
	RecordAuthor(ctx context.Context, entity, id string) (author string, found bool, err error)
}

// Accounts reads account state (store.Accounts.UserDeleted and
// store.Moderation.ListBlocks behind an adapter).
type Accounts interface {
	Deleted(ctx context.Context, uid string) (bool, error)
	// Blocks are the uids uid has blocked.
	Blocks(ctx context.Context, uid string) ([]string, error)
}

// Consent says whether uid agreed to receive this app's notifications. It is
// asked only for people other than the owner.
type Consent interface {
	Allows(ctx context.Context, uid string) (bool, error)
}

// NoConsent is the default Consent of a Resolver: nobody has agreed, so
// notifications to others are not sent. The runner passes the stored
// consents (users/{uid}/notifyConsent/{appId}, #339) instead.
type NoConsent struct{}

// Allows always reports false.
func (NoConsent) Allows(context.Context, string) (bool, error) { return false, nil }

// AccountAge gives when uid's account was created; known=false when unknown.
type AccountAge interface {
	CreatedAt(ctx context.Context, uid string) (at time.Time, known bool, err error)
}

// UnknownAge is the default AccountAge: no creation time is known, so an
// actor counts as new and cannot notify others.
type UnknownAge struct{}

// CreatedAt always reports unknown.
func (UnknownAge) CreatedAt(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

// LimitDrop is one person a RecipientLimiter left out.
type LimitDrop struct {
	UID   string
	Scope string // quota.Block.Scope (recipient_app / recipient_total / run_recipients / app_notify ...)
}

// RecipientLimiter counts recipients against the per-recipient limits (one
// app 20 / all apps 100 a day) and the per-run / per-app caps. The wiring PR
// adapts quota.Quota.Recipients (bound to the run's quota.Decision).
type RecipientLimiter interface {
	Limit(ctx context.Context, uids []string) (allowed []string, dropped []LimitDrop, err error)
}

// Unlimited lets everyone through (tests and local development only).
type Unlimited struct{}

// Limit returns uids unchanged.
func (Unlimited) Limit(_ context.Context, uids []string) ([]string, []LimitDrop, error) {
	return uids, nil, nil
}

// KindAllowed reports whether a rule on scope data may notify kind ("" scope
// = private). The table is pkg/dsl's (Validate rejects the rest when
// generating; this is the run-time enforcement).
func KindAllowed(scope dsl.EntityScope, kind string) bool {
	if scope == "" {
		scope = dsl.ScopePrivate
	}
	return dsl.RecipientAllowed(scope, kind)
}

// Resolver decides who receives a planned notification.
type Resolver struct {
	dir      Directory
	accounts Accounts
	consent  Consent
	age      AccountAge
	limiter  RecipientLimiter
}

// Config configures a Resolver. Directory, Accounts and Limiter are required.
type Config struct {
	Directory Directory
	Accounts  Accounts
	// Consent: nil = NoConsent (others are not notified).
	Consent Consent
	// Age: nil = UnknownAge (an actor with an unknown age cannot notify others).
	Age     AccountAge
	Limiter RecipientLimiter
}

// NewResolver builds a Resolver.
func NewResolver(cfg Config) (*Resolver, error) {
	if cfg.Directory == nil || cfg.Accounts == nil || cfg.Limiter == nil {
		return nil, errors.New("notify: Directory, Accounts and Limiter are required")
	}
	r := &Resolver{dir: cfg.Directory, accounts: cfg.Accounts, consent: cfg.Consent, age: cfg.Age, limiter: cfg.Limiter}
	if r.consent == nil {
		r.consent = NoConsent{}
	}
	if r.age == nil {
		r.age = UnknownAge{}
	}
	return r, nil
}

// Skip is one person not notified and why.
type Skip struct {
	UID    string
	Reason Reason
	// Limit: for ReasonRecipientLimit, the quota scope that stopped it.
	Limit string
}

// Resolution is who receives one planned notification.
type Resolution struct {
	Action int
	// Refused is set when the whole notification is not sent (To is empty).
	Refused Reason
	To      []string
	Skipped []Skip
}

// Resolve resolves n (planned for ev) to recipients and drops the people who
// must not receive it. An error means a read failed (retry the run).
func (r *Resolver) Resolve(ctx context.Context, ev engine.Trigger, n engine.PlannedNotify) (Resolution, error) {
	res := Resolution{Action: n.Action}
	scope := ev.Scope
	if scope == "" {
		scope = dsl.ScopePrivate
	}
	if !KindAllowed(scope, n.To.Kind) {
		res.Refused = ReasonScope
		return res, nil
	}
	owner, err := r.dir.Owner(ctx)
	if err != nil {
		return Resolution{}, fmt.Errorf("notify: owner: %w", err)
	}
	cands, err := r.candidates(ctx, ev, n, owner)
	if err != nil {
		return Resolution{}, err
	}
	if len(cands) == 0 {
		res.Refused = ReasonNoRecipient
		return res, nil
	}

	var members map[string]bool
	if scope == dsl.ScopeShared {
		ms, err := r.dir.Members(ctx)
		if err != nil {
			return Resolution{}, fmt.Errorf("notify: members: %w", err)
		}
		members = make(map[string]bool, len(ms)+1)
		for _, m := range ms {
			members[m] = true
		}
		if owner != "" {
			members[owner] = true
		}
	}
	var actorBlocks []string
	if ev.Actor != "" {
		if actorBlocks, err = r.accounts.Blocks(ctx, ev.Actor); err != nil {
			return Resolution{}, fmt.Errorf("notify: blocks: %w", err)
		}
	}
	newActor := false
	if ev.Actor != "" {
		if newActor, err = r.isNew(ctx, ev.Actor, ev.At); err != nil {
			return Resolution{}, err
		}
	}

	var pass []string
	for _, uid := range cands {
		reason, err := r.check(ctx, ev, scope, owner, uid, members, actorBlocks, newActor)
		if err != nil {
			return Resolution{}, err
		}
		if reason != "" {
			res.Skipped = append(res.Skipped, Skip{UID: uid, Reason: reason})
			continue
		}
		pass = append(pass, uid)
	}
	if len(pass) == 0 {
		return res, nil
	}
	allowed, dropped, err := r.limiter.Limit(ctx, pass)
	if err != nil {
		return Resolution{}, fmt.Errorf("notify: recipient limit: %w", err)
	}
	for _, d := range dropped {
		res.Skipped = append(res.Skipped, Skip{UID: d.UID, Reason: ReasonRecipientLimit, Limit: d.Scope})
	}
	res.To = allowed
	return res, nil
}

// candidates are the de-duplicated uids the kind points to.
func (r *Resolver) candidates(ctx context.Context, ev engine.Trigger, n engine.PlannedNotify, owner string) ([]string, error) {
	var uids []string
	switch n.To.Kind {
	case dsl.RecipientOwner:
		uids = []string{owner}
	case dsl.RecipientAuthor:
		uids = []string{ev.Record.Author}
	case dsl.RecipientAuthorOf:
		if n.RefEntity == "" || n.RefID == "" {
			return nil, nil
		}
		a, found, err := r.dir.RecordAuthor(ctx, n.RefEntity, n.RefID)
		if err != nil {
			return nil, fmt.Errorf("notify: author of %s: %w", n.RefEntity, err)
		}
		if found {
			uids = []string{a}
		}
	case dsl.RecipientMembers:
		ms, err := r.dir.Members(ctx)
		if err != nil {
			return nil, fmt.Errorf("notify: members: %w", err)
		}
		uids = append([]string{owner}, ms...)
	case dsl.RecipientRecipients:
		uids = n.UIDs
	}
	out := make([]string, 0, len(uids))
	for _, u := range uids {
		if u != "" && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out, nil
}

// check returns why uid must not receive it ("" = may).
func (r *Resolver) check(ctx context.Context, ev engine.Trigger, scope dsl.EntityScope, owner, uid string,
	members map[string]bool, actorBlocks []string, newActor bool) (Reason, error) {
	// the actor is not notified of their own write, except the owner of a
	// private app (a reminder to oneself is the only notification there)
	if uid == ev.Actor && scope != dsl.ScopePrivate {
		return ReasonActor, nil
	}
	deleted, err := r.accounts.Deleted(ctx, uid)
	if err != nil {
		return "", fmt.Errorf("notify: account state: %w", err)
	}
	if deleted {
		return ReasonDeleted, nil
	}
	switch scope {
	case dsl.ScopePrivate:
		if uid != owner {
			return ReasonNotMember, nil
		}
	case dsl.ScopeShared:
		if !members[uid] {
			return ReasonNotMember, nil
		}
	}
	if ev.Actor != "" && uid != ev.Actor {
		if slices.Contains(actorBlocks, uid) {
			return ReasonBlocked, nil
		}
		theirs, err := r.accounts.Blocks(ctx, uid)
		if err != nil {
			return "", fmt.Errorf("notify: blocks: %w", err)
		}
		if slices.Contains(theirs, ev.Actor) {
			return ReasonBlocked, nil
		}
		// a notification to someone other than the actor
		if newActor {
			return ReasonNewAccount, nil
		}
	}
	if uid != owner {
		ok, err := r.consent.Allows(ctx, uid)
		if err != nil {
			return "", fmt.Errorf("notify: consent: %w", err)
		}
		if !ok {
			return ReasonNoConsent, nil
		}
	}
	return "", nil
}

// isNew: created within NewAccountWindow before at, or creation unknown.
func (r *Resolver) isNew(ctx context.Context, uid string, at time.Time) (bool, error) {
	created, known, err := r.age.CreatedAt(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("notify: account age: %w", err)
	}
	if !known {
		return true, nil
	}
	return at.Sub(created) < NewAccountWindow, nil
}
