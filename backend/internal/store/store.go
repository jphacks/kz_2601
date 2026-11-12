// Package store persists apps, DSL versions and records under
// users/{uid}/apps/{appId}. Two implementations: Firestore (production) and
// Memory (tests / local without emulator).
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Errors returned by Store implementations.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// ConflictError is returned when the caller's base version is stale.
type ConflictError struct{ Current int }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("version conflict: current is %d", e.Current)
}

// App is users/{uid}/apps/{appId}.
type App struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	CurrentVersion int             `json:"currentVersion"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	DSL            json.RawMessage `json:"dsl,omitempty"`
	SortIndex      int             `json:"sortIndex,omitempty"`  // manual list position (1 = first, 0 = never reordered, #277)
	GlanceStat     string          `json:"glanceStat,omitempty"` // stat shown on the app list card: component id, "-" = none, "" = first stat
}

// Source mirrors VersionSource in openapi-appconfig.yaml.
type Source struct {
	Kind         string `json:"kind"`
	Mode         string `json:"mode,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Model        string `json:"model,omitempty"`
	RestoredFrom int    `json:"restoredFrom,omitempty"`
	// PromptVersion: the prompt that generated an ai version (#372, ai.PromptVersion).
	PromptVersion string `json:"promptVersion,omitempty"`
}

