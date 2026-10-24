package store

import (
	"context"
	"log/slog"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// watchRecordsWindow bounds the realtime record listener to the most recently
// updated documents: a listener is billed one read per matching document on
// every (re)connect, and creates/updates always land at the top. A delete of a
// record older than the window is not signalled (the next pull still drops it).
const watchRecordsWindow = 100

var _ Watcher = (*Firestore)(nil)

// WatchApp implements Watcher with a snapshot listener on users/{uid}/apps/{appId}.
func (f *Firestore) WatchApp(ctx context.Context, uid, appID string) (<-chan int, error) {
	it := f.appRef(uid, appID).Snapshots(ctx)
	snap, err := it.Next()
	if err != nil {
		it.Stop()
		return nil, err
	}
	if !snap.Exists() {
		it.Stop()
		return nil, ErrNotFound
	}
	out := make(chan int, 1)
	out <- versionOf(snap)
	go func() {
		defer close(out)
		defer it.Stop()
		for {
			snap, err := it.Next()
			if err != nil {
				logWatchEnd(ctx, "app", err)
				return
			}
			if snap.Exists() {
				offer(out, versionOf(snap))
			}
		}
	}()
	return out, nil
}

// WatchRecords implements Watcher with a snapshot listener on the record
// collection (newest watchRecordsWindow by updatedAt).
func (f *Firestore) WatchRecords(ctx context.Context, rs RecordSet) (<-chan struct{}, error) {
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
				logWatchEnd(ctx, "records", err)
				return
			}
			if len(snap.Changes) > 0 {
				offer(out, struct{}{})
			}
		}
	}()
	return out, nil
}

func versionOf(snap *firestore.DocumentSnapshot) int {
	v, err := snap.DataAt("currentVersion")
	if err != nil {
		return 0
	}
	n, _ := v.(int64)
	return int(n)
}

// logWatchEnd logs listener failures; a cancelled request context is the normal end.
func logWatchEnd(ctx context.Context, what string, err error) {
	if ctx.Err() != nil || status.Code(err) == codes.Canceled {
		return
	}
	slog.Warn("store: snapshot listener ended", slog.String("watch", what), slog.Any("error", err))
}
