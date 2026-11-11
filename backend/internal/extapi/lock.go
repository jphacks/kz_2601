package extapi

import (
	"context"
	"sync"
)

// keyedLock is one mutex per key, waited on with a context. A key's entry
// goes away when nobody holds or waits for it. The zero value is ready.
type keyedLock struct {
	mu sync.Mutex
	m  map[string]*keyedEntry
}

type keyedEntry struct {
	ch    chan struct{}
	users int
}

// lock takes key's mutex (or gives up with ctx) and returns its unlock.
func (l *keyedLock) lock(ctx context.Context, key string) (func(), error) {
	l.mu.Lock()
	if l.m == nil {
		l.m = map[string]*keyedEntry{}
	}
	e := l.m[key]
	if e == nil {
		e = &keyedEntry{ch: make(chan struct{}, 1)}
		l.m[key] = e
	}
	e.users++
	l.mu.Unlock()
	leave := func() {
		l.mu.Lock()
		if e.users--; e.users == 0 {
			delete(l.m, key)
		}
		l.mu.Unlock()
	}
	select {
	case e.ch <- struct{}{}:
		return func() { <-e.ch; leave() }, nil
	case <-ctx.Done():
		leave()
		return nil, ctx.Err()
	}
}
