package entitlement

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// memoryNotifications holds Memory's notification records (#315).
type memoryNotifications struct {
	mu   sync.Mutex
	recs map[string]NotificationRecord
}

// NotificationSeen implements NotifyStore.
func (m *Memory) NotificationSeen(_ context.Context, uuid string) (bool, error) {
	m.notes.mu.Lock()
	defer m.notes.mu.Unlock()
	_, ok := m.notes.recs[uuid]
	return ok, nil
}

// RecordNotification implements NotifyStore.
func (m *Memory) RecordNotification(_ context.Context, rec NotificationRecord) error {
	m.notes.mu.Lock()
	defer m.notes.mu.Unlock()
	if m.notes.recs == nil {
		m.notes.recs = map[string]NotificationRecord{}
	}
	if _, ok := m.notes.recs[rec.UUID]; !ok {
		m.notes.recs[rec.UUID] = rec
	}
	return nil
}

// Bindings implements NotifyStore.
func (m *Memory) Bindings(_ context.Context, after string, limit int) ([]Binding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.bindings))
	for otid := range m.bindings {
		if otid > after && !strings.HasPrefix(otid, playKeyPrefix) { // App Store only (like Firestore)
			ids = append(ids, otid)
		}
	}
	slices.Sort(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]Binding, len(ids))
	for i, otid := range ids {
		out[i] = Binding{OriginalTransactionID: otid, UID: m.bindings[otid]}
	}
	return out, nil
}
