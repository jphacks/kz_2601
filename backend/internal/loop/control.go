package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Stages that can be stopped one by one (#375). Each runs only while its env
// value is "on" and no Firestore switch says off; the default is stopped
// (fail closed, #365 decision 8).
const (
	StageIntake  = "intake"  // ingest
	StageTriage  = "triage"  // LLM classification
	StageDraft   = "draft"   // build issue drafts
	StagePublish = "publish" // post an approved draft to GitHub
	StageNotify  = "notify"  // 「あなたの声で直りました」
)

// Stages lists every stage.
var Stages = []string{StageIntake, StageTriage, StageDraft, StagePublish, StageNotify}

// SwitchAll is the Firestore switch that stops every stage at once.
const SwitchAll = "all"

// Event is loop_audit/{id}: one step of the loop. It holds ids, codes and
// counts only, never feedback text. It is the only place a feedback id is
// tied to an issue (#367: 対応は非公開の監査ログにだけ).
type Event struct {
	ID         string    `firestore:"-"`
	At         time.Time `firestore:"at"`
	Stage      string    `firestore:"stage"`
	Action     string    `firestore:"action"`
	Actor      string    `firestore:"actor"` // "loop" or the person
	ItemID     string    `firestore:"itemId,omitempty"`
	FeedbackID string    `firestore:"feedbackId,omitempty"`
	DraftID    string    `firestore:"draftId,omitempty"`
	IssueRepo  string    `firestore:"issueRepo,omitempty"`
	Issue      int       `firestore:"issue,omitempty"`
	PR         int       `firestore:"pr,omitempty"`
	Reason     string    `firestore:"reason,omitempty"` // a code
	Tokens     int       `firestore:"tokens,omitempty"`
	BodyHash   string    `firestore:"bodyHash,omitempty"`
	ExpiresAt  time.Time `firestore:"expiresAt"`
}

// ActorLoop is the actor of automatic steps.
const ActorLoop = "loop"

// AuditQuery filters ListAudit.
type AuditQuery struct {
	Since   time.Time
	DraftID string
	ItemID  string
	Limit   int // <= 0 = 200
}

// Control keeps the audit log, the stop switches and the LLM usage.
type Control interface {
	AppendAudit(ctx context.Context, e Event) error
	// ListAudit returns events oldest first.
	ListAudit(ctx context.Context, q AuditQuery) ([]Event, error)
	// Switches returns the switches set in Firestore (stage or SwitchAll ->
	// on). A stage that is absent is not stopped by Firestore.
	Switches(ctx context.Context) (map[string]bool, error)
	SetSwitch(ctx context.Context, name string, on bool, by string, at time.Time) error
	// AddUsage adds tokens to month ("2006-01") and returns the new total.
	AddUsage(ctx context.Context, month string, tokens int) (int, error)
	Usage(ctx context.Context, month string) (int, error)
}

// Month is the usage bucket of t (UTC).
func Month(t time.Time) string { return t.UTC().Format("2006-01") }

// ErrStopped is a stage that may not run now.
var ErrStopped = errors.New("loop stage stopped")

// DefaultMonthlyTokenCap is the triage LLM's monthly token cap when
// LOOP_MONTHLY_TOKEN_CAP is unset (#365 選択肢 5: 月の上限で止める).
const DefaultMonthlyTokenCap = 3_000_000

// Gate decides whether a stage may run and writes the audit log.
type Gate struct {
	Control Control
	Getenv  func(string) string
	Now     func() time.Time
	// Logger receives one structured line per event (ids and codes only).
	Logger *slog.Logger
}

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now().UTC()
	}
	return time.Now().UTC()
}

// EnvName is the env value that turns stage on ("LOOP_TRIAGE").
func EnvName(stage string) string { return "LOOP_" + strings.ToUpper(stage) }

// Allow returns nil when stage may run, or an ErrStopped error saying why.
// Anything unreadable stops it.
func (g *Gate) Allow(ctx context.Context, stage string) error {
	if g.Getenv("LOOP_PAUSED") == "1" {
		return fmt.Errorf("%w: LOOP_PAUSED=1", ErrStopped)
	}
	if g.Getenv(EnvName(stage)) != "on" {
		return fmt.Errorf("%w: %s is not on", ErrStopped, EnvName(stage))
	}
	sw, err := g.Control.Switches(ctx)
	if err != nil {
		return fmt.Errorf("%w: switches unreadable", ErrStopped)
	}
	if on, ok := sw[SwitchAll]; ok && !on {
		return fmt.Errorf("%w: loop_control/switches.all is off", ErrStopped)
	}
	if on, ok := sw[stage]; ok && !on {
		return fmt.Errorf("%w: loop_control/switches.%s is off", ErrStopped, stage)
	}
	return nil
}

// TokenCap is LOOP_MONTHLY_TOKEN_CAP (0 or invalid = the default).
func (g *Gate) TokenCap() int {
	if n, err := strconv.Atoi(g.Getenv("LOOP_MONTHLY_TOKEN_CAP")); err == nil && n > 0 {
		return n
	}
	return DefaultMonthlyTokenCap
}

// AllowLLM is Allow(triage) plus the monthly token cap.
func (g *Gate) AllowLLM(ctx context.Context) error {
	if err := g.Allow(ctx, StageTriage); err != nil {
		return err
	}
	used, err := g.Control.Usage(ctx, Month(g.now()))
	if err != nil {
		return fmt.Errorf("%w: usage unreadable", ErrStopped)
	}
	if used >= g.TokenCap() {
		return fmt.Errorf("%w: monthly token cap reached (%d >= %d)", ErrStopped, used, g.TokenCap())
	}
	return nil
}

// Record writes e (At, ID, ExpiresAt filled in) to the audit log and the
// structured log. A failed write is returned: a step that cannot be audited
// does not go on.
func (g *Gate) Record(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = g.now()
	}
	if e.Actor == "" {
		e.Actor = ActorLoop
	}
	if e.ID == "" {
		var b [10]byte
		_, _ = rand.Read(b[:])
		e.ID = "aud_" + e.At.Format("20060102T150405") + "_" + hex.EncodeToString(b[:])
	}
	e.ExpiresAt = expires(e.At)
	if err := g.Control.AppendAudit(ctx, e); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	l := g.Logger
	if l == nil {
		l = slog.Default()
	}
	l.Info("loop.audit", "stage", e.Stage, "action", e.Action, "actor", e.Actor, "itemId", e.ItemID,
		"draftId", e.DraftID, "issue", e.Issue, "pr", e.PR, "reason", e.Reason, "tokens", e.Tokens)
	return nil
}

// Stopped records that stage was refused and returns err.
func (g *Gate) Stopped(ctx context.Context, stage string, err error) error {
	reason := strings.TrimPrefix(err.Error(), ErrStopped.Error()+": ")
	if rerr := g.Record(ctx, Event{Stage: stage, Action: "stopped", Reason: reason}); rerr != nil {
		return errors.Join(err, rerr)
	}
	return err
}
