package store

import (
	"context"
	"sort"
	"strconv"
	"time"
)

var _ RunnerStops = (*Memory)(nil)

// GetStops implements RunnerStops.
func (m *Memory) GetStops(_ context.Context, env string, ids []string) (map[string]Stop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Stop{}
	for _, id := range ids {
		if s, ok := m.stops[env][id]; ok {
			out[id] = *s
		}
	}
	return out, nil
}

func (m *Memory) putStopLocked(env string, s Stop) {
	if m.stops == nil {
		m.stops = map[string]map[string]*Stop{}
	}
	if m.stops[env] == nil {
		m.stops[env] = map[string]*Stop{}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = m.now().UTC()
	}
	m.stops[env][s.ID] = &s
	m.appendStopLogLocked(env, setEvent(s))
}

func (m *Memory) appendStopLogLocked(env string, e StopEvent) {
	if m.stopLog == nil {
		m.stopLog = map[string][]StopEvent{}
	}
	e.ID = strconv.Itoa(len(m.stopLog[env]) + 1)
	e.Until, e.At = e.Until.UTC(), e.At.UTC()
	m.stopLog[env] = append(m.stopLog[env], e)
}

// StopLog implements RunnerStops.
func (m *Memory) StopLog(_ context.Context, env, appID string, limit int) ([]StopEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []StopEvent
	for i := len(m.stopLog[env]) - 1; i >= 0; i-- {
		if e := m.stopLog[env][i]; appID == "" || e.AppID == appID {
			out = append(out, e)
		}
	}
	return page(out, limit, 0), nil
}

// PutStop implements RunnerStops.
func (m *Memory) PutStop(_ context.Context, env string, s Stop) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putStopLocked(env, s)
	return nil
}

// PutStopIfInactive implements RunnerStops.
func (m *Memory) PutStopIfInactive(_ context.Context, env string, s Stop, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.stops[env][s.ID]; ok && old.Active(now) {
		return false, nil
	}
	m.putStopLocked(env, s)
	return true, nil
}

// LiftStop implements RunnerStops.
func (m *Memory) LiftStop(_ context.Context, env, id string, at time.Time, operator string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.stops[env][id]
	if !ok || !s.Active(at) {
		return false, nil
	}
	s.LiftedAt = at.UTC()
	m.appendStopLogLocked(env, liftEvent(*s, at, operator))
	return true, nil
}

// ListStops implements RunnerStops.
func (m *Memory) ListStops(_ context.Context, env string) ([]Stop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Stop, 0, len(m.stops[env]))
	for _, s := range m.stops[env] {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ReportsSince implements RunnerStops.
func (m *Memory) ReportsSince(_ context.Context, since time.Time, limit int) ([]Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Report
	for _, r := range m.reports {
		if !r.CreatedAt.Before(since) {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, limit, 0), nil
}
