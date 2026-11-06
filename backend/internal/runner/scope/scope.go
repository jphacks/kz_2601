// Package scope is the runner's appId-bound handle (#337 layers 1-3). A Handle
// is made from one trigger and touches that app only: no method takes an
// appId, every record path is built in one place (recordSet) and checked
// against the handle's appId, and records can be read only when they are the
// trigger record or a record its ref fields point to.
//
// It implements engine.RefReader and notify.Directory / notify.Accounts /
// notify.Consent, and reads device tokens (#269).
package scope

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// Errors.
var (
	// ErrViolation: a path outside the handle's app was built (must never
	// happen; logged as runner.scope_violation).
	ErrViolation = errors.New("scope: path outside the app")
	// ErrNotReadable: the record is neither the trigger record nor one of its ref targets.
	ErrNotReadable = errors.New("scope: record is not readable by this run")
	// ErrNotBound: Bind has not been called.
	ErrNotBound = errors.New("scope: handle is not bound to a trigger")
)

// ViolationEvent is the log message of a scope violation.
const ViolationEvent = "runner.scope_violation"

// Backend is the part of the store a handle uses.
type Backend interface {
	GetApp(ctx context.Context, uid, appID string) (*store.App, error)
	GetVersion(ctx context.Context, uid, appID string, n int) (*store.Version, error)
	GetPublicApp(ctx context.Context, appID string) (*store.PublicApp, error)
	AppOwner(ctx context.Context, appID string) (string, error)
	ListMembers(ctx context.Context, appID string, limit int) ([]store.Member, error)
	GetRecord(ctx context.Context, rs store.RecordSet, id string) (*store.Record, error)
	CreateRecord(ctx context.Context, rs store.RecordSet, rec store.Record) (*store.Record, bool, error)
	UserDeleted(ctx context.Context, uid string) (bool, error)
	ListBlocks(ctx context.Context, uid string) ([]store.Block, error)
	ListDevices(ctx context.Context, uid string) ([]store.Device, error)
	DeleteDeviceToken(ctx context.Context, uid, token string) (int, error)
	NotifyAllowed(ctx context.Context, uid, appID string) (bool, error)
	store.RunnerWrites
}

// Handle is bound to one app (and, for private data, one user).
type Handle struct {
	b      Backend
	env    string
	appID  string
	uid    string // private: whose records; "" for shared / public
	shared bool
	log    *slog.Logger

	owner     string
	ownerRead bool

	// set by Bind
	app      *dsl.App
	readable map[string]bool // entity + "\x00" + id
}

var (
	_ engine.RefReader = (*Handle)(nil)
	_ notify.Directory = (*Handle)(nil)
	_ notify.Accounts  = (*Handle)(nil)
	_ notify.Consent   = (*Handle)(nil)
)

// New binds a handle to the app of ev (an outbox event of env).
func New(b Backend, env string, ev store.OutboxEvent, logger *slog.Logger) (*Handle, error) {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handle{b: b, env: env, appID: ev.AppID, shared: ev.Scope != string(dsl.ScopePrivate) && ev.Scope != "", log: logger}
	if !h.shared {
		h.uid = ev.UID
	}
	if h.appID == "" || (!h.shared && h.uid == "") {
		return nil, fmt.Errorf("scope: event without appId or private uid")
	}
	return h, nil
}

// AppID is the app this handle is bound to.
func (h *Handle) AppID() string { return h.appID }

// Shared reports whether the handle works on shared / public data.
func (h *Handle) Shared() bool { return h.shared }

// Bind sets the DSL of the run and what it may read: the trigger record
// (entity/id) and the records its ref fields (in data) point to.
func (h *Handle) Bind(app *dsl.App, entity, id string, data map[string]any) {
	h.app = app
	h.readable = map[string]bool{entity + "\x00" + id: true}
	ent := app.EntityByID(entity)
	if ent == nil {
		return
	}
	for _, f := range ent.Fields {
		if f.Type != dsl.FieldRef {
			continue
		}
		if ref, _ := data[f.ID].(string); ref != "" {
			h.readable[f.RefEntity+"\x00"+ref] = true
		}
	}
}

