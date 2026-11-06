package store

import "context"

var _ Counter = (*Memory)(nil)

// CountApps implements Counter.
func (m *Memory) CountApps(_ context.Context, uid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.users[uid]), nil
}

// CountRecords implements Counter.
func (m *Memory) CountRecords(_ context.Context, rs RecordSet, field string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var recs map[string]*Record
	if rs.Public {
		if p := m.public[rs.AppID]; p != nil {
			recs = p.records[rs.Entity]
		}
	} else if a := m.get(rs.UID, rs.AppID); a != nil {
		recs = a.records[rs.Entity]
	}
	n := 0
	for _, r := range recs {
		if field == "" || r.Data[field] != nil {
			n++
		}
	}
	return n, nil
}
