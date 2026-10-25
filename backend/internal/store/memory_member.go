package store

import (
	"context"
	"sort"
)

// AddMember implements Store.
func (m *Memory) AddMember(_ context.Context, appID string, mem Member) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.members == nil {
		m.members = map[string]map[string]*Member{}
	}
	ms := m.members[appID]
	if ms == nil {
		ms = map[string]*Member{}
		m.members[appID] = ms
	}
	if old, ok := ms[mem.UID]; ok {
		if mem.DisplayName != "" {
			old.DisplayName = mem.DisplayName
		}
		return false, nil
	}
	if mem.JoinedAt.IsZero() {
		mem.JoinedAt = m.now().UTC()
	}
	ms[mem.UID] = &mem
	return true, nil
}

// GetMember implements Store.
func (m *Memory) GetMember(_ context.Context, appID, uid string) (*Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mem, ok := m.members[appID][uid]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *mem
	return &cp, nil
}

// ListMembers implements Store.
func (m *Memory) ListMembers(_ context.Context, appID string, limit int) ([]Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Member, 0, len(m.members[appID]))
	for _, mem := range m.members[appID] {
		out = append(out, *mem)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].JoinedAt.Equal(out[j].JoinedAt) {
			return out[i].JoinedAt.Before(out[j].JoinedAt)
		}
		return out[i].UID < out[j].UID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// RemoveMember implements Store.
func (m *Memory) RemoveMember(_ context.Context, appID, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.members[appID][uid]; !ok {
		return ErrNotFound
	}
	delete(m.members[appID], uid)
	return nil
}
