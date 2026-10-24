package store

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

type memApp struct {
	app      App
	versions map[int]*Version
	records  map[string]map[string]*Record // entity -> id -> record
}

// Memory is an in-process Store for tests and local development.
type Memory struct {
	mu    sync.Mutex
	users map[string]map[string]*memApp
	now   func() time.Time
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{users: map[string]map[string]*memApp{}, now: time.Now}
}

var _ Store = (*Memory)(nil)

func (m *Memory) get(uid, appID string) *memApp {
	if apps, ok := m.users[uid]; ok {
		return apps[appID]
	}
	return nil
}

func cloneRaw(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	return append(json.RawMessage(nil), b...)
}

func cloneData(d map[string]any) map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}

func (a *memApp) snapshot() *App {
	cp := a.app
	cp.DSL = cloneRaw(a.app.DSL)
	return &cp
}

func cloneVersion(v *Version) *Version {
	cp := *v
	cp.DSL = cloneRaw(v.DSL)
	return &cp
}

func cloneRecord(r *Record) *Record {
	cp := *r
	cp.Data = cloneData(r.Data)
	return &cp
}

// ListApps implements Store.
func (m *Memory) ListApps(_ context.Context, uid string, limit, offset int) ([]App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []App
	for _, a := range m.users[uid] {
		out = append(out, *a.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return page(out, limit, offset), nil
}

func page[T any](in []T, limit, offset int) []T {
	if offset >= len(in) {
		return []T{}
	}
	in = in[offset:]
	if limit > 0 && len(in) > limit {
		in = in[:limit]
	}
	return in
}

// GetApp implements Store.
func (m *Memory) GetApp(_ context.Context, uid, appID string) (*App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	return a.snapshot(), nil
}

// EnsureApp implements Store.
func (m *Memory) EnsureApp(_ context.Context, uid, appID string, initial json.RawMessage, name string) (*App, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.get(uid, appID); a != nil {
		return a.snapshot(), false, nil
	}
	now := m.now().UTC()
	a := &memApp{
		app: App{ID: appID, Name: name, CurrentVersion: 1, Status: "active", CreatedAt: now, UpdatedAt: now, DSL: cloneRaw(initial)},
		versions: map[int]*Version{1: {
			AppID: appID, Version: 1, DSL: cloneRaw(initial), Source: &Source{Kind: "system", Mode: "full"}, CreatedAt: now,
		}},
		records: map[string]map[string]*Record{},
	}
	if m.users[uid] == nil {
		m.users[uid] = map[string]*memApp{}
	}
	m.users[uid][appID] = a
	return a.snapshot(), true, nil
}

// UpdateAppMeta implements Store.
func (m *Memory) UpdateAppMeta(_ context.Context, uid, appID string, name, status *string) (*App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	if name != nil {
		a.app.Name = *name
	}
	if status != nil {
		a.app.Status = *status
	}
	a.app.UpdatedAt = m.now().UTC()
	return a.snapshot(), nil
}

// CommitVersion implements Store.
func (m *Memory) CommitVersion(_ context.Context, uid, appID string, c Commit) (*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	if c.BaseVersion != a.app.CurrentVersion {
		return nil, &ConflictError{Current: a.app.CurrentVersion}
	}
	now := m.now().UTC()
	n := a.app.CurrentVersion + 1
	v := &Version{AppID: appID, Version: n, ParentVersion: c.BaseVersion, DSL: cloneRaw(c.DSL), Patch: c.Patch, Source: c.Source, CreatedAt: now}
	a.versions[n] = v
	a.app.CurrentVersion = n
	a.app.DSL = cloneRaw(c.DSL)
	a.app.UpdatedAt = now
	if c.Name != "" {
		a.app.Name = c.Name
	}
	return cloneVersion(v), nil
}

// ListVersions implements Store.
func (m *Memory) ListVersions(_ context.Context, uid, appID string, limit, offset int) ([]Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	out := make([]Version, 0, len(a.versions))
	for _, v := range a.versions {
		out = append(out, *cloneVersion(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return page(out, limit, offset), nil
}

// GetVersion implements Store.
func (m *Memory) GetVersion(_ context.Context, uid, appID string, n int) (*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	v, ok := a.versions[n]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneVersion(v), nil
}

// ListRecords implements Store.
func (m *Memory) ListRecords(_ context.Context, uid, appID, entity string, limit int) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	out := make([]Record, 0, len(a.records[entity]))
	for _, r := range a.records[entity] {
		out = append(out, *cloneRecord(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return page(out, limit, 0), nil
}

// GetRecord implements Store.
func (m *Memory) GetRecord(_ context.Context, uid, appID, entity, id string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	r, ok := a.records[entity][id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRecord(r), nil
}

// CreateRecord implements Store.
func (m *Memory) CreateRecord(_ context.Context, uid, appID, entity string, rec Record) (*Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, false, ErrNotFound
	}
	if a.records[entity] == nil {
		a.records[entity] = map[string]*Record{}
	}
	if existing, ok := a.records[entity][rec.ID]; ok {
		return cloneRecord(existing), false, nil
	}
	now := m.now().UTC()
	rec.Entity = entity
	rec.CreatedAt, rec.UpdatedAt = now, now
	rec.Data = cloneData(rec.Data)
	a.records[entity][rec.ID] = &rec
	return cloneRecord(&rec), true, nil
}

// UpdateRecord implements Store.
func (m *Memory) UpdateRecord(_ context.Context, uid, appID, entity, id string, data map[string]any) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return nil, ErrNotFound
	}
	r, ok := a.records[entity][id]
	if !ok {
		return nil, ErrNotFound
	}
	r.Data = cloneData(data)
	r.UpdatedAt = m.now().UTC()
	return cloneRecord(r), nil
}

// DeleteRecord implements Store.
func (m *Memory) DeleteRecord(_ context.Context, uid, appID, entity, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return ErrNotFound
	}
	if _, ok := a.records[entity][id]; !ok {
		return ErrNotFound
	}
	delete(a.records[entity], id)
	return nil
}

// DeleteAllRecords implements Store.
func (m *Memory) DeleteAllRecords(_ context.Context, uid, appID, entity string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.get(uid, appID)
	if a == nil {
		return ErrNotFound
	}
	delete(a.records, entity)
	return nil
}
