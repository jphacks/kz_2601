package loop

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is an in-memory Store for tests and local runs.
type Memory struct {
	mu      sync.Mutex
	items   map[string]Item
	drafts  map[string]Draft
	notices map[string]Notice
	cursors map[Source]time.Time
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory {
	return &Memory{items: map[string]Item{}, drafts: map[string]Draft{}, notices: map[string]Notice{}, cursors: map[Source]time.Time{}}
}

var _ Store = (*Memory)(nil)

func limitOr(n, def int) int {
	if n <= 0 {
		return def
	}
	return min(n, 500)
}

// PutItem implements Store.
func (m *Memory) PutItem(_ context.Context, it Item) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[it.ID]; ok {
		return false, nil
	}
	m.items[it.ID] = cloneItem(it)
	return true, nil
}

// GetItem implements Store.
func (m *Memory) GetItem(_ context.Context, id string) (*Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := cloneItem(it)
	return &cp, nil
}

// UpdateItem implements Store.
func (m *Memory) UpdateItem(_ context.Context, it Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[it.ID]; !ok {
		return ErrNotFound
	}
	m.items[it.ID] = cloneItem(it)
	return nil
}

// ListItems implements Store.
func (m *Memory) ListItems(_ context.Context, q ItemQuery) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Item{}
	for _, it := range m.items {
		if q.Status == "" || it.Status == q.Status {
			out = append(out, cloneItem(it))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].IngestedAt.Equal(out[j].IngestedAt) {
			return out[i].IngestedAt.Before(out[j].IngestedAt)
		}
		return out[i].ID < out[j].ID
	})
	if n := limitOr(q.Limit, 100); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// PutDraft implements Store.
func (m *Memory) PutDraft(_ context.Context, d Draft) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drafts[d.ID] = cloneDraft(d)
	return nil
}

// GetDraft implements Store.
func (m *Memory) GetDraft(_ context.Context, id string) (*Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drafts[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := cloneDraft(d)
	return &cp, nil
}

// ListDrafts implements Store.
func (m *Memory) ListDrafts(_ context.Context, q DraftQuery) ([]Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Draft{}
	for _, d := range m.drafts {
		if q.Status == "" || d.Status == q.Status {
			out = append(out, cloneDraft(d))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if n := limitOr(q.Limit, 100); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// PutNotice implements Store.
func (m *Memory) PutNotice(_ context.Context, n Notice) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.notices[n.ID]; ok {
		return false, nil
	}
	m.notices[n.ID] = n
	return true, nil
}

// LastPush implements Store.
func (m *Memory) LastPush(_ context.Context, uid string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last time.Time
	for _, n := range m.notices {
		if n.UID == uid && n.Pushed && n.CreatedAt.After(last) {
			last = n.CreatedAt
		}
	}
	return last, nil
}

// ListNotices implements Store.
func (m *Memory) ListNotices(_ context.Context, uid string, limit int) ([]Notice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Notice{}
	for _, n := range m.notices {
		if n.UID == uid {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if n := limitOr(limit, 50); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// Cursor implements Store.
func (m *Memory) Cursor(_ context.Context, src Source) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cursors[src], nil
}

// SetCursor implements Store.
func (m *Memory) SetCursor(_ context.Context, src Source, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cursors[src] = t
	return nil
}

// PurgeExpired implements Store.
func (m *Memory) PurgeExpired(_ context.Context, now time.Time, limit int, dryRun bool) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return purgeMap(m.items, func(it Item) time.Time { return it.ExpiresAt }, now, limit, dryRun) +
		purgeMap(m.drafts, func(d Draft) time.Time { return d.ExpiresAt }, now, limit, dryRun) +
		purgeMap(m.notices, func(n Notice) time.Time { return n.ExpiresAt }, now, limit, dryRun), nil
}

func purgeMap[T any](docs map[string]T, expiry func(T) time.Time, now time.Time, limit int, dryRun bool) int {
	n := 0
	for id, d := range docs {
		if limit > 0 && n >= limit {
			break
		}
		if expiry(d).Before(now) {
			n++
			if !dryRun {
				delete(docs, id)
			}
		}
	}
	return n
}

// DeleteItems implements Store.
func (m *Memory) DeleteItems(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.items, id)
	}
	return nil
}

// ItemsByUID implements Store.
func (m *Memory) ItemsByUID(_ context.Context, uid string) ([]string, []string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items, notices []string
	for id, it := range m.items {
		if it.UID == uid {
			items = append(items, id)
		}
	}
	for id, n := range m.notices {
		if n.UID == uid {
			notices = append(notices, id)
		}
	}
	sort.Strings(items)
	sort.Strings(notices)
	return items, notices, nil
}

// DeleteNotices implements Store.
func (m *Memory) DeleteNotices(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.notices, id)
	}
	return nil
}

func cloneItem(it Item) Item {
	if it.MaskCounts != nil {
		mc := make(map[string]int, len(it.MaskCounts))
		for k, v := range it.MaskCounts {
			mc[k] = v
		}
		it.MaskCounts = mc
	}
	return it
}

func cloneDraft(d Draft) Draft {
	d.ItemIDs = append([]string(nil), d.ItemIDs...)
	d.Sources = append([]string(nil), d.Sources...)
	d.AppVersions = append([]string(nil), d.AppVersions...)
	return d
}
