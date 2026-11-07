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

// memPublic is apps_public/{appId}: the published DSL plus shared records.
type memPublic struct {
	app     *PublicApp
	records map[string]map[string]*Record
}

// Memory is an in-process Store for tests and local development.
type Memory struct {
	mu  sync.Mutex
	now func() time.Time
	hub hub // change notifications for Watcher (watch.go)

	// apps, versions, records
	users  map[string]map[string]*memApp
	public map[string]*memPublic

	// sharing and the public catalog
	shares   map[string]*Share          // shares/{token} (memory_share.go)
	catalog  map[string]*CatalogApp     // public_apps/{appId}
	releases map[string]*ReleaseRequest // release_requests/{id} (memory_release.go)
	owners   map[string]string          // app_owners/{appId} -> uid (memory_owner.go)
	installs map[string]string          // public_apps/{id}/installs/{uid} -> appId

	// participants
	members map[string]map[string]*Member // apps_public/{appId}/members/{uid} (memory_member.go)

	// marks
	marks map[string]bool // record/mark/uid keys (memory_marks.go)

	// follows (#270, memory_follow.go): appId -> follower uid -> followed uids
	follows map[string]map[string][]string

	// account deletion
	deleted map[string]time.Time // uid -> deletedAt of DELETE /v1/me (memory_account.go)
	purged  map[string]bool      // uids erased by PurgeAccount + MarkAccountPurged
	// uid -> users/{uid}.firstSeenAt (SeeUser, #339 account age)
	firstSeen map[string]time.Time

	// devices and consent (#269, memory_device.go)
	devices      map[string]map[string]*Device        // users/{uid}/devices/{installationId}
	deviceTokens map[string]deviceRef                 // device_tokens/{tokenKey}
	consents     map[string]map[string]*NotifyConsent // users/{uid}/notifyConsent/{appId}

	// feedback
	feedback map[string]*Feedback // feedback/{id} (memory_feedback.go)

	// moderation
	reports map[string]*Report           // reports/{id} (memory_moderation.go)
	quota   map[string]int               // uid/day -> reports filed
	blocks  map[string]map[string]*Block // users/{uid}/blocks/{blockedUid}

	// matching (#507, memory_matching.go)
	matchPairs map[string]map[string]*MatchPair // apps_public/{appId}/match_pairs/{pairId}
	matchQuota map[string]int                   // uid/day -> interests sent

	// runner outbox (#341, memory_outbox.go)
	outbox map[string]map[string]*OutboxEvent // env -> eventId

	// runner done marks (#341, memory_runner.go): env/key
	runnerDone map[string]bool

	// runner schedules and per-app state (#342, memory_schedule.go)
	schedules  map[string]map[string]*Schedule // env -> id
	runnerApps map[string]*RunnerApp           // env/key

	// runner stops (#346, memory_stop.go)
	stops   map[string]map[string]*Stop // env -> id
	stopLog map[string][]StopEvent      // env -> entries, oldest first

	// runner rule statuses (#347, memory_rule_status.go)
	ruleStatus map[string]map[string]*RuleStatus // env -> id
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{users: map[string]map[string]*memApp{}, public: map[string]*memPublic{}, now: time.Now}
}

var _ Store = (*Memory)(nil)

func (m *Memory) get(uid, appID string) *memApp {
	if apps, ok := m.users[uid]; ok {
		return apps[appID]
	}
	return nil
}

// recordsOf resolves rs to its entity->id map, creating the public bucket on
// demand (public sets have no owning app document).
func (m *Memory) recordsOf(rs RecordSet) (map[string]map[string]*Record, error) {
	if rs.Public {
		p := m.public[rs.AppID]
		if p == nil {
			p = &memPublic{records: map[string]map[string]*Record{}}
			m.public[rs.AppID] = p
		}
		return p.records, nil
	}
	a := m.get(rs.UID, rs.AppID)
	if a == nil {
		return nil, ErrNotFound
	}
	return a.records, nil
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
	if r.Origin != nil {
		o := *r.Origin
		cp.Origin = &o
	}
	if r.MarkCounts != nil {
		cp.MarkCounts = make(map[string]int, len(r.MarkCounts))
		for k, v := range r.MarkCounts {
			cp.MarkCounts[k] = v
		}
	}
	return &cp
}

