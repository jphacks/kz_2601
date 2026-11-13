package appconfig

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// ownerErrStore fails AppOwner with err.
type ownerErrStore struct {
	store.Store
	err error
}

func (s ownerErrStore) AppOwner(context.Context, string) (string, error) { return "", s.err }

func TestCommitPublish_2601(t *testing.T) {
	shared, err := dsl.Parse(familyKakeibo(t))
	if err != nil {
		t.Fatal(err)
	}
	private, err := dsl.Parse(kakeibo(t))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/apps/PUB-1/dsl", nil)

	h := &Handler{store: store.NewMemory()}
	cp, err := h.commitPublish(req, "PUB-1", shared, true)
	if err != nil || cp == nil || !cp.Shared || !cp.Claim {
		t.Fatalf("shared+claim: %+v %v", cp, err)
	}
	cp, err = h.commitPublish(req, "PUB-1", private, false)
	if err != nil || cp == nil || cp.Shared || cp.Claim {
		t.Fatalf("private: %+v %v", cp, err)
	}

	boom := errors.New("firestore down")
	h = &Handler{store: ownerErrStore{Store: store.NewMemory(), err: boom}}
	if cp, err = h.commitPublish(req, "PUB-1", shared, true); !errors.Is(err, boom) || cp != nil {
		t.Fatalf("store error must fail the commit: %+v %v", cp, err)
	}
}

func TestCallTokenFailures_2601(t *testing.T) {
	e := callEnv(t, nil, 0, 100)
	const app = "/v1/apps/CALL-2601"
	e.ownApp("owner", app, withCallButton(t, familyKakeibo(t), map[string]any{"type": "callButton", "label": "話す"}))
	url := app + "/calls/token"

	if r := e.do("owner", http.MethodPost, url, `{"sessionId":"bad id!"}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("bad sessionId: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, url, `{not json`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("broken body: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, "/v1/apps/NO-SUCH-APP/calls/token", `{}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("unknown app: %d %s", r.code, r.raw)
	}
	// a non-member is refused and does not spend the rate limit
	wantNotMember(t, e.do("stranger", http.MethodPost, url, `{}`, nil), "stranger")
}

func TestOutboxFromEnv_2601(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		want outboxConfig
	}{
		{"unset is off", nil, outboxConfig{}},
		{"zero is off", map[string]string{"RUNNER_OUTBOX": "0"}, outboxConfig{}},
		{"garbage is off", map[string]string{"RUNNER_OUTBOX": "yes please"}, outboxConfig{}},
		{"on with no APP_ENV files under local", map[string]string{"RUNNER_OUTBOX": "1"}, outboxConfig{on: true, env: "local"}},
		{"TRUE with spaces and case", map[string]string{"RUNNER_OUTBOX": " True ", "APP_ENV": "Prod"}, outboxConfig{on: true, env: "prod"}},
		{"on", map[string]string{"RUNNER_OUTBOX": "on", "APP_ENV": "dev"}, outboxConfig{on: true, env: "dev"}},
		{"staging maps to stg", map[string]string{"RUNNER_OUTBOX": "1", "APP_ENV": "staging"}, outboxConfig{on: true, env: "stg"}},
		{"bad APP_ENV keeps it off", map[string]string{"RUNNER_OUTBOX": "1", "APP_ENV": "a/b"}, outboxConfig{}},
		{"too long APP_ENV keeps it off", map[string]string{"RUNNER_OUTBOX": "1", "APP_ENV": "abcdefghijklmnopqrstuvwxyz0123456789"}, outboxConfig{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := outboxFromEnv(get(tc.env)); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
