// Package loop is the self-improvement loop (#364): feedback from every source
// is masked and stored in one inbox (#368), triaged by an LLM that only
// returns validated JSON (#376), turned into issue drafts that wait for a
// human (#369), and the senders are told when the fix lands (#374). Every
// step writes an audit event and can be stopped without a deploy (#375).
//
// Decisions: docs/decisions/365-self-improvement-loop.md, 366-feedback-
// prompt-injection.md, 367-feedback-pii.md. Operations:
// docs/backend/features/feedback-loop.md.
//
// Firestore layout (all written only by backend/cmd/feedback-loop; clients
// never read them):
//
//	loop_inbox/{itemId}     Item     masked copy of one piece of feedback
//	loop_drafts/{draftId}   Draft    issue draft waiting for a human
//	loop_notices/{noticeId} Notice   「あなたの声で直りました」 for one sender
//	loop_audit/{eventId}    Event    one step of the loop (ids and codes only)
//	loop_control/switches   per-stage stop flags
//	loop_control/usage-YYYY-MM  LLM tokens used by triage that month
//	loop_control/cursor-{source} ingest high-water mark
//
// Everything carries expiresAt = createdAt + Retention (#367: 90 days).
package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// Retention is how long every loop document is kept (#367 decision 8). It is
// shorter than the source feedback's 180 days, so no copy outlives its source.
const Retention = 90 * 24 * time.Hour

// Source is where a piece of feedback came from.
type Source string

// Sources (#368).
const (
	SourceInApp       Source = "in_app"           // feedback/{id} (#319)
	SourceStoreReview Source = "app_store_review" // App Store Connect customer reviews
	SourceCrash       Source = "crash"            // a crash spike, recorded as one item (#296)
	SourceTestFlight  Source = "testflight"       // TestFlight beta feedback (screenshot and crash submissions)
)

// Item statuses.
const (
	// ItemNew: ingested, waiting for triage.
	ItemNew = "new"
	// ItemHeld: kept for the weekly manual triage only (consent version not
	// allowed for AI, too much masked, triage output rejected, PII suspected).
	ItemHeld = "held"
	// ItemTriaged: classified and clustered, waiting for a draft.
	ItemTriaged = "triaged"
	// ItemSpam: classified as spam/abuse; never drafted (#366).
	ItemSpam = "spam"
	// ItemDrafted: part of a draft.
	ItemDrafted = "drafted"
)

// Hold reasons (Item.HoldReason). Codes only: never the text.
const (
	HoldConsent   = "consent_version"
	HoldSource    = "source_not_allowed"
	HoldMasked    = "mask_ratio"
	HoldEmpty     = "empty"
	HoldRejected  = "triage_output_rejected"
	HoldPII       = "pii_suspected"
	HoldPraise    = "praise"
	HoldNoCluster = "no_cluster"
)

// Item is loop_inbox/{id}: one piece of feedback, masked at ingest.
type Item struct {
	ID       string `firestore:"-"`
	Source   Source `firestore:"source"`
	SourceID string `firestore:"sourceId"`
	// UID is the in-app sender (to notify them, #374, and to follow account
	// deletion). It is never given to the LLM.
	UID string `firestore:"uid,omitempty"`
	// Text is the masked text (pii.Mask). The unmasked text is never stored here.
	Text       string         `firestore:"text"`
	MaskCounts map[string]int `firestore:"maskCounts,omitempty"`
	// AIAllowed: may be sent to the triage LLM (consent version allowed,
	// source allowed, masking within bounds).
	AIAllowed     bool   `firestore:"aiAllowed"`
	HoldReason    string `firestore:"holdReason,omitempty"`
	Kind          string `firestore:"kind,omitempty"` // the source's own kind (bug/idea/other, crash type)
	Rating        int    `firestore:"rating,omitempty"`
	Count         int    `firestore:"count,omitempty"` // crash spike size
	HasScreenshot bool   `firestore:"hasScreenshot,omitempty"`
	HasDSL        bool   `firestore:"hasDsl,omitempty"`
	AppVersion    string `firestore:"appVersion,omitempty"`
	OSMajor       string `firestore:"osMajor,omitempty"`
	Status        string `firestore:"status"`

	// Triage (#376).
	Category   string    `firestore:"category,omitempty"`
	Area       string    `firestore:"area,omitempty"`
	Priority   string    `firestore:"priority,omitempty"`
	Summary    string    `firestore:"summary,omitempty"` // LLM-written, checked (検査器 1)
	ClusterKey string    `firestore:"clusterKey,omitempty"`
	DraftID    string    `firestore:"draftId,omitempty"`
	TriagedAt  time.Time `firestore:"triagedAt,omitempty"`

	CreatedAt  time.Time `firestore:"createdAt"` // the source's
	IngestedAt time.Time `firestore:"ingestedAt"`
	ExpiresAt  time.Time `firestore:"expiresAt"`
}