// recordSet is the only place a record path is built.
func (h *Handle) recordSet(ctx context.Context, entity string) (store.RecordSet, error) {
	if h.app == nil {
		return store.RecordSet{}, ErrNotBound
	}
	ent := h.app.EntityByID(entity)
	if ent == nil {
		return store.RecordSet{}, fmt.Errorf("scope: entity %q is not in the app", entity)
	}
	var rs store.RecordSet
	if ent.IsShared() {
		rs = store.PublicRecords(h.appID, entity)
	} else {
		rs = store.PrivateRecords(h.uid, h.appID, entity)
	}
	return rs, h.check(ctx, rs)
}

// check rejects a record set outside the handle's app (and side).
func (h *Handle) check(ctx context.Context, rs store.RecordSet) error {
	ok := rs.AppID == h.appID && rs.Public == h.shared && (rs.Public || (rs.UID != "" && rs.UID == h.uid))
	if ok {
		return nil
	}
	h.log.ErrorContext(ctx, ViolationEvent,
		slog.String("event", ViolationEvent),
		slog.String("app_id", h.appID),
		slog.String("path_app_id", rs.AppID),
		slog.Bool("public", rs.Public),
		slog.String("path_uid_hash", httpx.UIDHash(rs.UID)))
	return ErrViolation
}

func (h *Handle) readableSet(ctx context.Context, entity, id string) (store.RecordSet, error) {
	if h.readable == nil {
		return store.RecordSet{}, ErrNotBound
	}
	if !h.readable[entity+"\x00"+id] {
		return store.RecordSet{}, ErrNotReadable
	}
	return h.recordSet(ctx, entity)
}

// ---- app state (the run's pre-checks) ----------------------------------------

// PublicApp is apps_public/{appId} (shared / public apps).
func (h *Handle) PublicApp(ctx context.Context) (*store.PublicApp, error) {
	return h.b.GetPublicApp(ctx, h.appID)
}

// PrivateApp is users/{uid}/apps/{appId} (private apps).
func (h *Handle) PrivateApp(ctx context.Context) (*store.App, error) {
	if h.shared {
		return nil, ErrViolation
	}
	return h.b.GetApp(ctx, h.uid, h.appID)
}

// Version is version n of the DSL the rules come from: the creator's
// versions for shared / public data, the user's own for private.
func (h *Handle) Version(ctx context.Context, n int) (*store.Version, error) {
	who := h.uid
	if h.shared {
		o, err := h.Owner(ctx)
		if err != nil {
			return nil, err
		}
		if o == "" {
			return nil, store.ErrNotFound
		}
		who = o
	}
	v, err := h.b.GetVersion(ctx, who, h.appID, n)
	if err == nil && v.AppID != "" && v.AppID != h.appID {
		return nil, h.check(ctx, store.RecordSet{AppID: v.AppID, Public: h.shared, UID: h.uid})
	}
	return v, err
}

// ---- engine.RefReader ---------------------------------------------------------

// Record implements engine.RefReader.
func (h *Handle) Record(ctx context.Context, entity, id string) (expreval.Record, bool, error) {
	rs, err := h.readableSet(ctx, entity, id)
	if err != nil {
		return expreval.Record{}, false, err
	}
	r, err := h.b.GetRecord(ctx, rs, id)
	if errors.Is(err, store.ErrNotFound) {
		return expreval.Record{}, false, nil
	}
	if err != nil {
		return expreval.Record{}, false, err
	}
	return ToExpr(r), true, nil
}