func clonePublic(p *PublicApp) *PublicApp {
	cp := *p
	cp.DSL = cloneRaw(p.DSL)
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
	m.hub.publish(appKey(uid, appID), 1)
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

// ReorderApps implements Store.
func (m *Memory) ReorderApps(_ context.Context, uid string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, id := range ids {
		if a := m.get(uid, id); a != nil {
			a.app.SortIndex = i + 1
		}
	}
	return nil
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
	events, err := commitEvents(c, n, now)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if _, dup := m.outbox[ev.Env][ev.EventID]; dup {
			return nil, ErrAlreadyExists
		}
	}
	m.storeOutboxLocked(events)
	if c.Publish != nil {
		m.publishOnCommit(uid, appID, c, n, now)
	}
	v := &Version{AppID: appID, Version: n, ParentVersion: c.BaseVersion, DSL: cloneRaw(c.DSL), Patch: c.Patch, Source: c.Source, CreatedAt: now}
	a.versions[n] = v
	a.app.CurrentVersion = n
	a.app.DSL = cloneRaw(c.DSL)
	a.app.UpdatedAt = now
	if c.Name != "" {
		a.app.Name = c.Name
	}
	m.hub.publish(appKey(uid, appID), n)
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

// PublishApp implements Store.
func (m *Memory) PublishApp(_ context.Context, uid, appID string, raw json.RawMessage, name string, version int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	p := m.public[appID]
	if p == nil {
		p = &memPublic{records: map[string]map[string]*Record{}}
		m.public[appID] = p
	}
	if p.app == nil {
		p.app = &PublicApp{ID: appID, OwnerUID: uid, Name: name, Version: version, DSL: cloneRaw(raw), CreatedAt: now, UpdatedAt: now}
		return true, nil
	}
	if p.app.OwnerUID != uid {
		return false, nil
	}
	p.app.Name, p.app.Version, p.app.DSL, p.app.UpdatedAt = name, version, cloneRaw(raw), now
	return true, nil
}

// publishOnCommit is CommitVersion's apps_public part (see CommitPublish).
// m.mu is held.
func (m *Memory) publishOnCommit(uid, appID string, c Commit, version int, now time.Time) {
	if m.owners == nil {
		m.owners = map[string]string{}
	}
	owner, known := m.owners[appID]
	if !known && c.Publish.Claim {
		m.owners[appID], owner = uid, uid
	}
	p := m.public[appID]
	mirror := p != nil && p.app != nil
	mirrorOwner := ""
	if mirror {
		mirrorOwner = p.app.OwnerUID
	}
	switch decidePublish(c.Publish, uid, owner, mirrorOwner, mirror) {
	case publishCreate:
		if p == nil {
			p = &memPublic{records: map[string]map[string]*Record{}}
			m.public[appID] = p
		}
		p.app = &PublicApp{ID: appID, OwnerUID: uid, Name: c.Name, Version: version, DSL: cloneRaw(c.DSL), CreatedAt: now, UpdatedAt: now}
	case publishUpdate:
		if c.Name != "" {
			p.app.Name = c.Name
		}
		p.app.Version, p.app.DSL, p.app.UpdatedAt = version, cloneRaw(c.DSL), now
	}
}

// GetPublicApp implements Store.
func (m *Memory) GetPublicApp(_ context.Context, appID string) (*PublicApp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.public[appID]
	if p == nil || p.app == nil {
		return nil, ErrNotFound
	}
	return clonePublic(p.app), nil
}

// ListRecords implements Store.
func (m *Memory) ListRecords(_ context.Context, rs RecordSet, limit int) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(recs[rs.Entity]))
	for _, r := range recs[rs.Entity] {
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
func (m *Memory) GetRecord(_ context.Context, rs RecordSet, id string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, err
	}
	r, ok := recs[rs.Entity][id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRecord(r), nil
}

// CreateRecord implements Store.
func (m *Memory) CreateRecord(_ context.Context, rs RecordSet, rec Record) (*Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, false, err
	}
	if recs[rs.Entity] == nil {
		recs[rs.Entity] = map[string]*Record{}
	}
	if existing, ok := recs[rs.Entity][rec.ID]; ok {
		return cloneRecord(existing), false, nil
	}
	now := m.now().UTC()
	rec.Entity = rs.Entity
	rec.CreatedAt, rec.UpdatedAt = now, now
	rec.Data = cloneData(rec.Data)
	recs[rs.Entity][rec.ID] = &rec
	m.hub.publish(recordsKey(rs), 0)
	return cloneRecord(&rec), true, nil
}

// UpdateRecord implements Store.
func (m *Memory) UpdateRecord(_ context.Context, rs RecordSet, id string, data map[string]any) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, err
	}
	r, ok := recs[rs.Entity][id]
	if !ok {
		return nil, ErrNotFound
	}
	r.Data = cloneData(data)
	r.UpdatedAt = m.now().UTC()
	m.hub.publish(recordsKey(rs), 0)
	return cloneRecord(r), nil
}

// DeleteRecord implements Store.
func (m *Memory) DeleteRecord(_ context.Context, rs RecordSet, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return err
	}
	if _, ok := recs[rs.Entity][id]; !ok {
		return ErrNotFound
	}
	watched := inWatchWindow(recs[rs.Entity], id)
	delete(recs[rs.Entity], id)
	m.dropMarks(recordsKey(rs) + "/" + id + "/")
	if watched {
		m.hub.publish(recordsKey(rs), 0)
	}
	return nil
}

// DeleteAllRecords implements Store.
func (m *Memory) DeleteAllRecords(_ context.Context, rs RecordSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return err
	}
	delete(recs, rs.Entity)
	m.dropMarks(recordsKey(rs) + "/")
	m.hub.publish(recordsKey(rs), 0)
	return nil
}