// ItemID is the deterministic inbox id of a source document, so ingesting
// the same feedback twice does not duplicate it. The source id is hashed: a
// feedback id is not shown anywhere public (#367).
func ItemID(src Source, sourceID string) string {
	sum := sha256.Sum256([]byte(string(src) + "\x00" + sourceID))
	return "lin_" + hex.EncodeToString(sum[:12])
}

// Draft statuses.
const (
	DraftPending   = "pending"   // waiting for a human
	DraftApproved  = "approved"  // approved; not posted yet (dry run or a failed post)
	DraftPublished = "published" // posted as a GitHub issue
	DraftRejected  = "rejected"
)

// Draft is loop_drafts/{id}: an issue draft for one cluster (#369). Title and
// Body are what would be posted; they hold summaries only, never feedback
// text, and pass the exit check before they are stored and again before
// they are posted.
type Draft struct {
	ID          string   `firestore:"-"`
	ClusterKey  string   `firestore:"clusterKey"`
	Title       string   `firestore:"title"`
	Body        string   `firestore:"body"`
	Category    string   `firestore:"category"`
	Area        string   `firestore:"area"`
	Priority    string   `firestore:"priority"`
	ItemIDs     []string `firestore:"itemIds"`
	Count       int      `firestore:"count"`
	Sources     []string `firestore:"sources"`
	AppVersions []string `firestore:"appVersions,omitempty"`
	Status      string   `firestore:"status"`

	ReviewedBy   string    `firestore:"reviewedBy,omitempty"`
	ReviewedAt   time.Time `firestore:"reviewedAt,omitempty"`
	RejectReason string    `firestore:"rejectReason,omitempty"`
	IssueRepo    string    `firestore:"issueRepo,omitempty"`
	IssueNumber  int       `firestore:"issueNumber,omitempty"`
	BodyHash     string    `firestore:"bodyHash,omitempty"` // sha256 of the posted title+body (#366 decision 11)
	// AppendRepo and AppendIssue: the cluster already has a published,
	// not-fixed issue there, so this draft is a comment that adds the new
	// counts and examples to it instead of a new issue (#369). Set by the
	// builder; the approved body is posted as that comment.
	AppendRepo  string `firestore:"appendRepo,omitempty"`
	AppendIssue int    `firestore:"appendIssue,omitempty"`

	// PublishedAt: when the issue or comment was posted. ClosedAt: the
	// issue was found closed (any reason) when an append draft was about to
	// be posted to it, so the loop stops adding to it.
	PublishedAt time.Time `firestore:"publishedAt,omitempty"`
	ClosedAt    time.Time `firestore:"closedAt,omitempty"`

	FixedPR    int       `firestore:"fixedPr,omitempty"`
	FixedAt    time.Time `firestore:"fixedAt,omitempty"`
	NotifiedAt time.Time `firestore:"notifiedAt,omitempty"`

	CreatedAt time.Time `firestore:"createdAt"`
	UpdatedAt time.Time `firestore:"updatedAt"`
	ExpiresAt time.Time `firestore:"expiresAt"`
}

// Notice is loop_notices/{id}: 「あなたの声で直りました」 for one sender of
// one fixed draft (#374). The text is a fixed template plus the approved
// public title; no LLM writes it.
type Notice struct {
	ID        string    `firestore:"-"`
	UID       string    `firestore:"uid"`
	DraftID   string    `firestore:"draftId"`
	Title     string    `firestore:"title"`
	Body      string    `firestore:"body"`
	Pushed    bool      `firestore:"pushed"`
	ReadAt    time.Time `firestore:"readAt,omitempty"`
	CreatedAt time.Time `firestore:"createdAt"`
	ExpiresAt time.Time `firestore:"expiresAt"`
}

