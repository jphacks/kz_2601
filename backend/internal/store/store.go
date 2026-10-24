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
}

// Source mirrors VersionSource in openapi-appconfig.yaml.
type Source struct {
	Kind         string `json:"kind"`
	Mode         string `json:"mode,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Model        string `json:"model,omitempty"`
	RestoredFrom int    `json:"restoredFrom,omitempty"`
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
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
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
}

// Commit is one new DSL version to append.
type Commit struct {
	BaseVersion int // must equal currentVersion, else ConflictError
	DSL         json.RawMessage
	Name        string // app name to set (dsl.app.name); "" keeps
	Patch       []dsl.PatchOp
	Source      *Source
}

// Store is what the handlers depend on. All methods are scoped by uid.
type Store interface {
	ListApps(ctx context.Context, uid string, limit, offset int) ([]App, error)
	GetApp(ctx context.Context, uid, appID string) (*App, error)
	// EnsureApp returns the app, creating it at version 1 with initial DSL if absent.
	EnsureApp(ctx context.Context, uid, appID string, initial json.RawMessage, name string) (app *App, created bool, err error)
	UpdateAppMeta(ctx context.Context, uid, appID string, name, status *string) (*App, error)
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
}

// MaxRecordsPerEntity bounds in-memory sort (architecture.md §3).
const MaxRecordsPerEntity = 2000

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
