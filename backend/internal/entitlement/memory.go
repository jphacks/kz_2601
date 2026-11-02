package entitlement

import (
	"context"
	"sync"
	"time"
)

// Memory is the Store for tests and local runs without Firestore.
type Memory struct {
	mu       sync.Mutex
	pro      map[string]Pro    // uid -> Pro
	bindings map[string]string // originalTransactionId -> uid
	now      func() time.Time
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{pro: map[string]Pro{}, bindings: map[string]string{}, now: time.Now}
}

// Get implements Store.
func (m *Memory) Get(_ context.Context, uid string) (*Pro, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pro[uid]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

// Owner implements Store.
func (m *Memory) Owner(_ context.Context, otid string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bindings[otid], nil
}

// Apply implements Store.
func (m *Memory) Apply(_ context.Context, uid string, p Pro, transferFrom string) (*Pro, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !owns(m.bindings[p.OriginalTransactionID], uid, transferFrom) {
		return nil, ErrOwnedByOther
	}
	m.bindings[p.OriginalTransactionID] = uid
	var cur *Pro
	if c, ok := m.pro[uid]; ok {
		cur = &c
	}
	out, write := merge(cur, p, m.now())
	if write {
		m.pro[uid] = out
	}
	return &out, nil
}

// PurgeUser implements Store.
func (m *Memory) PurgeUser(_ context.Context, uid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	if _, ok := m.pro[uid]; ok {
		delete(m.pro, uid)
		n++
	}
	for otid, owner := range m.bindings {
		if owner == uid {
			delete(m.bindings, otid)
			n++
		}
	}
	return n, nil
}
