package appconfig

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// listRunner answers List with a fixed list / error; the other routes come
// from the embedded stubRunner.
type listRunner struct {
	stubRunner
	ans extapi.ConnectionList
	err error
}

func (l listRunner) List(context.Context, string) (extapi.ConnectionList, error) {
	return l.ans, l.err
}

func listEnv2604(t *testing.T, r ExtRunner) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithExt(r)))}
}

func TestListConnectionsFailures_2604(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"runner unreachable", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, http.StatusBadGateway, httpx.CodeUpstreamUnavailable},
		{"runner timeout", context.DeadlineExceeded, http.StatusGatewayTimeout, httpx.CodeUpstreamTimeout},
		{"runner 503 unavailable", &extapi.RunnerError{Status: 503, Reason: extapi.ReasonUnavailable}, http.StatusServiceUnavailable, httpx.CodeExtUnavailable},
		{"runner 502 upstream", &extapi.RunnerError{Status: 502, Reason: extapi.ReasonUpstream}, http.StatusBadGateway, httpx.CodeUpstreamUnavailable},
		{"runner 500 internal", &extapi.RunnerError{Status: 500, Reason: extapi.ReasonInternal}, http.StatusInternalServerError, httpx.CodeInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := listEnv2604(t, listRunner{err: c.err})
			wantExtErr(t, e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil), c.status, c.code, c.name)
		})
	}
}

func TestListConnectionsShapes_2604(t *testing.T) {
	// no connections and a nil slice: still a JSON array, not null
	e := listEnv2604(t, listRunner{ans: extapi.ConnectionList{Enabled: []string{"todoist"}}})
	r := e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil)
	list, ok := r.body["connections"].([]any)
	if r.code != http.StatusOK || !ok || len(list) != 0 {
		t.Fatalf("zero connections: %d %s", r.code, r.raw)
	}
	got := providerIDs(t, r)
	found := false
	for _, p := range got {
		switch p {
		case "todoist:true":
			found = true
		case "slack:true", "github:true":
			t.Fatalf("a provider the runner did not enable is on: %s", p)
		}
	}
	if !found {
		t.Fatalf("enabled provider missing: %v", got)
	}

	// a provider that is off by default is listed as enabled=false; the fake is not listed
	for _, p := range got {
		if p == "fakeOauth:false" || p == "fakeOauth:true" {
			t.Fatalf("fake listed while off: %v", got)
		}
	}
}
