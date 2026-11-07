package store

import (
	"context"
	"time"
)

var (
	_ RuleStatuses   = (*Memory)(nil)
	_ RunnerActivity = (*Memory)(nil)
)

// PutRuleStatus implements RuleStatuses.
func (m *Memory) PutRuleStatus(_ context.Context, env string, s RuleStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ruleStatus == nil {
		m.ruleStatus = map[string]map[string]*RuleStatus{}
	}
	if m.ruleStatus[env] == nil {
		m.ruleStatus[env] = map[string]*RuleStatus{}
	}
	s.Failures = 0
	if old, ok := m.ruleStatus[env][s.ID]; ok && m.now().Before(old.ExpireAt) {
		keepRuleStatus(&s, *old)
	}
	if s.Failed {
		s.Failures++
	}
	s.Failed = false
	m.ruleStatus[env][s.ID] = &s
	return nil
}

// GetRuleStatuses implements RunnerActivity.
func (m *Memory) GetRuleStatuses(_ context.Context, env string, ids []string) (map[string]RuleStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	out := map[string]RuleStatus{}
	for _, id := range ids {
		if s, ok := m.ruleStatus[env][id]; ok && now.Before(s.ExpireAt) {
			out[id] = *s
		}
	}
	return out, nil
}

// keepRuleStatus fills what s leaves empty from the stored old.
func keepRuleStatus(s *RuleStatus, old RuleStatus) {
	if s.LastOKAt.IsZero() {
		s.LastOKAt = old.LastOKAt
		s.Failures = old.Failures
	}
	if s.QuotaDay == "" {
		s.QuotaDay, s.QuotaUsed, s.QuotaLimit = old.QuotaDay, old.QuotaUsed, old.QuotaLimit
	}
}

// ActiveStops implements RunnerActivity.
func (m *Memory) ActiveStops(ctx context.Context, env string, ids []string, now time.Time) (map[string]StopState, error) {
	got, err := m.GetStops(ctx, env, ids)
	if err != nil {
		return nil, err
	}
	return activeStops(got, now), nil
}