// ToExpr converts a stored record for the evaluator.
func ToExpr(r *store.Record) expreval.Record {
	out := expreval.Record{ID: r.ID, Data: r.Data, Author: r.Author, AuthorName: r.AuthorName}
	if !r.CreatedAt.IsZero() {
		out.CreatedAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ---- notify.Directory -----------------------------------------------------------

// Owner implements notify.Directory: the creator (app_owners) of a shared
// app, the user of a private one.
func (h *Handle) Owner(ctx context.Context) (string, error) {
	if !h.shared {
		return h.uid, nil
	}
	if h.ownerRead {
		return h.owner, nil
	}
	o, err := h.b.AppOwner(ctx, h.appID)
	if errors.Is(err, store.ErrNotFound) {
		o, err = "", nil
	}
	if err != nil {
		return "", err
	}
	h.owner, h.ownerRead = o, true
	return o, nil
}

// Members implements notify.Directory (none for private data).
func (h *Handle) Members(ctx context.Context) ([]string, error) {
	if !h.shared {
		return nil, nil
	}
	ms, err := h.b.ListMembers(ctx, h.appID, store.MaxMembers)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.UID)
	}
	return out, nil
}

// RecordAuthor implements notify.Directory (readable records only).
func (h *Handle) RecordAuthor(ctx context.Context, entity, id string) (string, bool, error) {
	r, found, err := h.Record(ctx, entity, id)
	if err != nil || !found {
		return "", found, err
	}
	return r.Author, true, nil
}

// ---- notify.Accounts --------------------------------------------------------------

// Deleted implements notify.Accounts.
func (h *Handle) Deleted(ctx context.Context, uid string) (bool, error) {
	return h.b.UserDeleted(ctx, uid)
}

// Blocks implements notify.Accounts.
func (h *Handle) Blocks(ctx context.Context, uid string) ([]string, error) {
	bs, err := h.b.ListBlocks(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.UID)
	}
	return out, nil
}

// ---- notify.Consent and devices (#269, #339) ---------------------------------------

// Allows implements notify.Consent: uid's stored consent for this app.
func (h *Handle) Allows(ctx context.Context, uid string) (bool, error) {
	return h.b.NotifyAllowed(ctx, uid, h.appID)
}

// DeviceTokens are uid's tokens valid on the APNs environment env (all when
// env is ""). The notifier asks only for recipients that passed the checks
// (#337: tokens are personal data across apps).
func (h *Handle) DeviceTokens(ctx context.Context, uid, env string) ([]string, error) {
	ds, err := h.b.ListDevices(ctx, uid)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range ds {
		if env == "" || d.Environment == env {
			out = append(out, d.Token)
		}
	}
	return out, nil
}

// DropDeviceToken deletes uid's devices holding token (APNs 410).
func (h *Handle) DropDeviceToken(ctx context.Context, uid, token string) error {
	_, err := h.b.DeleteDeviceToken(ctx, uid, token)
	return err
}

// ---- writes -----------------------------------------------------------------------

// CreateRecord creates entity/id (idempotent on id) with origin. It goes
// through no outbox, so it triggers no rule.
func (h *Handle) CreateRecord(ctx context.Context, entity, id string, data map[string]any, origin store.RecordOrigin) (created bool, err error) {
	rs, err := h.recordSet(ctx, entity)
	if err != nil {
		return false, err
	}
	_, created, err = h.b.CreateRecord(ctx, rs, store.Record{ID: id, Data: data, Origin: &origin})
	return created, err
}

// ApplyOnce writes fn(data) to a readable record once per key (see
// store.RunnerWrites.ApplyRecordOnce). The record is the trigger record or
// one of its ref targets.
func (h *Handle) ApplyOnce(ctx context.Context, entity, id, key string, fn func(map[string]any) map[string]any) (bool, error) {
	rs, err := h.readableSet(ctx, entity, id)
	if err != nil {
		return false, err
	}
	return h.b.ApplyRecordOnce(ctx, rs, id, h.env, key, fn)
}

// Marked reports whether the action key already ran.
func (h *Handle) Marked(ctx context.Context, key string) (bool, error) {
	return h.b.RunnerMarked(ctx, h.env, key)
}

// Mark records that the action key ran.
func (h *Handle) Mark(ctx context.Context, key string) error {
	return h.b.MarkRunnerDone(ctx, h.env, key)
}
