package appconfig

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// faultStore2576 fails ListApps / ReorderApps on demand.
type faultStore2576 struct {
	store.Store
	list, reorder error
}

func (s *faultStore2576) ListApps(ctx context.Context, uid string, limit, offset int) ([]store.App, error) {
	if s.list != nil {
		return nil, s.list
	}
	return s.Store.ListApps(ctx, uid, limit, offset)
}

func (s *faultStore2576) ReorderApps(ctx context.Context, uid string, ids []string) error {
	if s.reorder != nil {
		return s.reorder
	}
	return s.Store.ReorderApps(ctx, uid, ids)
}

func newFaultEnv2576(t *testing.T) (*env, *faultStore2576) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &faultStore2576{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func TestListAppsFailures2576(t *testing.T) {
	e, st := newFaultEnv2576(t)
	const uid = "lister2576"
	a, b := "LIST-A-2576", "LIST-B-2576"
	e.ownApp(uid, "/v1/apps/"+a, kakeibo(t))
	e.ownApp(uid, "/v1/apps/"+b, kakeibo(t))

	token := func(n int) string { return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprint(n))) }
	for _, q := range []string{
		"?pageSize=0",
		"?pageSize=x",
		"?pageToken=%21%21",
		"?pageToken=" + base64.RawURLEncoding.EncodeToString([]byte("x")),
		"?pageToken=" + token(-1),
	} {
		if r := e.do(uid, http.MethodGet, "/v1/apps"+q, nil, nil); r.code != http.StatusBadRequest || r.errCode() != "bad_request" {
			t.Fatalf("%s: got %d %s", q, r.code, r.raw)
		}
	}

	// One per page: the first page is cut at pageSize and points to the next.
	r := e.do(uid, http.MethodGet, "/v1/apps?pageSize=1", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 1 || r.body["nextPageToken"] != token(1) {
		t.Fatalf("first page: %d %s", r.code, r.raw)
	}
	r = e.do(uid, http.MethodGet, "/v1/apps?pageSize=1&pageToken="+token(1), nil, nil)
	if items, _ = r.body["items"].([]any); r.code != http.StatusOK || len(items) != 1 || r.body["nextPageToken"] != nil {
		t.Fatalf("last page: %d %s", r.code, r.raw)
	}
	r = e.do(uid, http.MethodGet, "/v1/apps?pageToken="+token(9), nil, nil)
	if items, _ = r.body["items"].([]any); r.code != http.StatusOK || len(items) != 0 {
		t.Fatalf("past the end: %d %s", r.code, r.raw)
	}

	// A deleted app leaves the list.
	if r := e.do(uid, http.MethodDelete, "/v1/apps/"+a, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	got := appItems(e, uid)
	if _, ok := got[a]; ok || len(got) != 1 || got[b] == nil {
		t.Fatalf("after delete: %v", got)
	}

	// A user with no apps gets an empty list, not null.
	r = e.do("nobody2576", http.MethodGet, "/v1/apps", nil, nil)
	if items, ok := r.body["items"].([]any); r.code != http.StatusOK || !ok || len(items) != 0 {
		t.Fatalf("no apps: %d %s", r.code, r.raw)
	}

	st.list = errors.New("injected")
	if r := e.do(uid, http.MethodGet, "/v1/apps", nil, nil); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("store error: %d %s", r.code, r.raw)
	}
}

func TestReorderAppsFailures2576(t *testing.T) {
	e, st := newFaultEnv2576(t)
	const uid = "sorter2576"
	a, b := "ORD-A-2576", "ORD-B-2576"
	e.ownApp(uid, "/v1/apps/"+a, kakeibo(t))
	e.ownApp(uid, "/v1/apps/"+b, kakeibo(t))
	e.ownApp("other2576", "/v1/apps/ORD-X-2576", kakeibo(t))
	if r := e.do(uid, http.MethodPut, "/v1/apps/order", map[string]any{"ids": []string{b, a}}, nil); r.code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", r.code, r.raw)
	}
	sortOf := func(uid, id string) any {
		t.Helper()
		return appItems(e, uid)[id]["sortIndex"]
	}

	tooMany := make([]string, maxReorderIDs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("APP-%d", i)
	}
	for name, body := range map[string]any{
		"same id twice": map[string]any{"ids": []string{a, b, a}},
		"invalid id":    map[string]any{"ids": []string{a, "bad id"}},
		"reserved id":   map[string]any{"ids": []string{"null"}},
		"too many ids":  map[string]any{"ids": tooMany},
		"not json":      "{",
		"ids not array": `{"ids":"x"}`,
	} {
		if r := e.do(uid, http.MethodPut, "/v1/apps/order", body, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d %s", name, r.code, r.raw)
		}
	}
	if sortOf(uid, b) != float64(1) || sortOf(uid, a) != float64(2) {
		t.Fatalf("rejected requests changed the order: %v", appItems(e, uid))
	}

	// Unknown ids, another user's app and an empty list are accepted and change nothing.
	for name, ids := range map[string][]string{
		"unknown id":    {"MISSING-2576"},
		"someone elses": {"ORD-X-2576"},
		"empty":         {},
	} {
		if r := e.do(uid, http.MethodPut, "/v1/apps/order", map[string]any{"ids": ids}, nil); r.code != http.StatusNoContent {
			t.Fatalf("%s: want 204, got %d %s", name, r.code, r.raw)
		}
	}
	if sortOf(uid, b) != float64(1) || sortOf(uid, a) != float64(2) || sortOf("other2576", "ORD-X-2576") != nil {
		t.Fatalf("no-op reorders changed something: %v %v", appItems(e, uid), appItems(e, "other2576"))
	}
	if _, ok := appItems(e, uid)["ORD-X-2576"]; ok {
		t.Fatal("another user's app joined my list")
	}

	st.reorder = errors.New("injected")
	if r := e.do(uid, http.MethodPut, "/v1/apps/order", map[string]any{"ids": []string{a}}, nil); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("store error: %d %s", r.code, r.raw)
	}
}
