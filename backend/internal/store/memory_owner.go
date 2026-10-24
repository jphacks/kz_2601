package store

import "context"

// ClaimApp implements Store.
func (m *Memory) ClaimApp(_ context.Context, uid, appID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners == nil {
		m.owners = map[string]string{}
	}
	if o, ok := m.owners[appID]; ok {
		return o, nil
	}
	m.owners[appID] = uid
	return uid, nil
}

// AppOwner implements Store.
func (m *Memory) AppOwner(_ context.Context, appID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o, ok := m.owners[appID]; ok {
		return o, nil
	}
	return "", ErrNotFound
}

func installKey(id, uid string) string { return id + "\x00" + uid }

// GetInstall implements Store.
func (m *Memory) GetInstall(_ context.Context, id, uid string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.installs[installKey(id, uid)]; ok {
		return a, nil
	}
	return "", ErrNotFound
}

// RecordInstall implements Store.
func (m *Memory) RecordInstall(_ context.Context, id, uid, appID string, count bool) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.installs == nil {
		m.installs = map[string]string{}
	}
	k := installKey(id, uid)
	_, seen := m.installs[k]
	m.installs[k] = appID
	if seen {
		return false, nil
	}
	if c, ok := m.catalog[id]; ok && count {
		c.Installs++
	}
	return true, nil
}
