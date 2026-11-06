package store

import "context"

// heldByOther reports whether a record of recs other than id holds claim
// (memory keeps no lock documents: the mutex makes the scan atomic).
func heldByOther(recs map[string]*Record, id string, claim UniqueClaim) bool {
	if claim.Key == "" {
		return false
	}
	for rid, r := range recs {
		if rid != id && (r.UniqueKey == claim.Key || claim.matches(r)) {
			return true
		}
	}
	return false
}

// CreateRecordUnique implements UniqueRecords.
func (m *Memory) CreateRecordUnique(_ context.Context, rs RecordSet, rec Record, claim UniqueClaim, emit EmitFunc) (*Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, false, err
	}
	if existing, ok := recs[rs.Entity][rec.ID]; ok {
		return cloneRecord(existing), false, nil
	}
	if heldByOther(recs[rs.Entity], rec.ID, claim) {
		return nil, false, ErrDuplicate
	}
	now := m.now().UTC()
	rec.Entity = rs.Entity
	rec.CreatedAt, rec.UpdatedAt = now, now
	rec.Data = cloneData(rec.Data)
	rec.UniqueKey = claim.Key
	events, err := m.emitLocked(emit, nil, rec)
	if err != nil {
		return nil, false, err
	}
	if recs[rs.Entity] == nil {
		recs[rs.Entity] = map[string]*Record{}
	}
	recs[rs.Entity][rec.ID] = &rec
	m.storeOutboxLocked(events)
	m.hub.publish(recordsKey(rs), 0)
	return cloneRecord(&rec), true, nil
}

// UpdateRecordUnique implements UniqueRecords.
func (m *Memory) UpdateRecordUnique(_ context.Context, rs RecordSet, id string, data map[string]any, claim UniqueClaim, emit EmitFunc) (*Record, error) {
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
	if claim.Key != r.UniqueKey && heldByOther(recs[rs.Entity], id, claim) {
		return nil, ErrDuplicate
	}
	next := cloneRecord(r)
	next.Data = cloneData(data)
	next.UpdatedAt = m.now().UTC()
	next.UniqueKey = claim.Key
	events, err := m.emitLocked(emit, cloneRecord(r), *cloneRecord(next))
	if err != nil {
		return nil, err
	}
	r.Data, r.UpdatedAt, r.UniqueKey = next.Data, next.UpdatedAt, next.UniqueKey
	m.storeOutboxLocked(events)
	m.hub.publish(recordsKey(rs), 0)
	return cloneRecord(r), nil
}
