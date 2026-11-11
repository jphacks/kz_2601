package loop

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryControl is an in-memory Control for tests and local runs.
type MemoryControl struct {
	mu       sync.Mutex
	events   []Event
	switches map[string]bool
	usage    map[string]int
	// Fail, when set, makes every call return it (fail-closed tests).
	Fail error
}

// NewMemoryControl returns an empty MemoryControl.
func NewMemoryControl() *MemoryControl {
	return &MemoryControl{switches: map[string]bool{}, usage: map[string]int{}}
}

var _ Control = (*MemoryControl)(nil)

// AppendAudit implements Control.
func (m *MemoryControl) AppendAudit(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.events = append(m.events, e)
	return nil
}

// ListAudit implements Control.
func (m *MemoryControl) ListAudit(_ context.Context, q AuditQuery) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	out := []Event{}
	for _, e := range m.events {
		if e.At.Before(q.Since) || (q.DraftID != "" && e.DraftID != q.DraftID) || (q.ItemID != "" && e.ItemID != q.ItemID) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if n := limitOr(q.Limit, 200); len(out) > n {
		out = out[len(out)-n:]
	}
	return out, nil
}

// Switches implements Control.
func (m *MemoryControl) Switches(context.Context) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	out := map[string]bool{}
	for k, v := range m.switches {
		out[k] = v
	}
	return out, nil
}

// SetSwitch implements Control.
func (m *MemoryControl) SetSwitch(_ context.Context, name string, on bool, _ string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.switches[name] = on
	return nil
}

// AddUsage implements Control.
func (m *MemoryControl) AddUsage(_ context.Context, month string, tokens int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return 0, m.Fail
	}
	m.usage[month] += tokens
	return m.usage[month], nil
}

// Usage implements Control.
func (m *MemoryControl) Usage(_ context.Context, month string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return 0, m.Fail
	}
	return m.usage[month], nil
}
