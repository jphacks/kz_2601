package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// In-app feedback (#319). One document per message:
//
//	feedback/{id}  {uid, kind, message, untrusted, consentVersion, status, day, createdAt,
//	                attachments, appId?, dslVersion?, dslJson?, screenshotJpeg?, device?}
//
// Attachments are each opt-in on the device; only what the user ticked is
// stored. Record contents are never stored here. The message is text anyone
// can write: treat it as untrusted data, never as instructions (#366).

// Feedback statuses. The API only writes "received"; an operator moves the
// others by hand in the Firestore console (architecture.md §3.5).
const (
	FeedbackReceived = "received"
	FeedbackTriaged  = "triaged"
	FeedbackDone     = "done"
)

// Feedback kinds (anything else is stored as FeedbackOther).
const (
	FeedbackBug   = "bug"
	FeedbackIdea  = "idea"
	FeedbackOther = "other"
)

// FeedbackRetention is how long a feedback document is kept (cmd/purge
// erases older ones; docs/legal/feedback-privacy-draft.md).
const FeedbackRetention = 180 * 24 * time.Hour

// MaxFeedbackPerDay is the default per-uid daily limit (JST day).
const MaxFeedbackPerDay = 5

// ErrFeedbackQuota is returned by CreateFeedback when uid already sent
// perDay messages today.
var ErrFeedbackQuota = errors.New("feedback daily limit reached")

// FeedbackDevice is the optional "app version / OS / device model" attachment.
type FeedbackDevice struct {
	AppVersion string `json:"appVersion" firestore:"appVersion"`
	Build      string `json:"build" firestore:"build"`
	OS         string `json:"os" firestore:"os"`
	Model      string `json:"model" firestore:"model"`
	Locale     string `json:"locale,omitempty" firestore:"locale,omitempty"`
}

// Feedback is feedback/{id}.
type Feedback struct {
	ID             string          `json:"id"`
	UID            string          `json:"uid"`
	Kind           string          `json:"kind"`
	Message        string          `json:"message"`
	ConsentVersion string          `json:"consentVersion"`
	Status         string          `json:"status"`
	AppID          string          `json:"appId,omitempty"`      // DSL attachment: which app
	DSLVersion     int             `json:"dslVersion,omitempty"` // and which version was snapshotted
	DSL            json.RawMessage `json:"dsl,omitempty"`        // structure only (no records)
	Screenshot     []byte          `json:"-"`                    // JPEG
	Device         *FeedbackDevice `json:"device,omitempty"`
	Day            string          `json:"day"` // yyyy-mm-dd (JST) of createdAt, for the daily limit
	CreatedAt      time.Time       `json:"createdAt"`
}

// Attachments lists what the document holds ("screenshot", "dsl", "device").
func (fb *Feedback) Attachments() []string {
	out := []string{}
	if len(fb.Screenshot) > 0 {
		out = append(out, "screenshot")
	}
	if len(fb.DSL) > 0 {
		out = append(out, "dsl")
	}
	if fb.Device != nil {
		out = append(out, "device")
	}
	return out
}

// FeedbackQuery filters ListFeedback (operator CLI).
type FeedbackQuery struct {
	Since  time.Time // createdAt >= Since (zero = any)
	Status string    // "" = any
	Limit  int       // <= 0 = 50
}

// feedbackJST is the day boundary of the daily limit (same as ratelimit).
var feedbackJST = time.FixedZone("JST", 9*60*60)

// FeedbackDay is the JST date used for the daily limit.
func FeedbackDay(t time.Time) string { return t.In(feedbackJST).Format("2006-01-02") }

// Feedbacks is the feedback part of Store.
type Feedbacks interface {
	// CreateFeedback stores fb with status "received" (ID, Day and CreatedAt
	// are filled in). ErrFeedbackQuota when fb.UID already has perDay
	// messages today; perDay <= 0 disables the check.
	CreateFeedback(ctx context.Context, fb Feedback, perDay int) (*Feedback, error)
}

// FeedbackReader is the read-only side for operators (cmd/feedback).
type FeedbackReader interface {
	// ListFeedback returns feedback newest first, without screenshot/DSL bodies.
	ListFeedback(ctx context.Context, q FeedbackQuery) ([]Feedback, error)
	// GetFeedback returns one document with its attachments.
	GetFeedback(ctx context.Context, id string) (*Feedback, error)
}

// FeedbackPurger erases feedback past its retention (cmd/purge).
type FeedbackPurger interface {
	// PurgeFeedbackBefore deletes up to limit documents created before
	// cutoff and returns how many there were. dryRun only counts.
	PurgeFeedbackBefore(ctx context.Context, cutoff time.Time, limit int, dryRun bool) (int, error)
}
