package store

import (
	"context"
	"sort"
)

// CreateRecordEmit implements Outbox.
func (m *Memory) CreateRecordEmit(_ context.Context, rs RecordSet, rec Record, emit EmitFunc) (*Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return nil, false, err
	}
	if existing, ok := recs[rs.Entity][rec.ID]; ok {
		return cloneRecord(existing), false, nil
	}
	now := m.now().UTC()
	rec.Entity = rs.Entity
	rec.CreatedAt, rec.UpdatedAt = now, now
	rec.Data = cloneData(rec.Data)
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

// UpdateRecordEmit implements Outbox.
func (m *Memory) UpdateRecordEmit(_ context.Context, rs RecordSet, id string, data map[string]any, emit EmitFunc) (*Record, error) {
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
	next := cloneRecord(r)
	next.Data = cloneData(data)
	next.UpdatedAt = m.now().UTC()
	events, err := m.emitLocked(emit, cloneRecord(r), *cloneRecord(next))
	if err != nil {
		return nil, err
	}
	r.Data, r.UpdatedAt = next.Data, next.UpdatedAt
	m.storeOutboxLocked(events)
	m.hub.publish(recordsKey(rs), 0)
	return cloneRecord(r), nil
}

// emitLocked runs emit and validates its events against the stored ones.
func (m *Memory) emitLocked(emit EmitFunc, prev *Record, next Record) ([]OutboxEvent, error) {
	if emit == nil {
		return nil, nil
	}
	events := emit(prev, next)
	if err := prepareOutbox(events, m.now().UTC()); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if _, dup := m.outbox[ev.Env][ev.EventID]; dup || seen[ev.Env+"/"+ev.EventID] {
			return nil, ErrAlreadyExists
		}
		seen[ev.Env+"/"+ev.EventID] = true
	}
	return events, nil
}

func (m *Memory) storeOutboxLocked(events []OutboxEvent) {
	for i := range events {
		ev := events[i]
		if m.outbox == nil {
			m.outbox = map[string]map[string]*OutboxEvent{}
		}
		if m.outbox[ev.Env] == nil {
			m.outbox[ev.Env] = map[string]*OutboxEvent{}
		}
		m.outbox[ev.Env][ev.EventID] = &ev
	}
}

// OutboxEvents returns env's queued events (oldest first, up to limit; 0 = all).
func (m *Memory) OutboxEvents(_ context.Context, env string, limit int) ([]OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]OutboxEvent, 0, len(m.outbox[env]))
	for _, ev := range m.outbox[env] {
		out = append(out, *ev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EventID < out[j].EventID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
