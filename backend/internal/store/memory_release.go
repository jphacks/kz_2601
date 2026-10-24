package store

import (
	"context"
	"sort"
)

func cloneRelease(r *ReleaseRequest, withDSL bool) *ReleaseRequest {
	cp := *r
	cp.DSL = nil
	if withDSL {
		cp.DSL = cloneRaw(r.DSL)
	}
	return &cp
}

// CreateReleaseRequest implements Store.
func (m *Memory) CreateReleaseRequest(_ context.Context, rr ReleaseRequest) (*ReleaseRequest, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.releases == nil {
		m.releases = map[string]*ReleaseRequest{}
	}
	for _, old := range m.releases {
		if old.OwnerUID == rr.OwnerUID && old.AppID == rr.AppID && ReleaseOpen(old.Status) {
			return cloneRelease(old, false), false, nil
		}
	}
	now := m.now().UTC()
	rr.ID, rr.Status, rr.CreatedAt, rr.UpdatedAt = NewID("rr_"), ReleaseReceived, now, now
	m.releases[rr.ID] = cloneRelease(&rr, true)
	return cloneRelease(&rr, false), true, nil
}

// ListReleaseRequests implements Store.
func (m *Memory) ListReleaseRequests(_ context.Context, uid, appID string) ([]ReleaseRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []ReleaseRequest{}
	for _, r := range m.releases {
		if r.OwnerUID == uid && (appID == "" || r.AppID == appID) {
			out = append(out, *cloneRelease(r, false))
		}
	}
	sortReleases(out)
	return page(out, MaxReleaseRequestsPerUser, 0), nil
}

// ReleaseSnapshot returns the stored request including its DSL (tests only).
func (m *Memory) ReleaseSnapshot(id string) (*ReleaseRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.releases[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRelease(r, true), nil
}

// SetReleaseStatus stands in for the admin editing status in the Firestore
// console (tests and local demos; not part of Store).
func (m *Memory) SetReleaseStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.releases[id]
	if !ok {
		return ErrNotFound
	}
	r.Status, r.UpdatedAt = status, m.now().UTC()
	return nil
}

func sortReleases(rs []ReleaseRequest) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].CreatedAt.Equal(rs[j].CreatedAt) {
			return rs[i].CreatedAt.After(rs[j].CreatedAt)
		}
		return rs[i].ID > rs[j].ID
	})
}
