package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// entitiesFaultStore fails GetApp, or hands back a stored DSL that no longer
// parses, for one appId.
type entitiesFaultStore struct {
	store.Store
	failGetApp bool
	brokenApp  string
}

var errEntitiesDown = errors.New("injected entities failure")

func (s *entitiesFaultStore) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.failGetApp {
		return nil, errEntitiesDown
	}
	a, err := s.Store.GetApp(ctx, uid, appID)
	if err == nil && appID == s.brokenApp {
		cp := *a
		cp.DSL = json.RawMessage(`{"version":"1","app":{"name":"x"},"entities":[{"id":`)
		return &cp, nil
	}
	return a, err
}

func entityIDs(r resp) string {
	items, _ := r.body["items"].([]any)
	var ids []string
	for _, it := range items {
		m, _ := it.(map[string]any)
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	return strings.Join(ids, ",")
}

// #2568: the failure paths of GET /v1/apps/{appId}/entities.
func TestListEntitiesFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &entitiesFaultStore{Store: store.NewMemory()}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const app = "/v1/apps/ENTITIES-FAIL-1"
	e.ownApp("owner", app, kakeibo(t))

	if r := e.do("owner", http.MethodGet, app+"/entities", nil, nil); r.code != http.StatusOK || r.version() != 2 || !strings.Contains(entityIDs(r), "expense") {
		t.Fatalf("owner: %d %s", r.code, r.raw)
	}

	// invalid appId
	if r := e.do("owner", http.MethodGet, "/v1/apps/null/entities", nil, nil); r.code != http.StatusBadRequest || r.errCode() != "bad_request" {
		t.Errorf("invalid appId: %d %s", r.code, r.raw)
	}

	// an unknown appId is seeded (version 1, no entities) instead of 404
	if r := e.do("owner", http.MethodGet, "/v1/apps/ENTITIES-NONE/entities", nil, nil); r.code != http.StatusOK || r.version() != 1 || entityIDs(r) != "" {
		t.Errorf("unknown app: %d %s", r.code, r.raw)
	}

	// someone else's private app: they only see their own blank seed, never
	// the owner's entities
	if r := e.do("other", http.MethodGet, app+"/entities", nil, nil); r.code != http.StatusOK || r.version() != 1 || entityIDs(r) != "" {
		t.Errorf("not the owner: %d %s", r.code, r.raw)
	}

	// a stored DSL that no longer parses is a 500, not a partial list
	const broken = "/v1/apps/ENTITIES-BROKEN"
	e.ownApp("owner", broken, kakeibo(t))
	st.brokenApp = "ENTITIES-BROKEN"
	if r := e.do("owner", http.MethodGet, broken+"/entities", nil, nil); r.code != http.StatusInternalServerError || r.errCode() != "internal" || r.body["items"] != nil {
		t.Errorf("broken dsl: %d %s", r.code, r.raw)
	}
	st.brokenApp = ""

	// a store error is a 500 that does not leak the cause
	st.failGetApp = true
	r := e.do("owner", http.MethodGet, app+"/entities", nil, nil)
	st.failGetApp = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errEntitiesDown.Error()) {
		t.Errorf("store error: %d %s", r.code, r.raw)
	}

	if r := e.do("owner", http.MethodGet, app+"/entities", nil, nil); r.code != http.StatusOK || !strings.Contains(entityIDs(r), "expense") {
		t.Fatalf("recovered: %d %s", r.code, r.raw)
	}
}
