package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/queue"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type runnerStore interface {
	store.Store
	store.OutboxQueue
	store.RunnerWrites
}

type outboxLister interface {
	OutboxEvents(ctx context.Context, env string, limit int) ([]store.OutboxEvent, error)
}

// todoRules: private tasks; a new task writes a log record, counts up the
// project it points to and notifies the owner.
const todoRules = `{
  "schemaVersion": 1,
  "app": {"name": "やること", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "task", "name": "やること", "fields": [
      {"id": "title", "name": "題", "type": "string"},
      {"id": "project", "name": "計画", "type": "ref", "refEntity": "project"}
    ]},
    {"id": "project", "name": "計画", "fields": [
      {"id": "name", "name": "名前", "type": "string"},
      {"id": "count", "name": "数", "type": "integer"}
    ]},
    {"id": "log", "name": "記録", "fields": [{"id": "note", "name": "メモ", "type": "string"}]}
  ],
  "screens": [
    {"id": "home", "title": "やること", "components": [{"type": "list", "entity": "task", "item": {"title": "title"}}]}
  ],
  "serverRules": [
    {"id": "on_task", "on": {"record": "created", "entity": "task"},
     "do": [
       {"createRecord": {"entity": "log", "values": {"note": {"op": "concat", "args": ["追加: ", {"ref": "title"}]}}}},
       {"increment": {"ref": "project", "field": "count"}},
       {"notify": {"to": {"kind": "owner"}, "title": "追加", "body": "{title}"}}
     ]}
  ]
}`

