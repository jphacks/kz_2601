package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/firestore"
)

// VisibleWatcher (#271) is WatchRecords for a record set some of whose
// records the watcher may not read (addressed messages): it signals only a
// change to a record visible reports true for, so the stream does not tell
// a participant when others write to each other.
type VisibleWatcher interface {
	WatchVisibleRecords(ctx context.Context, rs RecordSet, visible func(*Record) bool) (<-chan struct{}, error)
}

var (
	_ VisibleWatcher = (*Memory)(nil)
	_ VisibleWatcher = (*Firestore)(nil)
)

// WatchVisibleRecords implements VisibleWatcher: on every change of the set
// it compares the visible records of the watch window with the last state.
func (m *Memory) WatchVisibleRecords(ctx context.Context, rs RecordSet, visible func(*Record) bool) (<-chan struct{}, error) {
	src := m.hub.subscribe(ctx, recordsKey(rs))
	last := m.visibleState(rs, visible)
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-src:
				if cur := m.visibleState(rs, visible); cur != last {
					last = cur
					offer(out, struct{}{})
				}
			}
		}
	}()
	return out, nil
}

// visibleState fingerprints the visible records of the watch window (id,
// updatedAt, mark counts).
func (m *Memory) visibleState(rs RecordSet, visible func(*Record) bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return ""
	}
	set := recs[rs.Entity]
	parts := make([]string, 0, len(set))
	for id, r := range set {
		if !inWatchWindow(set, id) || !visible(cloneRecord(r)) {
			continue
		}
		marks := make([]string, 0, len(r.MarkCounts))
		for k, v := range r.MarkCounts {
			marks = append(marks, fmt.Sprintf("%s=%d", k, v))
		}
		sort.Strings(marks)
		parts = append(parts, id+"@"+r.UpdatedAt.String()+"#"+strings.Join(marks, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// WatchVisibleRecords implements VisibleWatcher with the WatchRecords
// listener, signalling only changes whose document (for a removal, the
// document as it was) is visible.
func (f *Firestore) WatchVisibleRecords(ctx context.Context, rs RecordSet, visible func(*Record) bool) (<-chan struct{}, error) {
	it := f.recordsCol(rs).OrderBy("updatedAt", firestore.Desc).Limit(watchRecordsWindow).Snapshots(ctx)
	if _, err := it.Next(); err != nil { // initial state: not a change
		it.Stop()
		return nil, err
	}
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		defer it.Stop()
		for {
			snap, err := it.Next()
			if err != nil {
				logWatchEnd(ctx, "visible records", err)
				return
			}
			for _, ch := range snap.Changes {
				var d recordDoc
				if ch.Doc == nil || ch.Doc.DataTo(&d) != nil {
					continue
				}
				if visible(d.toRecord(ch.Doc.Ref.ID)) {
					offer(out, struct{}{})
					break
				}
			}
		}
	}()
	return out, nil
}