// Version is users/{uid}/apps/{appId}/versions/{n} (full snapshot).
type Version struct {
	AppID         string          `json:"appId"`
	Version       int             `json:"version"`
	ParentVersion int             `json:"parentVersion,omitempty"`
	DSL           json.RawMessage `json:"dsl,omitempty"`
	Patch         []dsl.PatchOp   `json:"patch,omitempty"`
	Source        *Source         `json:"source,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// Record is one document of a RecordSet. Author/AuthorName are set only on
// shared/public records (private records are implicitly the uid's own).
type Record struct {
	ID         string         `json:"id"`
	Entity     string         `json:"entity"`
	Data       map[string]any `json:"data"`
	ClientID   string         `json:"clientId,omitempty"`
	Author     string         `json:"author,omitempty"`     // uid of the poster (shared/public)
	AuthorName string         `json:"authorName,omitempty"` // display name chosen by the poster
	MarkCounts map[string]int `json:"markCounts,omitempty"` // mark id -> how many people set it (#255, marks.go)
	// Origin is set on records a server rule created (#341, runner_write.go).
	// Not part of the API.
	Origin *RecordOrigin `json:"-"`
	// UniqueKey is the entity.unique claim the record holds (#273, unique.go).
	// Not part of the API.
	UniqueKey string `json:"-"`
	// Readers are the uids that may read an addressed record (#271: author
	// and recipients, set when it is created). Empty = everyone who may read
	// the entity. Not part of the API.
	Readers []string `json:"-"`
	// OwnerAgeConfirmed marks a profile record of a matching app whose
	// author made the 18+ declaration (#507, users/{uid}.ageConfirmedAt).
	// false = not known from the record: read the author's declaration.
	// Not part of the API.
	OwnerAgeConfirmed bool      `json:"-"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// RecordSet locates one record collection (architecture.md §3):
//
//	private: users/{UID}/apps/{AppID}/entities/{Entity}/records
//	public:  apps_public/{AppID}/entities/{Entity}/records   (UID ignored)
//
// AppID is unique on its own (iOS uses a UUID), so the public collection is
// the same for every user who opens that appId.
type RecordSet struct {
	UID    string
	AppID  string
	Entity string
	Public bool
}

// PrivateRecords is the per-user collection.
func PrivateRecords(uid, appID, entity string) RecordSet {
	return RecordSet{UID: uid, AppID: appID, Entity: entity}
}

// PublicRecords is the app-wide collection shared by all users.
func PublicRecords(appID, entity string) RecordSet {
	return RecordSet{AppID: appID, Entity: entity, Public: true}
}

// PublicApp is apps_public/{appId}: the current DSL of an app that has at
// least one shared/public entity, so that other users can install it.
type PublicApp struct {
	ID        string          `json:"id"`
	OwnerUID  string          `json:"ownerUid"`
	Name      string          `json:"name"`
	Version   int             `json:"version"` // owner's version it was copied from
	DSL       json.RawMessage `json:"dsl"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	// ModerationState: unpublished by the operator (#321). Kept across PublishApp.
	ModerationState
}

// Commit is one new DSL version to append.
type Commit struct {
	BaseVersion int // must equal currentVersion, else ConflictError
	DSL         json.RawMessage
	Name        string // app name to set (dsl.app.name); "" keeps
	Patch       []dsl.PatchOp
	Source      *Source
	// Publish, when set, updates apps_public/{appId} in the same transaction
	// as the commit, so the mirror never lags behind the creator's version.
	Publish *CommitPublish
	// Events are outbox events stored in the same transaction (#342:
	// rules_changed). An event with Version 0 gets the new version. nil
	// stores nothing (the commit costs what it did before).
	Events []OutboxEvent
	// HeldLimit > 0: a commit on a blank app (version HeldVersion) makes it
	// held (#630 app slots), so it fails with *HeldLimitError when uid
	// already holds HeldLimit apps. Counted in the same transaction, which
	// also writes users/{uid}.heldAppsAt so two such commits of one uid run
	// one after the other (the second counts the first). 0 = not checked.
	HeldLimit int
}

// commitEvents gives c's events the new version n and validates them.
func commitEvents(c Commit, n int, now time.Time) ([]OutboxEvent, error) {
	if len(c.Events) == 0 {
		return nil, nil
	}
	events := append([]OutboxEvent(nil), c.Events...)
	for i := range events {
		if events[i].Version == 0 {
			events[i].Version = n
		}
	}
	if err := prepareOutbox(events, now); err != nil {
		return nil, err
	}
	return events, nil
}

// CommitPublish is how a commit touches apps_public/{appId}. Nothing is
// published unless app_owners names the committer as the creator (Claim
// records them first when no creator is known). A mirror the committer owns
// is updated even when no shared entity is left (sharing was withdrawn); a
// missing one is created only when Shared. A mirror owned by another uid is
// left alone, and moderation fields are kept.
type CommitPublish struct {
	Shared bool // the DSL has a shared / public entity
	Claim  bool // record the committer as the creator when none is recorded
}

// publishAction is what a commit does to apps_public/{appId}.
type publishAction int

const (
	publishNone publishAction = iota
	publishCreate
	publishUpdate
)

// decidePublish: owner is the recorded creator after any claim ("" none),
// mirrorOwner the current mirror's owner ("" when there is no mirror).
func decidePublish(p *CommitPublish, uid, owner, mirrorOwner string, mirror bool) publishAction {
	switch {
	case p == nil || owner != uid:
		return publishNone
	case mirror && mirrorOwner == uid:
		return publishUpdate
	case !mirror && p.Shared:
		return publishCreate
	}
	return publishNone
}

// Store is what the handlers depend on. All methods are scoped by uid.
type Store interface {
	ListApps(ctx context.Context, uid string, limit, offset int) ([]App, error)
	GetApp(ctx context.Context, uid, appID string) (*App, error)
	// EnsureApp returns the app, creating it at version 1 with initial DSL if absent.
	EnsureApp(ctx context.Context, uid, appID string, initial json.RawMessage, name string) (app *App, created bool, err error)
	UpdateAppMeta(ctx context.Context, uid, appID string, name, status *string) (*App, error)
	// ReorderApps sets SortIndex = position+1 for each listed app the caller
	// has (unknown ids are skipped, unlisted apps keep theirs). It does not
	// touch updatedAt, which stays "last changed".
	ReorderApps(ctx context.Context, uid string, ids []string) error
	// SetAppGlance sets GlanceStat ("" removes it). Like ReorderApps it does
	// not touch updatedAt.
	SetAppGlance(ctx context.Context, uid, appID, glance string) (*App, error)
	CommitVersion(ctx context.Context, uid, appID string, c Commit) (*Version, error)
	ListVersions(ctx context.Context, uid, appID string, limit, offset int) ([]Version, error)
	GetVersion(ctx context.Context, uid, appID string, n int) (*Version, error)

	// PublishApp writes apps_public/{appId} (create, or update when the
	// existing document is owned by uid). It returns false, nil when another
	// uid owns that appId: the caller's private copy is unaffected.
	PublishApp(ctx context.Context, uid, appID string, dsl json.RawMessage, name string, version int) (published bool, err error)
	GetPublicApp(ctx context.Context, appID string) (*PublicApp, error)

	// ListRecords returns up to limit records, newest first (sort/filter happen in the handler).
	// Private sets return ErrNotFound when the app does not exist; public sets are never "missing".
	ListRecords(ctx context.Context, rs RecordSet, limit int) ([]Record, error)
	GetRecord(ctx context.Context, rs RecordSet, id string) (*Record, error)
	// CreateRecord is idempotent on rec.ID (derived from clientId by the caller).
	CreateRecord(ctx context.Context, rs RecordSet, rec Record) (out *Record, created bool, err error)
	UpdateRecord(ctx context.Context, rs RecordSet, id string, data map[string]any) (*Record, error)
	DeleteRecord(ctx context.Context, rs RecordSet, id string) error
	DeleteAllRecords(ctx context.Context, rs RecordSet) error

	Sharing         // share links + public catalog (share.go)
	ReleaseRequests // "ship this as its own app" requests (release.go)
	Accounts        // account deletion (account.go)
	Ownership       // appId creators + catalog installs (owner.go)
	Membership      // shared-app participants (member.go)
	Purger          // physical app deletion (purge.go)
	AccountPurger   // physical erasure of deleted accounts (account_purge.go)
	Marks           // per-user like / save toggles on shared records (marks.go)
	Follows         // whom people follow in shared apps (follow.go, #270)
	Feedbacks       // in-app feedback (feedback.go)
	Moderation      // reports, blocks, operator unpublish (moderation.go, #321)
	Matching        // interests and matches of matching apps (matching.go, #507)
	Devices         // push tokens of users' devices (device.go, #269)
	NotifyConsents  // recipients' consent to others' notifications (device.go, #339)
	Outbox          // record writes that queue runner triggers (outbox.go, #341)
	Schedules       // runner schedules and per-app state (schedule.go, #342)
	RunnerActivity  // read-only rule statuses and stops for the creator (rule_status.go, #347)
	UniqueRecords   // writes that keep entity.unique (unique.go, #273)
}

// MaxRecordsPerEntity bounds in-memory sort (architecture.md §3).
const MaxRecordsPerEntity = 2000

// Now is the current UTC time at the microsecond precision Firestore keeps,
// so a value returned right after a write equals what a later read returns.
func Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// NewID returns a time-ordered random id with prefix (ULID-like, Crockford base32).
func NewID(prefix string) string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0], b[1], b[2], b[3], b[4], b[5] = byte(ms>>40), byte(ms>>32), byte(ms>>24), byte(ms>>16), byte(ms>>8), byte(ms)
	_, _ = rand.Read(b[6:])
	return prefix + strings.ToUpper(crockford.EncodeToString(b[:]))[:26]
}

// IDFromClientID derives a stable record id from an idempotency key.
func IDFromClientID(clientID string) string {
	sum := sha256.Sum256([]byte(clientID))
	return "rec_" + strings.ToUpper(crockford.EncodeToString(sum[:16]))[:26]
}

var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// VersionDocID formats n as the 8-digit document id.
func VersionDocID(n int) string { return fmt.Sprintf("%08d", n) }
