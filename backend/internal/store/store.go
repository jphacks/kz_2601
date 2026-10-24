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

// Record is users/{uid}/apps/{appId}/entities/{entity}/records/{id}.
type Record struct {
	ID        string         `json:"id"`
	Entity    string         `json:"entity"`
	Data      map[string]any `json:"data"`
	ClientID  string         `json:"clientId,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
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

	// ListRecords returns up to limit records, newest first (sort/filter happen in the handler).
	ListRecords(ctx context.Context, uid, appID, entity string, limit int) ([]Record, error)
	GetRecord(ctx context.Context, uid, appID, entity, id string) (*Record, error)
	// CreateRecord is idempotent on rec.ID (derived from clientId by the caller).
	CreateRecord(ctx context.Context, uid, appID, entity string, rec Record) (out *Record, created bool, err error)
	UpdateRecord(ctx context.Context, uid, appID, entity, id string, data map[string]any) (*Record, error)
	DeleteRecord(ctx context.Context, uid, appID, entity, id string) error
	DeleteAllRecords(ctx context.Context, uid, appID, entity string) error
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