func circleRules(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../pkg/dsl/testdata/server_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func withoutRules(t *testing.T, raw []byte) []byte {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	delete(a, "serverRules")
	out, _ := json.Marshal(a)
	return out
}

type fakePro map[string]bool

func (p fakePro) IsPro(_ context.Context, uid string) bool { return p[uid] }

type oldAccounts struct{}

func (oldAccounts) CreatedAt(context.Context, string) (time.Time, bool, error) {
	return time.Now().Add(-30 * 24 * time.Hour), true, nil
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// harness: appconfig (with RUNNER_OUTBOX=1) and a runner on one store.
type harness struct {
	t      *testing.T
	st     runnerStore
	h      http.Handler
	env    string
	sfx    string
	clock  time.Time
	paused bool
	sender *notify.FakeSender
	tokens notify.MemoryTokens
	r      *runner.Runner
	logs   *syncBuffer
	pro    fakePro
}

type options struct {
	pro fakePro
	age notify.AccountAge
}

func stores(t *testing.T, opts options, f func(t *testing.T, h *harness)) {
	t.Run("memory", func(t *testing.T) { f(t, newHarness(t, store.NewMemory(), "", opts)) })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		fs, err := store.NewFirestore(t.Context(), "demo-kz2601", "(default)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = fs.Close() })
		f(t, newHarness(t, fs, "-"+strconv.FormatInt(time.Now().UnixNano(), 36), opts))
	})
}

func newHarness(t *testing.T, st runnerStore, sfx string, opts options) *harness {
	t.Helper()
	env := "rt" + sfx
	t.Setenv("RUNNER_OUTBOX", "1")
	t.Setenv("APP_ENV", env)
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	h := &harness{t: t, st: st, env: env, sfx: sfx, sender: &notify.FakeSender{}, tokens: notify.MemoryTokens{}, logs: &syncBuffer{}}
	h.h = auth.Middleware(nil, auth.ModeDev)(appconfig.New(st))
	lim := quota.DefaultLimits
	lim.RunWritesFree = 10
	if opts.pro == nil {
		opts.pro = fakePro{}
	}
	h.pro = opts.pro
	q := quota.New(quota.Config{Limits: lim, Env: env}, nil, opts.pro)
	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	q.Logger = logger
	r, err := runner.New(runner.Config{
		Env: env, Store: st, Quota: q, Sender: h.sender, Tokens: h.tokens, Age: opts.age,
		Paused: func() bool { return h.paused }, Now: func() time.Time { return h.clock }, Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.r = r
	return h
}

func (h *harness) do(uid, method, path, body string, hdr map[string]string) (int, map[string]any) {
	h.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(auth.DebugUIDHeader, uid)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.h.ServeHTTP(rr, req)
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

func (h *harness) putDSL(uid, app string, dsl []byte) {
	h.t.Helper()
	_, cur := h.do(uid, http.MethodGet, app+"/dsl", "", nil)
	v, _ := cur["version"].(float64)
	body := `{"dsl":` + string(dsl) + `,"source":{"kind":"ai","mode":"full","transcript":"x","summary":"x","model":"test"}}`
	if code, out := h.do(uid, http.MethodPut, app+"/dsl", body, map[string]string{"If-Match": `"` + strconv.Itoa(int(v)) + `"`}); code != http.StatusCreated {
		h.t.Fatalf("put dsl %s: %d %v", app, code, out)
	}
}

func (h *harness) post(uid, path, body string) string {
	h.t.Helper()
	code, out := h.do(uid, http.MethodPost, path, body, nil)
	if code != http.StatusCreated && code != http.StatusOK {
		h.t.Fatalf("post %s: %d %v", path, code, out)
	}
	return out["id"].(string)
}

// circle makes owner's shared circle app and lets members join.
func (h *harness) circle(id, owner string, dsl []byte, members ...string) string {
	h.t.Helper()
	app := "/v1/apps/" + id
	h.putDSL(owner, app, dsl)
	code, out := h.do(owner, http.MethodPost, app+"/share", "", nil)
	if code != http.StatusCreated {
		h.t.Fatalf("share: %d %v", code, out)
	}
	tok := out["token"].(string)
	for _, m := range members {
		if code, out := h.do(m, http.MethodPost, "/v1/shares/"+tok+"/import", "", nil); code != http.StatusCreated {
			h.t.Fatalf("join %s: %d %v", m, code, out)
		}
	}
	return app
}

// tick runs the runner with the clock at least a second past real time.
func (h *harness) tick() runner.TickResult {
	h.t.Helper()
	if now := time.Now().Add(time.Second); h.clock.Before(now) {
		h.clock = now
	}
	res, err := h.r.Tick(h.t.Context(), 30*time.Second)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func (h *harness) events(appID string) []store.OutboxEvent {
	h.t.Helper()
	all, err := h.st.(outboxLister).OutboxEvents(h.t.Context(), h.env, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []store.OutboxEvent
	for _, ev := range all {
		if ev.AppID == appID {
			out = append(out, ev)
		}
	}
	return out
}

func (h *harness) oneEvent(appID string) store.OutboxEvent {
	h.t.Helper()
	evs := h.events(appID)
	if len(evs) != 1 {
		h.t.Fatalf("want 1 outbox event of %s, got %+v", appID, evs)
	}
	return evs[0]
}

func (h *harness) records(rs store.RecordSet) []store.Record {
	h.t.Helper()
	recs, err := h.st.ListRecords(h.t.Context(), rs, 100)
	if err != nil {
		h.t.Fatal(err)
	}
	return recs
}

func (h *harness) record(rs store.RecordSet, id string) *store.Record {
	h.t.Helper()
	r, err := h.st.GetRecord(h.t.Context(), rs, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func num(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

// privateTodo makes uid's private todo app with one project and one task.
func (h *harness) privateTodo(id, uid string) (app, project, task string) {
	h.t.Helper()
	app = "/v1/apps/" + id
	h.putDSL(uid, app, []byte(todoRules))
	project = h.post(uid, app+"/entities/project/records", `{"data":{"name":"家","count":0}}`)
	task = h.post(uid, app+"/entities/task/records", `{"data":{"title":"牛乳を買う","project":"`+project+`"}}`)
	return app, project, task
}

// #341 完了の条件: a record made through appconfig triggers the rule; the
// rule's record carries origin; a second tick adds nothing; the rule's own
// writes queue nothing (no chain); the owner gets their own notification.
func TestRunnerPrivateRunsOnce(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-TODO"+h.sfx, "uid-todo-owner"+h.sfx
		h.tokens[uid] = []string{"tok-owner"}
		_, project, _ := h.privateTodo(id, uid)
		ev := h.oneEvent(id)

		res := h.tick()
		if res.Done != 1 || res.Retried != 0 {
			t.Fatalf("tick: %+v", res)
		}
		logs := h.records(store.PrivateRecords(uid, id, "log"))
		if len(logs) != 1 || logs[0].Data["note"] != "追加: 牛乳を買う" {
			t.Fatalf("log records: %+v", logs)
		}
		o := logs[0].Origin
		if o == nil || o.RuleID != "on_task" || o.EventID != ev.EventID || o.Version != ev.Version || o.ActorUID != uid {
			t.Fatalf("origin: %+v (event %+v)", o, ev)
		}
		if logs[0].ID != store.IDFromClientID(ev.EventID+"/on_task/0") {
			t.Fatalf("log id %s", logs[0].ID)
		}
		if c := num(h.record(store.PrivateRecords(uid, id, "project"), project).Data["count"]); c != 1 {
			t.Fatalf("count after run: %v", c)
		}
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Token != "tok-owner" || pushes[0].Body != "牛乳を買う" || pushes[0].AppName != "やること" {
			t.Fatalf("pushes: %+v", pushes)
		}
		done := h.oneEvent(id) // the log write queued no event
		if done.Status != store.OutboxDone || done.Outcome != quota.OutcomeOK || done.RuleOutcomes["on_task"] != quota.OutcomeOK || !done.AvailableAt.IsZero() {
			t.Fatalf("event after run: %+v", done)
		}

		h.clock = h.clock.Add(time.Hour)
		if res := h.tick(); res.Claimed != 0 {
			t.Fatalf("second tick: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("logs after second tick: %d", n)
		}
		if c := num(h.record(store.PrivateRecords(uid, id, "project"), project).Data["count"]); c != 1 {
			t.Fatalf("count after second tick: %v", c)
		}

		// runner.run / runner.usage: no content, no raw uid
		out := h.logs.String()
		if !strings.Contains(out, `"event":"runner.run"`) || !strings.Contains(out, `"event":"runner.usage"`) {
			t.Fatalf("logs: %s", out)
		}
		for _, secret := range []string{uid, "牛乳", "追加: "} {
			if strings.Contains(out, secret) {
				t.Fatalf("log contains %q: %s", secret, out)
			}
		}
	})
}

// #341: a run that fails after its writes is retried without writing twice;
// the 4th claim gives up.
func TestRunnerRetryGiveUp(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-RETRY"+h.sfx, "uid-retry"+h.sfx
		h.tokens[uid] = []string{"tok-retry"}
		h.sender.Results = map[string]notify.Result{"tok-retry": {Status: 503, Retryable: true}}
		_, project, _ := h.privateTodo(id, uid)
		q := h.r.Queue()
		for attempt := 1; attempt <= 4; attempt++ {
			res := h.tick()
			ev := h.oneEvent(id)
			if ev.Attempts != attempt {
				t.Fatalf("attempt %d: %+v (tick %+v)", attempt, ev, res)
			}
			if attempt < 4 && (res.Retried != 1 || ev.Status != store.OutboxRetry || ev.LastError != "send_failed") {
				t.Fatalf("attempt %d: %+v (tick %+v)", attempt, ev, res)
			}
			if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
				t.Fatalf("attempt %d: %d log records", attempt, n)
			}
			if c := num(h.record(store.PrivateRecords(uid, id, "project"), project).Data["count"]); c != 1 {
				t.Fatalf("attempt %d: count %v", attempt, c)
			}
			h.clock = h.clock.Add(q.Delay(attempt) + time.Second)
		}
		ev := h.oneEvent(id)
		if ev.Status != store.OutboxGaveUp || ev.Outcome != queue.OutcomeGaveUp || !ev.AvailableAt.IsZero() {
			t.Fatalf("gave up: %+v", ev)
		}
		if n := len(h.sender.Pushes()); n != 3 { // attempts 1-3 tried to send; the 4th did not run
			t.Fatalf("pushes: %d", n)
		}
	})
}

// #341: paused events are finished as skipped_paused and never run later;
// a trigger older than an hour is not run.
func TestRunnerPausedAndStale(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		uid := "uid-pause" + h.sfx
		id := "RUN-PAUSE" + h.sfx
		h.paused = true
		h.privateTodo(id, uid)
		if res := h.tick(); res.Done != 1 {
			t.Fatalf("paused tick: %+v", res)
		}
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeSkippedPaused || ev.Status != store.OutboxDone {
			t.Fatalf("paused event: %+v", ev)
		}
		h.paused = false
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 {
			t.Fatalf("ran after resume: %d", n)
		}

		stale := "RUN-STALE" + h.sfx
		h.privateTodo(stale, uid)
		h.clock = time.Now().Add(2 * time.Hour)
		h.tick()
		if ev := h.oneEvent(stale); ev.Outcome != runner.OutcomeSkippedStale {
			t.Fatalf("stale event: %+v", ev)
		}
		if n := len(h.records(store.PrivateRecords(uid, stale, "log"))); n != 0 {
			t.Fatalf("stale ran: %d", n)
		}
	})
}

// #337: stopping takes effect at once — unpublished by the operator, the
// rule removed from the current DSL, the creator gone, the app deleted —
// and a shared rule needs the creator's Pro.
func TestRunnerStops(t *testing.T) {
	stores(t, options{pro: fakePro{}}, func(t *testing.T, h *harness) {
		cases := []struct {
			name   string
			pro    bool
			stop   func(id, app, owner string)
			want   string
			reason string
		}{
			{name: "runs", pro: true, want: quota.OutcomeOK},
			{name: "no pro", want: quota.OutcomeSkippedPlan},
			{name: "hidden", pro: true, want: runner.OutcomeSkippedStopped, stop: func(id, _, _ string) {
				if err := h.st.SetPublicAppHidden(h.t.Context(), id, true, "test"); err != nil {
					h.t.Fatal(err)
				}
			}},
			{name: "rule removed", pro: true, want: runner.OutcomeSkippedStopped, stop: func(_, app, owner string) {
				h.putDSL(owner, app, withoutRules(h.t, circleRules(h.t)))
			}},
			{name: "creator deleted", pro: true, want: runner.OutcomeSkippedStopped, stop: func(_, _, owner string) {
				if err := h.st.DeactivateUser(h.t.Context(), owner); err != nil {
					h.t.Fatal(err)
				}
			}},
			{name: "app deleted", pro: true, want: runner.OutcomeSkippedStopped, stop: func(_, app, owner string) {
				if code, out := h.do(owner, http.MethodDelete, app, "", nil); code/100 != 2 {
					h.t.Fatalf("delete: %d %v", code, out)
				}
			}},
		}
		for i, c := range cases {
			id := "RUN-STOP-" + strconv.Itoa(i) + h.sfx
			owner, mom := "uid-stop-owner"+strconv.Itoa(i)+h.sfx, "uid-stop-mom"+strconv.Itoa(i)+h.sfx
			h.pro[owner] = c.pro
			app := h.circle(id, owner, circleRules(t), mom)
			event := h.post(owner, app+"/entities/events/records", `{"data":{"title":"練習","status":"予定"}}`)
			h.post(mom, app+"/entities/rsvps/records", `{"data":{"event":"`+event+`","answer":"yes"}}`)
			if c.stop != nil {
				c.stop(id, app, owner)
			}
			h.tick()
			ev := h.oneEvent(id)
			if ev.Status != store.OutboxDone || ev.Outcome != c.want {
				t.Fatalf("%s: %+v", c.name, ev)
			}
			if c.name == "app deleted" {
				continue
			}
			got := num(h.record(store.PublicRecords(id, "events"), event).Data["yes_count"])
			if want := map[bool]float64{true: 1, false: 0}[c.want == quota.OutcomeOK]; got != want {
				t.Fatalf("%s: yes_count %v, want %v", c.name, got, want)
			}
		}
	})
}

// #337: a trigger of app X changes nothing in app Y, even with the same
// shape and a ref value naming Y's record.
func TestRunnerIsolation(t *testing.T) {
	stores(t, options{pro: fakePro{}}, func(t *testing.T, h *harness) {
		owner, mom := "uid-iso-owner"+h.sfx, "uid-iso-mom"+h.sfx
		h.pro[owner] = true
		x, y := "RUN-ISO-X"+h.sfx, "RUN-ISO-Y"+h.sfx
		appX := h.circle(x, owner, circleRules(t), mom)
		appY := h.circle(y, owner, circleRules(t), mom)
		evX := h.post(owner, appX+"/entities/events/records", `{"data":{"title":"X"}}`)
		evY := h.post(owner, appY+"/entities/events/records", `{"data":{"title":"Y"}}`)
		h.post(mom, appX+"/entities/rsvps/records", `{"data":{"event":"`+evX+`","answer":"yes"}}`)
		// a dangling ref in X that names Y's event
		code, _ := h.do(mom, http.MethodPost, appX+"/entities/rsvps/records", `{"data":{"event":"`+evY+`","answer":"yes"}}`, nil)
		h.tick()
		if c := num(h.record(store.PublicRecords(x, "events"), evX).Data["yes_count"]); c != 1 {
			t.Fatalf("X yes_count: %v", c)
		}
		if c := h.record(store.PublicRecords(y, "events"), evY).Data["yes_count"]; c != nil {
			t.Fatalf("Y changed by X's trigger (dangling post %d): %v", code, c)
		}
		if n := len(h.events(y)); n != 0 {
			t.Fatalf("Y events: %d", n)
		}
	})
}

// #339 / notify: in a shared app the creator gets a member's message,
// another member does not without consent (the default).
func TestRunnerNotifyConsent(t *testing.T) {
	stores(t, options{pro: fakePro{}, age: oldAccounts{}}, func(t *testing.T, h *harness) {
		owner, mom, dad := "uid-n-owner"+h.sfx, "uid-n-mom"+h.sfx, "uid-n-dad"+h.sfx
		h.pro[owner] = true
		h.tokens[owner], h.tokens[mom], h.tokens[dad] = []string{"tok-o"}, []string{"tok-m"}, []string{"tok-d"}
		id := "RUN-NOTIFY" + h.sfx
		app := h.circle(id, owner, circleRules(t), mom, dad)
		h.post(mom, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
		h.tick()
		var toks []string
		for _, p := range h.sender.Pushes() {
			toks = append(toks, p.Token)
		}
		if len(toks) != 1 || toks[0] != "tok-o" {
			t.Fatalf("pushes: %v", toks)
		}
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("event: %+v", ev)
		}
		if out := h.logs.String(); !strings.Contains(out, `"no_consent":1`) {
			t.Fatalf("no_consent not counted: %s", out)
		}
	})
}
