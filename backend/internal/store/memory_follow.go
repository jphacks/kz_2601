package store

import (
	"context"
	"slices"
)

// SetFollow implements Follows.
func (m *Memory) SetFollow(_ context.Context, appID, uid, target string, on bool) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, changed, err := nextFollows(m.follows[appID][uid], target, on)
	if err != nil || !changed {
		return len(next), err
	}
	if m.follows == nil {
		m.follows = map[string]map[string][]string{}
	}
	if m.follows[appID] == nil {
		m.follows[appID] = map[string][]string{}
	}
	if len(next) == 0 {
		delete(m.follows[appID], uid)
	} else {
		m.follows[appID][uid] = next
	}
	return len(next), nil
}

// ListFollows implements Follows.
func (m *Memory) ListFollows(_ context.Context, appID, uid string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.follows[appID][uid]), nil
}

// purgeFollows drops uid's follows in appID and uid from everyone's follows
// there. Callers hold m.mu.
func (m *Memory) purgeFollows(appID, uid string, res *AccountPurgeResult) {
	if _, ok := m.follows[appID][uid]; ok {
		delete(m.follows[appID], uid)
		res.Follows++
	}
	for other, uids := range m.follows[appID] {
		if i := slices.Index(uids, uid); i >= 0 {
			m.follows[appID][other] = slices.Delete(slices.Clone(uids), i, i+1)
			res.Follows++
		}
	}
}
