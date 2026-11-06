package store

import (
	"context"
	"maps"
	"sort"
	"time"
)

var (
	_ OutboxQueue  = (*Memory)(nil)
	_ RunnerWrites = (*Memory)(nil)
)

// ClaimOutbox implements OutboxQueue.
func (m *Memory) ClaimOutbox(_ context.Context, env string, now time.Time, lease time.Duration, limit int) ([]OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []*OutboxEvent
	for _, ev := range m.outbox[env] {
		if !ev.AvailableAt.IsZero() && !ev.AvailableAt.After(now) {
			due = append(due, ev)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].AvailableAt.Equal(due[j].AvailableAt) {
			return due[i].AvailableAt.Before(due[j].AvailableAt)
		}
		return due[i].EventID < due[j].EventID
	})
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	out := make([]OutboxEvent, 0, len(due))
	for _, ev := range due {
		ev.AvailableAt = now.Add(lease)
		ev.Attempts++
		out = append(out, cloneOutbox(ev))
	}
	return out, nil
}

// FinishOutbox implements OutboxQueue.
func (m *Memory) FinishOutbox(_ context.Context, env, eventID string, attempt int, res OutboxResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, err := m.leasedLocked(env, eventID, attempt)
	if err != nil {
		return err
	}
	ev.AvailableAt = time.Time{}
	ev.Status, ev.Outcome, ev.RuleOutcomes, ev.LastError, ev.FinishedAt = res.Status, res.Outcome, maps.Clone(res.RuleOutcomes), res.LastError, res.FinishedAt
	return nil
}

// RetryOutbox implements OutboxQueue.
func (m *Memory) RetryOutbox(_ context.Context, env, eventID string, attempt int, at time.Time, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, err := m.leasedLocked(env, eventID, attempt)
	if err != nil {
		return err
	}
	ev.AvailableAt, ev.Status, ev.LastError = at, OutboxRetry, lastError
	return nil
}

func (m *Memory) leasedLocked(env, eventID string, attempt int) (*OutboxEvent, error) {
	ev, ok := m.outbox[env][eventID]
	if !ok {
		return nil, ErrNotFound
	}
	if ev.AvailableAt.IsZero() || ev.Attempts != attempt {
		return nil, ErrLeaseLost
	}
	return ev, nil
}

func cloneOutbox(ev *OutboxEvent) OutboxEvent {
	cp := *ev
	cp.Data, cp.Prev = cloneData(ev.Data), cloneData(ev.Prev)
	if ev.Data == nil {
		cp.Data = nil
	}
	if ev.Prev == nil {
		cp.Prev = nil
	}
	cp.RuleIDs = append([]string(nil), ev.RuleIDs...)
	cp.ChangedFields = append([]string(nil), ev.ChangedFields...)
	cp.RuleOutcomes = maps.Clone(ev.RuleOutcomes)
	return cp
}

// RunnerMarked implements RunnerWrites.
func (m *Memory) RunnerMarked(_ context.Context, env, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runnerDone[env+"/"+runnerMarkID(key)], nil
}

// MarkRunnerDone implements RunnerWrites.
func (m *Memory) MarkRunnerDone(_ context.Context, env, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markLocked(env, key)
	return nil
}

func (m *Memory) markLocked(env, key string) {
	if m.runnerDone == nil {
		m.runnerDone = map[string]bool{}
	}
	m.runnerDone[env+"/"+runnerMarkID(key)] = true
}

// ApplyRecordOnce implements RunnerWrites.
func (m *Memory) ApplyRecordOnce(_ context.Context, rs RecordSet, id, env, key string, fn func(map[string]any) map[string]any) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runnerDone[env+"/"+runnerMarkID(key)] {
		return false, nil
	}
	recs, err := m.recordsOf(rs)
	if err != nil {
		return false, err
	}
	r, ok := recs[rs.Entity][id]
	if !ok {
		return false, ErrNotFound
	}
	r.Data = cloneData(fn(cloneData(r.Data)))
	r.UpdatedAt = m.now().UTC()
	m.markLocked(env, key)
	m.hub.publish(recordsKey(rs), 0)
	return true, nil
}
