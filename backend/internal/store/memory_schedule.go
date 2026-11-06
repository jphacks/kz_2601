package store

import (
	"context"
	"sort"
	"time"
)

var _ Schedules = (*Memory)(nil)

// PutSchedule implements Schedules.
func (m *Memory) PutSchedule(_ context.Context, env string, s Schedule) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.schedules == nil {
		m.schedules = map[string]map[string]*Schedule{}
	}
	if m.schedules[env] == nil {
		m.schedules[env] = map[string]*Schedule{}
	}
	now := m.now().UTC()
	if old, ok := m.schedules[env][s.ID]; ok {
		if old.Version > s.Version {
			return false, nil
		}
		if old.SameTiming(s) && old.Status == ScheduleActive {
			s.NextRunAt, s.LastDueAt, s.CreatedAt = old.NextRunAt, old.LastDueAt, old.CreatedAt
		}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.Status == "" {
		s.Status = ScheduleActive
	}
	m.schedules[env][s.ID] = &s
	return true, nil
}

// DeleteSchedule implements Schedules.
func (m *Memory) DeleteSchedule(_ context.Context, env, id string, maxVersion int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.schedules[env][id]
	if !ok || s.Version > maxVersion {
		return false, nil
	}
	delete(m.schedules[env], id)
	return true, nil
}

// ListSchedules implements Schedules.
func (m *Memory) ListSchedules(_ context.Context, env, appID string) ([]Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Schedule
	for _, s := range m.schedules[env] {
		if s.AppID == appID {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ClaimSchedules implements Schedules.
func (m *Memory) ClaimSchedules(_ context.Context, env string, now time.Time, limit int, advance ScheduleAdvance) ([]OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []*Schedule
	for _, s := range m.schedules[env] {
		if s.Status == ScheduleActive && !s.NextRunAt.IsZero() && !s.NextRunAt.After(now) {
			due = append(due, s)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].NextRunAt.Equal(due[j].NextRunAt) {
			return due[i].NextRunAt.Before(due[j].NextRunAt)
		}
		return due[i].ID < due[j].ID
	})
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	var out []OutboxEvent
	for _, s := range due {
		next, ev := advance(*s)
		if ev != nil {
			events := []OutboxEvent{*ev}
			if err := prepareOutbox(events, now); err != nil {
				return out, err
			}
			if _, dup := m.outbox[events[0].Env][events[0].EventID]; !dup {
				m.storeOutboxLocked(events)
				out = append(out, events[0])
			}
		}
		s.LastDueAt = s.NextRunAt
		s.NextRunAt = next
		if next.IsZero() {
			s.Status = ScheduleInvalid
		}
		s.UpdatedAt = m.now().UTC()
	}
	return out, nil
}

func (m *Memory) runnerAppLocked(env, key string) *RunnerApp {
	if m.runnerApps == nil {
		m.runnerApps = map[string]*RunnerApp{}
	}
	a := m.runnerApps[env+"/"+key]
	if a == nil {
		a = &RunnerApp{}
		m.runnerApps[env+"/"+key] = a
	}
	return a
}

// TouchAppOpened implements Schedules.
func (m *Memory) TouchAppOpened(_ context.Context, env, key string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runnerAppLocked(env, key).LastOpenedAt = at.UTC()
	return nil
}

// SetAppTimeZone implements Schedules.
func (m *Memory) SetAppTimeZone(_ context.Context, env, key, tz string, version int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.runnerAppLocked(env, key)
	if a.TimeZoneVersion > version {
		return nil
	}
	a.TimeZone, a.TimeZoneVersion = tz, version
	return nil
}

// RunnerApp implements Schedules.
func (m *Memory) RunnerApp(_ context.Context, env, key string) (RunnerApp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.runnerApps[env+"/"+key]; a != nil {
		return *a, nil
	}
	return RunnerApp{}, nil
}
