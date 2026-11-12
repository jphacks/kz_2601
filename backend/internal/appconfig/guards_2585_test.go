package appconfig

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

var errBoom2585 = errors.New("boom")

// guardStore2585 fails or overrides the calls the guards make.
type guardStore2585 struct {
	store.Store
	stopsErr error
	ownerErr error
	claimErr error
	claimAs  string // ClaimApp answers this owner when set (another caller won the race)
}

func (s *guardStore2585) ActiveStops(ctx context.Context, env string, ids []string, now time.Time) (map[string]store.StopState, error) {
	if s.stopsErr != nil {
		return nil, s.stopsErr
	}
	return s.Store.ActiveStops(ctx, env, ids, now)
}

func (s *guardStore2585) AppOwner(ctx context.Context, appID string) (string, error) {
	if s.ownerErr != nil {
		return "", s.ownerErr
	}
	return s.Store.AppOwner(ctx, appID)
}

func (s *guardStore2585) ClaimApp(ctx context.Context, uid, appID string) (string, error) {
	if s.claimErr != nil {
		return "", s.claimErr
	}
	if s.claimAs != "" {
		return s.claimAs, nil
	}
	return s.Store.ClaimApp(ctx, uid, appID)
}

func req2585(uid string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	return r.WithContext(auth.ContextWithUID(r.Context(), uid))
}

func apiErrOf2585(t *testing.T, err error) *apiError {
	t.Helper()
	var ae *apiError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *apiError", err)
	}
	return ae
}

func TestCheckAppStopTable2585(t *testing.T) {
	const env, app = "prod", "a1"
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	cases := []struct {
		name      string
		stops     []store.Stop
		stopsErr  error
		wantErr   error
		wantCode  string
		wantUntil string
	}{
		{name: "no stop", wantCode: ""},
		{name: "app stop without force", stops: []store.Stop{{ID: store.AppStopID(app), Level: store.StopApp, AppID: app}}},
		{name: "other app forced", stops: []store.Stop{{ID: store.AppStopID("other"), Level: store.StopApp, AppID: "other", Force: true}}},
		{name: "lifted forced stop", stops: []store.Stop{{ID: store.AppStopID(app), Level: store.StopApp, AppID: app, Force: true, LiftedAt: time.Now().Add(-time.Minute)}}},
		{name: "app forced until lifted", stops: []store.Stop{{ID: store.AppStopID(app), Level: store.StopApp, AppID: app, Force: true}}, wantCode: httpx.CodeAppStopped},
		{name: "app forced with until", stops: []store.Stop{{ID: store.AppStopID(app), Level: store.StopApp, AppID: app, Force: true, Until: until}}, wantCode: httpx.CodeAppStopped, wantUntil: until.Format(time.RFC3339)},
		{name: "global forced", stops: []store.Stop{{ID: store.GlobalStopID, Level: store.StopApp, Force: true}}, wantCode: httpx.CodeAppStopped},
		{name: "store error", stopsErr: errBoom2585, wantErr: errBoom2585},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := store.NewMemory()
			for _, s := range tc.stops {
				if err := mem.PutStop(context.Background(), env, s); err != nil {
					t.Fatal(err)
				}
			}
			h := &Handler{store: &guardStore2585{Store: mem, stopsErr: tc.stopsErr}}
			err := h.checkAppStop(req2585("u1"), env, app)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			case tc.wantCode == "":
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			ae := apiErrOf2585(t, err)
			if ae.status != http.StatusForbidden || ae.code != tc.wantCode {
				t.Fatalf("got %d %s, want 403 %s", ae.status, ae.code, tc.wantCode)
			}
			if ae.details["level"] != store.StopApp {
				t.Errorf("level = %v", ae.details["level"])
			}
			got, has := ae.details["until"]
			if tc.wantUntil == "" && has {
				t.Errorf("until = %v, want none", got)
			}
			if tc.wantUntil != "" && got != tc.wantUntil {
				t.Errorf("until = %v, want %s", got, tc.wantUntil)
			}
		})
	}
}

