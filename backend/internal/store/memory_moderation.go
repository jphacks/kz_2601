package store

import (
	"context"
	"sort"
)

// AddReport implements Moderation.
func (m *Memory) AddReport(_ context.Context, r Report, day string, limit int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reports[r.ID]; ok {
		return false, nil
	}
	if m.reports == nil {
		m.reports, m.quota = map[string]*Report{}, map[string]int{}
	}
	key := r.ReporterUID + "/" + day
	if limit > 0 && m.quota[key] >= limit {
		return false, ErrRateLimited
	}
	m.quota[key]++
	if r.CreatedAt.IsZero() {
		r.CreatedAt = m.now().UTC()
	}
	cp := r
	m.reports[r.ID] = &cp
	return true, nil
}

// ListReports implements Moderation.
func (m *Memory) ListReports(_ context.Context, limit int) ([]Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Report, 0, len(m.reports))
	for _, r := range m.reports {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, limit, 0), nil
}

// SetReportStatus implements Moderation.
func (m *Memory) SetReportStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reports[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	return nil
}

// PutBlock implements Moderation.
func (m *Memory) PutBlock(_ context.Context, uid string, b Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blocks == nil {
		m.blocks = map[string]map[string]*Block{}
	}
	if m.blocks[uid] == nil {
		m.blocks[uid] = map[string]*Block{}
	}
	if old, ok := m.blocks[uid][b.UID]; ok {
		b.CreatedAt = old.CreatedAt
	} else if b.CreatedAt.IsZero() {
		b.CreatedAt = m.now().UTC()
	}
	m.blocks[uid][b.UID] = &b
	return nil
}

// DeleteBlock implements Moderation.
func (m *Memory) DeleteBlock(_ context.Context, uid, blockedUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blocks[uid], blockedUID)
	return nil
}

// ListBlocks implements Moderation.
func (m *Memory) ListBlocks(_ context.Context, uid string) ([]Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Block, 0, len(m.blocks[uid]))
	for _, b := range m.blocks[uid] {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return page(out, MaxBlocks, 0), nil
}

func (m *Memory) moderationState(hidden bool, reason string) ModerationState {
	if !hidden {
		return ModerationState{}
	}
	return ModerationState{Hidden: true, HiddenReason: reason, HiddenAt: m.now().UTC()}
}

// SetCatalogHidden implements Moderation.
func (m *Memory) SetCatalogHidden(_ context.Context, id string, hidden bool, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.catalog[id]
	if !ok {
		return ErrNotFound
	}
	c.ModerationState = m.moderationState(hidden, reason)
	return nil
}

// SetPublicAppHidden implements Moderation.
func (m *Memory) SetPublicAppHidden(_ context.Context, appID string, hidden bool, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.public[appID]
	if p == nil || p.app == nil {
		return ErrNotFound
	}
	p.app.ModerationState = m.moderationState(hidden, reason)
	return nil
}
