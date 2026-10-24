package store

import "context"

// DeleteUserData implements Accounts.
func (m *Memory) DeleteUserData(_ context.Context, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.public {
		if p.app != nil && p.app.OwnerUID == uid {
			p.app = nil
		}
		for _, recs := range p.records {
			for id, r := range recs {
				if r.Author == uid {
					delete(recs, id)
				}
			}
		}
	}
	for token, s := range m.shares {
		if s.OwnerUID == uid {
			delete(m.shares, token)
		}
	}
	for id, c := range m.catalog {
		if c.OwnerUID == uid {
			delete(m.catalog, id)
		}
	}
	delete(m.users, uid)
	return nil
}
