package store

import "context"

// DeactivateUser implements Accounts.
func (m *Memory) DeactivateUser(_ context.Context, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	if m.deleted == nil {
		m.deleted = map[string]bool{}
	}
	m.deleted[uid] = true
	for _, a := range m.users[uid] {
		if a.app.Status != "archived" {
			a.app.Status, a.app.UpdatedAt = "archived", now
		}
	}
	for _, s := range m.shares {
		if s.OwnerUID == uid {
			s.Revoked = true
		}
	}
	for id, c := range m.catalog {
		if c.OwnerUID == uid {
			delete(m.catalog, id)
		}
	}
	return nil
}

// UserDeleted implements Accounts.
func (m *Memory) UserDeleted(_ context.Context, uid string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleted[uid], nil
}
