package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Memory is a per-process Store (tests, local runs, and the fallback when
// Firestore is unavailable). Counts are not shared between instances.
type Memory struct {
	mu      sync.Mutex
	entries map[string]memEntry
	now     func() time.Time
}

type memEntry struct {
	count    int
	expireAt time.Time
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory {
	return &Memory{entries: map[string]memEntry{}, now: time.Now}
}

// Consume implements Store.
func (m *Memory) Consume(_ context.Context, counters []Counter, weight int, expireAt time.Time) (Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for id, e := range m.entries {
		if now.After(e.expireAt) {
			delete(m.entries, id)
		}
	}
	counts := make([]int, len(counters))
	for i, c := range counters {
		counts[i] = m.entries[c.ID].count
	}
	u := evaluate(counters, counts, weight)
	if u.Blocked < 0 {
		for i, c := range counters {
			m.entries[c.ID] = memEntry{count: u.Counts[i], expireAt: expireAt}
		}
	}
	return u, nil
}
