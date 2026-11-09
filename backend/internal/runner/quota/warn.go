package quota

import (
	"context"
	"strconv"
	"sync"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Levels of the creator's daily-limit push (#345, #338 「80% / 100% で 1 日 1 回」).
const (
	WarnNear = 80
	WarnFull = 100
)

// Warning is the level of the app's daily executions this run reached:
// WarnFull when the app's counter stopped it, WarnNear when it ran with the
// counter at 80% or more, 0 otherwise. Only the app's counter counts (the
// push names the app); retries read no counter, so they warn nothing.
func (d Decision) Warning() int {
	if d.Block != nil && d.Block.Scope == ScopeAppExec {
		return WarnFull
	}
	if d.Allowed && d.AppExec != nil && d.AppExec.Limit > 0 && d.AppExec.Used*5 >= d.AppExec.Limit*4 {
		return WarnNear
	}
	return 0
}

// warned remembers the warnings this instance already claimed (or saw
// claimed) on one day, so a busy app does not hit the store on every run.
type warned struct {
	mu  sync.Mutex
	day string
	ids map[string]bool
}

func (w *warned) has(day, id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.day == day && w.ids[id]
}

func (w *warned) add(day, id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.day != day {
		w.day, w.ids = day, map[string]bool{}
	}
	w.ids[id] = true
}

// ClaimWarning reports whether the caller is the first, on dec's day, to
// warn the creator of dec's app at level: a Consume of the limit-1 counter
// rq_warn_app_{appId}_{level} (atomic across instances; the memory fallback
// when the store fails, like every counter). The counter is not a quota: the
// warning itself counts against nothing (#338).
func (q *Quota) ClaimWarning(ctx context.Context, dec Decision, level int) (bool, error) {
	if level != WarnNear && level != WarnFull || dec.run.AppID == "" {
		return false, nil
	}
	id := q.id(dec.Day, "rq_warn_app_"+docSafe(dec.run.AppID)+"_"+strconv.Itoa(level))
	if q.warned.has(dec.Day, id) {
		return false, nil
	}
	c := ratelimit.Counter{ID: id, Scope: "warn", Limit: 1}
	u, _, err := q.consume(ctx, []ratelimit.Counter{c}, 1, dec.expireAt)
	if err != nil {
		return false, err
	}
	q.warned.add(dec.Day, id)
	return u.Blocked < 0, nil
}
