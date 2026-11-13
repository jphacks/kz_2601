package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

var errBoom2587 = errors.New("boom")

// failStore2587 keeps the memory store's Watcher and fails the calls it is told to.
type failStore2587 struct {
	*store.Memory
	deactivateErr error
	getAppErr     error
	ownerErr      error
	memberErr     error
	watchAppErr   error
	watchRecsErr  error
	closeVersions bool // WatchApp sends the version once and closes (listener failed)
	closeRecords  bool // WatchRecords closes at once (listener failed)
}

func (s *failStore2587) DeactivateUser(ctx context.Context, uid string) error {
	if s.deactivateErr != nil {
		return s.deactivateErr
	}
	return s.Memory.DeactivateUser(ctx, uid)
}

func (s *failStore2587) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.getAppErr != nil {
		return nil, s.getAppErr
	}
	return s.Memory.GetApp(ctx, uid, appID)
}

func (s *failStore2587) AppOwner(ctx context.Context, appID string) (string, error) {
	if s.ownerErr != nil {
		return "", s.ownerErr
	}
	return s.Memory.AppOwner(ctx, appID)
}

func (s *failStore2587) GetMember(ctx context.Context, appID, uid string) (*store.Member, error) {
	if s.memberErr != nil {
		return nil, s.memberErr
	}
	return s.Memory.GetMember(ctx, appID, uid)
}

func (s *failStore2587) WatchApp(ctx context.Context, uid, appID string) (<-chan int, error) {
	if s.watchAppErr != nil {
		return nil, s.watchAppErr
	}
	if s.closeVersions {
		a, err := s.Memory.GetApp(ctx, uid, appID)
		if err != nil {
			return nil, err
		}
		ch := make(chan int, 1)
		ch <- a.CurrentVersion
		close(ch)
		return ch, nil
	}
	return s.Memory.WatchApp(ctx, uid, appID)
}

func (s *failStore2587) WatchRecords(ctx context.Context, rs store.RecordSet) (<-chan struct{}, error) {
	if s.watchRecsErr != nil {
		return nil, s.watchRecsErr
	}
	if s.closeRecords {
		ch := make(chan struct{})
		close(ch)
		return ch, nil
	}
	return s.Memory.WatchRecords(ctx, rs)
}

// noWatch2587 hides the memory store's Watcher.
type noWatch2587 struct{ store.Store }

func api2587(s store.Store) http.Handler {
	return auth.Middleware(nil, auth.ModeDev)(New(s))
}

// serve2587 runs one request to the end (the stream ends at EVENTS_MAX_STREAM).
func serve2587(h http.Handler, ctx context.Context, uid, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil).WithContext(ctx)
	req.Header.Set(auth.DebugUIDHeader, uid)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func errCodeOf2587(rr *httptest.ResponseRecorder) string {
	r := resp{code: rr.Code, raw: rr.Body.Bytes()}
	_ = json.Unmarshal(r.raw, &r.body)
	return r.errCode()
}

func TestDeleteMeFailures2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	cases := []struct {
		name     string
		store    failStore2587
		ext      ExtRunner
		wantCode int
		wantErr  string
	}{
		{name: "deactivate fails", store: failStore2587{deactivateErr: errBoom2587}, wantCode: http.StatusInternalServerError, wantErr: httpx.CodeInternal},
		{name: "deactivate fails: runner not called", store: failStore2587{deactivateErr: errBoom2587}, ext: stubRunner{err: errBoom2587}, wantCode: http.StatusInternalServerError, wantErr: httpx.CodeInternal},
		{name: "revoke fails: still deleted", ext: stubRunner{err: errBoom2587}, wantCode: http.StatusNoContent},
		{name: "no runner", wantCode: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.store
			s.Memory = store.NewMemory()
			h := auth.Middleware(nil, auth.ModeDev)(New(&s, WithExt(tc.ext)))
			rr := serve2587(h, context.Background(), "leaver", http.MethodDelete, "/v1/me")
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantErr != "" {
				if got := errCodeOf2587(rr); got != tc.wantErr {
					t.Fatalf("code = %q, want %q", got, tc.wantErr)
				}
				if strings.Contains(rr.Body.String(), errBoom2587.Error()) {
					t.Fatalf("internal error leaked: %s", rr.Body)
				}
			}
		})
	}
}