func TestCheckAppStopOtherEnv2585(t *testing.T) {
	mem := store.NewMemory()
	if err := mem.PutStop(context.Background(), "staging", store.Stop{ID: store.GlobalStopID, Level: store.StopApp, Force: true}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: mem}
	if err := h.checkAppStop(req2585("u1"), "prod", "a1"); err != nil {
		t.Fatalf("a stop in another env must not apply: %v", err)
	}
}

func TestRequireCreatorTable2585(t *testing.T) {
	const app = "a1"
	full := &dsl.App{Entities: []dsl.Entity{{ID: "e"}}}
	blank := &dsl.App{}
	cases := []struct {
		name      string
		owner     string // pre-claimed creator, "" = none
		app       *dsl.App
		store     guardStore2585
		wantErr   error
		wantCode  string
		wantOwner string // creator after the call, "" = unchecked
	}{
		{name: "creator with content", owner: "u1", app: full, wantOwner: "u1"},
		{name: "creator blank copy", owner: "u1", app: blank, wantCode: httpx.CodeBadRequest},
		{name: "not the creator", owner: "u2", app: full, wantCode: httpx.CodeForbidden, wantOwner: "u2"},
		{name: "not the creator blank copy", owner: "u2", app: blank, wantCode: httpx.CodeForbidden},
		{name: "unclaimed is claimed", app: full, wantOwner: "u1"},
		{name: "unclaimed blank is not claimed", app: blank, wantCode: httpx.CodeBadRequest},
		{name: "claim lost to another", app: full, store: guardStore2585{claimAs: "u2"}, wantCode: httpx.CodeForbidden},
		{name: "claim error", app: full, store: guardStore2585{claimErr: errBoom2585}, wantErr: errBoom2585},
		{name: "owner lookup error", owner: "u1", app: full, store: guardStore2585{ownerErr: errBoom2585}, wantErr: errBoom2585},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := store.NewMemory()
			if tc.owner != "" {
				if _, err := mem.ClaimApp(context.Background(), tc.owner, app); err != nil {
					t.Fatal(err)
				}
			}
			s := tc.store
			s.Store = mem
			h := &Handler{store: &s}
			err := h.requireCreator(req2585("u1"), app, tc.app)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case tc.wantCode == "":
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			default:
				ae := apiErrOf2585(t, err)
				if ae.code != tc.wantCode {
					t.Fatalf("code = %s, want %s", ae.code, tc.wantCode)
				}
				want := http.StatusForbidden
				if tc.wantCode == httpx.CodeBadRequest {
					want = http.StatusBadRequest
				}
				if ae.status != want {
					t.Fatalf("status = %d, want %d", ae.status, want)
				}
			}
			o, err := mem.AppOwner(context.Background(), app)
			if tc.owner == "" && tc.wantOwner == "" && tc.store == (guardStore2585{}) {
				if !errors.Is(err, store.ErrNotFound) {
					t.Errorf("owner = %q (%v), want unclaimed", o, err)
				}
			}
			if tc.wantOwner != "" && o != tc.wantOwner {
				t.Errorf("owner = %q, want %q", o, tc.wantOwner)
			}
		})
	}
}

func TestCheckRecordSizeTable2585(t *testing.T) {
	// {"k":"…"} is 8 bytes around the value.
	pad := func(n int) map[string]any { return map[string]any{"k": strings.Repeat("a", n-8)} }
	cases := []struct {
		name     string
		data     map[string]any
		wantCode string
		wantErr  bool
	}{
		{name: "nil", data: nil},
		{name: "empty", data: map[string]any{}},
		{name: "exactly the limit", data: pad(maxRecordBody)},
		{name: "one byte over", data: pad(maxRecordBody + 1), wantCode: httpx.CodePayloadTooLarge},
		{name: "unencodable", data: map[string]any{"c": make(chan int)}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRecordSize(tc.data)
			switch {
			case tc.wantErr:
				var ae *apiError
				if err == nil || errors.As(err, &ae) {
					t.Fatalf("err = %v, want a plain encode error", err)
				}
			case tc.wantCode == "":
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			default:
				ae := apiErrOf2585(t, err)
				if ae.status != http.StatusRequestEntityTooLarge || ae.code != tc.wantCode {
					t.Fatalf("got %d %s", ae.status, ae.code)
				}
			}
		})
	}
}
