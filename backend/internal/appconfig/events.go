package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Server-Sent Events for hot reload (G13 Day2 / #82). The stream only says
// *what* changed; clients re-fetch through the normal API, which stays the
// single source of truth (and the only place authorization happens).
//
//	event: hello    data: {"version":7,"entities":["post"]}
//	event: dsl      data: {"version":8}     (then bye: the entity set may have changed)
//	event: records  data: {"entity":"post"}
//	event: bye      data: {}                (planned end; reconnect right away)
//
// A stream lives at most eventsMaxStream (Cloud Run --timeout is 60s for
// appconfig), so a missing "bye" means the listener failed and the client
// should poll until it reconnects.
const (
	defaultEventsMaxStream = 50 * time.Second
	eventsHeartbeat        = 15 * time.Second
	eventsRetryMillis      = 2000
)

// eventsMaxStream reads EVENTS_MAX_STREAM (Go duration); keep it below the
// Cloud Run request timeout.
func eventsMaxStream() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("EVENTS_MAX_STREAM")); err == nil && d > 0 {
		return d
	}
	return defaultEventsMaxStream
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	watcher, ok := h.store.(store.Watcher)
	if !ok {
		fail(w, apiErr(http.StatusNotImplemented, httpx.CodeInternal, "realtime events are not supported by this store"))
		return
	}
	appID, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	uid := uidOf(r)
	cur, err := h.ensureApp(r, appID)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	app, err := h.parsedDSL(uid, cur)
	if err != nil {
		fail(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.eventsMax)
	defer cancel()
	versions, err := watcher.WatchApp(ctx, uid, appID)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	changed := make(chan string, len(app.Entities)+1)
	broken := make(chan struct{})
	entities := make([]string, 0, len(app.Entities))
	for i := range app.Entities {
		ent := &app.Entities[i]
		ch, err := watcher.WatchRecords(ctx, recordSet(r, cur, ent, ent.ID))
		if err != nil {
			fail(w, err)
			return
		}
		entities = append(entities, ent.ID)
		go forwardRecords(ctx, ent.ID, ch, changed, broken)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	send := func(event string, payload any) bool {
		b, _ := json.Marshal(payload)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	version := <-versions
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", eventsRetryMillis); err != nil {
		return
	}
	if !send("hello", map[string]any{"version": version, "entities": entities}) {
		return
	}
	heartbeat := time.NewTicker(eventsHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && r.Context().Err() == nil {
				send("bye", struct{}{})
			}
			return
		case v, ok := <-versions:
			if !ok {
				return // listener failed: no bye, the client polls until it reconnects
			}
			if v == version {
				continue
			}
			send("dsl", map[string]int{"version": v})
			send("bye", struct{}{}) // entities/scopes may differ in the new DSL: resubscribe
			return
		case e := <-changed:
			if !send("records", map[string]string{"entity": e}) {
				return
			}
		case <-broken:
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

// forwardRecords relays one entity's change signals; a closed source before
// ctx ends means the listener failed.
func forwardRecords(ctx context.Context, entity string, src <-chan struct{}, changed chan<- string, broken chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-src:
			if !ok {
				if ctx.Err() == nil {
					select {
					case broken <- struct{}{}:
					case <-ctx.Done():
					}
				}
				return
			}
			select {
			case changed <- entity:
			case <-ctx.Done():
				return
			}
		}
	}
}