func TestEventsFailures2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	const app = "/v1/apps/EV-2587/events"
	cases := []struct {
		name     string
		store    store.Store
		path     string
		wantCode int
		wantErr  string
	}{
		{name: "store without watch", store: noWatch2587{store.NewMemory()}, wantCode: http.StatusNotImplemented, wantErr: httpx.CodeInternal},
		{name: "reserved appId", store: store.NewMemory(), path: "/v1/apps/null/events", wantCode: http.StatusBadRequest, wantErr: httpx.CodeBadRequest},
		{name: "app read fails", store: &failStore2587{Memory: store.NewMemory(), getAppErr: errBoom2587}, wantCode: http.StatusInternalServerError, wantErr: httpx.CodeInternal},
		{name: "owner read fails", store: &failStore2587{Memory: store.NewMemory(), ownerErr: errBoom2587}, wantCode: http.StatusInternalServerError, wantErr: httpx.CodeInternal},
		{name: "watch app: missing", store: &failStore2587{Memory: store.NewMemory(), watchAppErr: store.ErrNotFound}, wantCode: http.StatusNotFound, wantErr: httpx.CodeNotFound},
		{name: "watch app fails", store: &failStore2587{Memory: store.NewMemory(), watchAppErr: errBoom2587}, wantCode: http.StatusInternalServerError, wantErr: httpx.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if path == "" {
				path = app
			}
			rr := serve2587(api2587(tc.store), context.Background(), "u1", http.MethodGet, path)
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if got := errCodeOf2587(rr); got != tc.wantErr {
				t.Fatalf("code = %q, want %q: %s", got, tc.wantErr, rr.Body)
			}
		})
	}
}

// ownedEnv2587 is an env whose u1 owns EV-2587 with kakeibo's expense entity.
func ownedEnv2587(t *testing.T, s store.Store) http.Handler {
	t.Helper()
	h := api2587(s)
	e := &env{t: t, h: h}
	e.ownApp("u1", "/v1/apps/EV-2587", kakeibo(t))
	return h
}

func TestEventsRecordWatchFails2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	s := &failStore2587{Memory: store.NewMemory()}
	h := ownedEnv2587(t, s)
	s.watchRecsErr = errBoom2587
	rr := serve2587(h, context.Background(), "u1", http.MethodGet, "/v1/apps/EV-2587/events")
	if rr.Code != http.StatusInternalServerError || errCodeOf2587(rr) != httpx.CodeInternal {
		t.Fatalf("got %d %s", rr.Code, rr.Body)
	}
}

func TestEventsMemberLookupFails2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	s := &failStore2587{Memory: store.NewMemory()}
	h := api2587(s)
	e := &env{t: t, h: h}
	e.ownApp("owner", "/v1/apps/FAM-2587", familyKakeibo(t))
	e.do("stranger", http.MethodGet, "/v1/apps/FAM-2587/dsl", nil, nil)
	s.memberErr = errBoom2587
	rr := serve2587(h, context.Background(), "stranger", http.MethodGet, "/v1/apps/FAM-2587/events")
	if rr.Code != http.StatusInternalServerError || errCodeOf2587(rr) != httpx.CodeInternal {
		t.Fatalf("got %d %s", rr.Code, rr.Body)
	}
}

// A non-participant's stream opens but carries no shared entity.
func TestEventsNonParticipant2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "200ms")
	h := api2587(store.NewMemory())
	e := &env{t: t, h: h}
	e.ownApp("owner", "/v1/apps/FAM-2587", familyKakeibo(t))
	e.do("stranger", http.MethodGet, "/v1/apps/FAM-2587/dsl", nil, nil)
	rr := serve2587(h, context.Background(), "stranger", http.MethodGet, "/v1/apps/FAM-2587/events")
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, `event: hello`) || !strings.Contains(body, `"entities":[]`) {
		t.Fatalf("stranger stream: %d %s", rr.Code, body)
	}
	if !strings.Contains(body, "event: bye") {
		t.Fatalf("planned end must say bye: %s", body)
	}
	// the owner hears the same app's shared entity
	rr = serve2587(h, context.Background(), "owner", http.MethodGet, "/v1/apps/FAM-2587/events")
	if !strings.Contains(rr.Body.String(), `"entities":["expense"]`) {
		t.Fatalf("owner stream: %s", rr.Body)
	}
}

// Streams that end without the planned "bye": the client polls until it
// reconnects.
func TestEventsEndWithoutBye2587(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		set  func(s *failStore2587)
	}{
		{name: "client cancelled", ctx: cancelled, set: func(*failStore2587) {}},
		{name: "app listener closed", ctx: context.Background(), set: func(s *failStore2587) { s.closeVersions = true }},
		{name: "records listener closed", ctx: context.Background(), set: func(s *failStore2587) { s.closeRecords = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &failStore2587{Memory: store.NewMemory()}
			h := ownedEnv2587(t, s)
			tc.set(s)
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- serve2587(h, tc.ctx, "u1", http.MethodGet, "/v1/apps/EV-2587/events") }()
			var rr *httptest.ResponseRecorder
			select {
			case rr = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("stream did not end within 5s")
			}
			body := rr.Body.String()
			if rr.Code != http.StatusOK || !strings.Contains(body, "event: hello") {
				t.Fatalf("stream: %d %s", rr.Code, body)
			}
			if strings.Contains(body, "event: bye") {
				t.Fatalf("a broken stream must not say bye: %s", body)
			}
		})
	}
}
