package extapi

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// Memory is ConnectionStore and StateStore in memory (tests, local runs).
type Memory struct {
	mu     sync.Mutex
	conns  map[string]map[string]Connection
	states map[string]OAuthState
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{conns: map[string]map[string]Connection{}, states: map[string]OAuthState{}}
}

var (
	_ ConnectionStore = (*Memory)(nil)
	_ StateStore      = (*Memory)(nil)
)

// Get implements ConnectionStore.
func (m *Memory) Get(_ context.Context, uid, provider string) (Connection, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[uid][provider]
	c.Sealed = slices.Clone(c.Sealed)
	return c, ok, nil
}

// Put implements ConnectionStore.
func (m *Memory) Put(_ context.Context, uid string, c Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conns[uid] == nil {
		m.conns[uid] = map[string]Connection{}
	}
	c.Sealed = slices.Clone(c.Sealed)
	m.conns[uid][c.Provider] = c
	return nil
}

// Delete implements ConnectionStore.
func (m *Memory) Delete(_ context.Context, uid, provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns[uid], provider)
	return nil
}

// List implements ConnectionStore.
func (m *Memory) List(_ context.Context, uid string) ([]Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Connection
	for _, c := range m.conns[uid] {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Connection) int { return strings.Compare(a.Provider, b.Provider) })
	return out, nil
}

// Due implements ConnectionStore.
func (m *Memory) Due(_ context.Context, now time.Time, limit int) ([]ConnRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ConnRef
	for uid, cs := range m.conns {
		for p, c := range cs {
			if !c.DeleteAt.IsZero() && c.DeleteAt.Before(now) {
				out = append(out, ConnRef{UID: uid, Provider: p})
			}
		}
	}
	slices.SortFunc(out, func(a, b ConnRef) int { return strings.Compare(a.UID+"/"+a.Provider, b.UID+"/"+b.Provider) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// PutState implements StateStore.
func (m *Memory) PutState(_ context.Context, state string, s OAuthState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[state] = s
	return nil
}

// TakeState implements StateStore.
func (m *Memory) TakeState(_ context.Context, state string) (OAuthState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[state]
	delete(m.states, state)
	return s, ok, nil
}
