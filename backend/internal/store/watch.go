package store

import (
	"context"
	"sync"
)

// Watcher is implemented by stores that can push change notifications
// (GET /v1/apps/{appId}/events). Both channels are closed when ctx ends or
// the underlying listener fails; callers then end the stream and the client
// reconnects (or falls back to polling).
type Watcher interface {
	// WatchApp sends the app's currentVersion: once with the current value,
	// then on every change. Slow consumers only see the latest value.
	WatchApp(ctx context.Context, uid, appID string) (<-chan int, error)
	// WatchRecords signals each change to the record set after its initial state.
	WatchRecords(ctx context.Context, rs RecordSet) (<-chan struct{}, error)
}

// offer sends v without blocking, replacing a value the consumer has not read yet.
func offer[T any](ch chan T, v T) {
	for {
		select {
		case ch <- v:
			return
		default:
		}
		select {
		case <-ch:
		default:
		}
	}
}

func appKey(uid, appID string) string { return "app/" + uid + "/" + appID }

func recordsKey(rs RecordSet) string {
	if rs.Public {
		return "pub/" + rs.AppID + "/" + rs.Entity
	}
	return "priv/" + rs.UID + "/" + rs.AppID + "/" + rs.Entity
}

// hub fans in-process notifications out to Memory watchers.
type hub struct {
	mu   sync.Mutex
	subs map[string]map[chan int]struct{}
}

func (h *hub) subscribe(ctx context.Context, key string) chan int {
	ch := make(chan int, 1)
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[string]map[chan int]struct{}{}
	}
	if h.subs[key] == nil {
		h.subs[key] = map[chan int]struct{}{}
	}
	h.subs[key][ch] = struct{}{}
	h.mu.Unlock()
	go func() {
		<-ctx.Done()
		h.mu.Lock()
		delete(h.subs[key], ch)
		if len(h.subs[key]) == 0 {
			delete(h.subs, key)
		}
		h.mu.Unlock()
	}()
	return ch
}

func (h *hub) publish(key string, v int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[key] {
		offer(ch, v)
	}
}

var _ Watcher = (*Memory)(nil)

// WatchApp implements Watcher.
func (m *Memory) WatchApp(ctx context.Context, uid, appID string) (<-chan int, error) {
	m.mu.Lock()
	a := m.get(uid, appID)
	var cur int
	if a != nil {
		cur = a.app.CurrentVersion
	}
	m.mu.Unlock()
	if a == nil {
		return nil, ErrNotFound
	}
	src := m.hub.subscribe(ctx, appKey(uid, appID))
	out := make(chan int, 1)
	out <- cur
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-src:
				offer(out, v)
			}
		}
	}()
	return out, nil
}

// WatchRecords implements Watcher.
func (m *Memory) WatchRecords(ctx context.Context, rs RecordSet) (<-chan struct{}, error) {
	src := m.hub.subscribe(ctx, recordsKey(rs))
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-src:
				offer(out, struct{}{})
			}
		}
	}()
	return out, nil
}
