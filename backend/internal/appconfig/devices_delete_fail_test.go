package appconfig

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// deviceDeleteDownStore fails DeleteDevice on demand.
type deviceDeleteDownStore struct {
	store.Store
	fail bool
}

var errDeviceDeleteDown = errors.New("injected device delete failure")

func (s *deviceDeleteDownStore) DeleteDevice(ctx context.Context, uid, installationID string) (bool, error) {
	if s.fail {
		return false, errDeviceDeleteDown
	}
	return s.Store.DeleteDevice(ctx, uid, installationID)
}

// #2571: the failure paths of DELETE /v1/me/devices/{installationId}.
func TestDeleteDeviceFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &deviceDeleteDownStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const path = "/v1/me/devices/inst-delfail-01"
	ctx := context.Background()
	count := func(uid string) int {
		t.Helper()
		ds, err := mem.ListDevices(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		return len(ds)
	}
	if r := e.do("dev-owner", http.MethodPut, path, deviceBody(901), nil); r.code != http.StatusOK {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	for _, c := range []struct {
		name, uid, path string
		code            int
		errCode         string
	}{
		{"no auth", "", path, http.StatusUnauthorized, "unauthenticated"},
		{"too short id", "dev-owner", "/v1/me/devices/short", http.StatusBadRequest, "bad_request"},
		{"bad characters", "dev-owner", "/v1/me/devices/inst.delfail.01", http.StatusBadRequest, "bad_request"},
		{"too long id", "dev-owner", "/v1/me/devices/" + strings.Repeat("a", 65), http.StatusBadRequest, "bad_request"},
		// unknown and someone else's device: idempotent 204, nothing removed
		{"unknown device", "dev-owner", "/v1/me/devices/inst-delfail-none", http.StatusNoContent, ""},
		{"someone else's device", "dev-other", path, http.StatusNoContent, ""},
	} {
		if r := e.do(c.uid, http.MethodDelete, c.path, nil, nil); r.code != c.code || r.errCode() != c.errCode {
			t.Errorf("%s: %d %s", c.name, r.code, r.raw)
		}
	}
	if n := count("dev-owner"); n != 1 {
		t.Fatalf("owner's device removed by a rejected delete: %d", n)
	}

	// a store error is a 500 that hides the cause and keeps the device
	st.fail = true
	r := e.do("dev-owner", http.MethodDelete, path, nil, nil)
	st.fail = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errDeviceDeleteDown.Error()) {
		t.Errorf("store error: %d %s", r.code, r.raw)
	}
	if n := count("dev-owner"); n != 1 {
		t.Fatalf("device removed on a failed delete: %d", n)
	}

	// deleting twice gives the same answer
	for i := range 2 {
		if r := e.do("dev-owner", http.MethodDelete, path, nil, nil); r.code != http.StatusNoContent || len(r.raw) != 0 {
			t.Errorf("delete #%d: %d %s", i+1, r.code, r.raw)
		}
	}
	if n := count("dev-owner"); n != 0 {
		t.Fatalf("left: %d", n)
	}
}