// FeedbackFixedConsentID is the users/{uid}/notifyConsent/{id} document
// that records a sender's agreement to a push when their feedback is fixed.
// The app sets it (PUT /v1/me/notify-consent/feedback-fixed); the in-app
// notice does not need it.
const FeedbackFixedConsentID = "_feedback_fixed"

// NoticeID is deterministic per (draft, uid): one sender gets one notice per fix.
func NoticeID(draftID, uid string) string {
	sum := sha256.Sum256([]byte(draftID + "\x00" + uid))
	return "ntc_" + hex.EncodeToString(sum[:12])
}

// NoticeReader is what the app API (GET /v1/me/notices, #374) needs: the
// caller's notices and marking one read. Memory and Firestore implement it.
// MarkNoticeRead returns ErrNotFound for a missing notice and for another
// user's, so a caller cannot tell them apart; marking twice keeps the first
// readAt.
type NoticeReader interface {
	ListNotices(ctx context.Context, uid string, limit int) ([]Notice, error)
	MarkNoticeRead(ctx context.Context, uid, id string, at time.Time) (*Notice, error)
}

// ItemQuery filters ListItems.
type ItemQuery struct {
	Status string // "" = any
	Limit  int    // <= 0 = 100
}

// DraftQuery filters ListDrafts.
type DraftQuery struct {
	Status string // "" = any
	Limit  int    // <= 0 = 100
}

// ErrNotFound is returned for a missing document.
var ErrNotFound = errors.New("loop: not found")

// Store keeps the loop's documents.
type Store interface {
	// PutItem creates it unless it.ID already exists (created=false then;
	// the existing item is left as it is).
	PutItem(ctx context.Context, it Item) (created bool, err error)
	GetItem(ctx context.Context, id string) (*Item, error)
	UpdateItem(ctx context.Context, it Item) error
	// ListItems returns items oldest-ingested first.
	ListItems(ctx context.Context, q ItemQuery) ([]Item, error)

	PutDraft(ctx context.Context, d Draft) error
	GetDraft(ctx context.Context, id string) (*Draft, error)
	// ListDrafts returns drafts newest first.
	ListDrafts(ctx context.Context, q DraftQuery) ([]Draft, error)

	// PutNotice creates n unless n.ID already exists.
	PutNotice(ctx context.Context, n Notice) (created bool, err error)
	// LastPush is when uid was last sent a push for a notice (zero if never).
	LastPush(ctx context.Context, uid string) (time.Time, error)
	// ListNotices returns uid's notices newest first.
	ListNotices(ctx context.Context, uid string, limit int) ([]Notice, error)

	// Cursor is the ingest high-water mark of src (zero if none).
	Cursor(ctx context.Context, src Source) (time.Time, error)
	SetCursor(ctx context.Context, src Source, t time.Time) error

	// PurgeExpired deletes up to limit documents per collection whose
	// expiresAt is before now. dryRun only counts.
	PurgeExpired(ctx context.Context, now time.Time, limit int, dryRun bool) (int, error)
	// DeleteItems deletes the given inbox items.
	DeleteItems(ctx context.Context, ids []string) error
	// ItemsByUID returns the ids of inbox items and notices of uid (account deletion).
	ItemsByUID(ctx context.Context, uid string) (items, notices []string, err error)
	// DeleteNotices deletes the given notices.
	DeleteNotices(ctx context.Context, ids []string) error
}

// expires returns t + Retention.
func expires(t time.Time) time.Time { return t.Add(Retention) }

// ExpiresFrom is the expiry of a document created at t, never later than
// the source's own expiry (source created at srcCreated with srcRetention;
// zero srcRetention = no bound).
func ExpiresFrom(t, srcCreated time.Time, srcRetention time.Duration) time.Time {
	e := expires(t)
	if srcRetention > 0 && !srcCreated.IsZero() {
		if s := srcCreated.Add(srcRetention); s.Before(e) {
			return s
		}
	}
	return e
}
