package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ErrRateLimited is returned by AddReport when the reporter used up the day's quota.
var ErrRateLimited = errors.New("rate limited")

// Report kinds (what was reported, #321).
const (
	ReportListing = "listing" // public_apps/{id} ("探す")
	ReportApp     = "app"     // apps_public/{appId} (a shared app)
	ReportShare   = "share"   // shares/{token}
	ReportRecord  = "record"  // a post in a shared app: TargetID = appId/entityId/recordId
)

// Report is reports/{id}. It carries only what the operator needs to find the
// content: no copy of the content, no free text.
type Report struct {
	ID          string    `json:"id"`
	ReporterUID string    `json:"reporterUid"`
	Kind        string    `json:"kind"`
	TargetID    string    `json:"targetId"`
	TargetUID   string    `json:"targetUid,omitempty"` // owner / author, resolved by the server
	Reason      string    `json:"reason"`
	Status      string    `json:"status"` // "open" until the operator handles it
	CreatedAt   time.Time `json:"createdAt"`
}

// ReportID is deterministic per (reporter, target): reporting the same thing
// twice keeps one document.
func ReportID(reporterUID, kind, targetID string) string {
	sum := sha256.Sum256([]byte(reporterUID + "\x00" + kind + "\x00" + targetID))
	return "rpt_" + hex.EncodeToString(sum[:16])
}

// Block is users/{uid}/blocks/{blockedUid}: uid does not want to see
// blockedUid's posts or listings.
type Block struct {
	UID       string    `json:"uid"` // the blocked user
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// MaxBlocks bounds one user's block list (it is read on every shared listing).
const MaxBlocks = 500

// Moderation is reports, blocks and operator unpublishing (#321, App Review 1.2).
//
//	reports/{id}                    {reporterUid,kind,targetId,targetUid,reason,status,createdAt}
//	report_quota/{uid}              {day,n}  daily cap per reporter
//	users/{uid}/blocks/{blockedUid} {name,createdAt}
//	public_apps/{id}, apps_public/{appId}  + {hidden,hiddenReason,hiddenAt}
type Moderation interface {
	// AddReport stores r (r.ID from ReportID). created is false when the
	// reporter already reported that target. ErrRateLimited when the reporter
	// filed limit new reports on day (a "2006-01-02" key chosen by the caller).
	AddReport(ctx context.Context, r Report, day string, limit int) (created bool, err error)
	// ListReports returns reports, newest first (operator CLI).
	ListReports(ctx context.Context, limit int) ([]Report, error)
	// SetReportStatus marks a report handled ("done" / "dismissed"). ErrNotFound when absent.
	SetReportStatus(ctx context.Context, id, status string) error

	// PutBlock adds (or renames) a block; idempotent.
	PutBlock(ctx context.Context, uid string, b Block) error
	// DeleteBlock removes a block; idempotent.
	DeleteBlock(ctx context.Context, uid, blockedUID string) error
	// ListBlocks returns up to MaxBlocks blocks, oldest first.
	ListBlocks(ctx context.Context, uid string) ([]Block, error)

	// SetCatalogHidden / SetPublicAppHidden are the operator's unpublish
	// switch. ErrNotFound when the document does not exist.
	SetCatalogHidden(ctx context.Context, id string, hidden bool, reason string) error
	SetPublicAppHidden(ctx context.Context, appID string, hidden bool, reason string) error
}

// ModerationState is the operator's unpublish state of a listing / shared app.
type ModerationState struct {
	Hidden       bool      `json:"hidden,omitempty"`
	HiddenReason string    `json:"hiddenReason,omitempty"`
	HiddenAt     time.Time `json:"hiddenAt,omitzero"`
}

// BlockedSet is a uid lookup built from ListBlocks.
func BlockedSet(bs []Block) map[string]bool {
	out := make(map[string]bool, len(bs))
	for _, b := range bs {
		out[b.UID] = true
	}
	return out
}
