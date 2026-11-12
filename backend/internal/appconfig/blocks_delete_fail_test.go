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

// blockDeleteDownStore fails DeleteBlock on demand.
type blockDeleteDownStore struct {
	store.Store
	fail bool
}

var errBlockDeleteDown = errors.New("injected block delete failure")

func (s *blockDeleteDownStore) DeleteBlock(ctx context.Context, uid, blockedUID string) error {
	if s.fail {
		return errBlockDeleteDown
	}
	return s.Store.DeleteBlock(ctx, uid, blockedUID)
}

// #2571: the failure paths of DELETE /v1/me/blocks/{uid}.
func TestDeleteBlockFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &blockDeleteDownStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	ctx := context.Background()
	blocked := func(uid string) string {
		t.Helper()
		bs, err := mem.ListBlocks(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, b := range bs {
			out = append(out, b.UID)
		}
		return strings.Join(out, ",")
	}
	if r := e.do("blk-a", http.MethodPost, "/v1/me/blocks", `{"uid":"blk-b"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("block: %d %s", r.code, r.raw)
	}

	for _, c := range []struct {
		name, uid, path string
		code            int
		errCode         string
	}{
		{"no auth", "", "/v1/me/blocks/blk-b", http.StatusUnauthorized, "unauthenticated"},
		{"invalid uid", "blk-a", "/v1/me/blocks/bad%21uid", http.StatusBadRequest, "bad_request"},
		// not blocked and someone else's block: idempotent 204, nothing removed
		{"not blocked", "blk-a", "/v1/me/blocks/blk-none", http.StatusNoContent, ""},
		{"someone else's block", "blk-c", "/v1/me/blocks/blk-b", http.StatusNoContent, ""},
	} {
		if r := e.do(c.uid, http.MethodDelete, c.path, nil, nil); r.code != c.code || r.errCode() != c.errCode {
			t.Errorf("%s: %d %s", c.name, r.code, r.raw)
		}
	}
	if got := blocked("blk-a"); got != "blk-b" {
		t.Fatalf("a's block changed by a rejected delete: %q", got)
	}

	// a store error is a 500 that hides the cause and keeps the block
	st.fail = true
	r := e.do("blk-a", http.MethodDelete, "/v1/me/blocks/blk-b", nil, nil)
	st.fail = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errBlockDeleteDown.Error()) {
		t.Errorf("store error: %d %s", r.code, r.raw)
	}
	if got := blocked("blk-a"); got != "blk-b" {
		t.Fatalf("block removed on a failed delete: %q", got)
	}

	// unblocking twice gives the same answer
	for i := range 2 {
		if r := e.do("blk-a", http.MethodDelete, "/v1/me/blocks/blk-b", nil, nil); r.code != http.StatusNoContent || len(r.raw) != 0 {
			t.Errorf("unblock #%d: %d %s", i+1, r.code, r.raw)
		}
	}
	if got := blocked("blk-a"); got != "" {
		t.Fatalf("left: %q", got)
	}
}
