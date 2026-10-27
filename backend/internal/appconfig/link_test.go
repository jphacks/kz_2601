package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// linkedTokens stands in for Firebase after an anonymous account is linked to
// Sign in with Apple (#276): the ID token changes (sign_in_provider
// anonymous -> apple.com) but the uid stays the same.
type linkedTokens map[string]string

func (l linkedTokens) VerifyIDToken(_ context.Context, token string) (string, error) {
	if uid, ok := l[token]; ok {
		return uid, nil
	}
	return "", errors.New("invalid token")
}

// #276: apps and records made while anonymous stay with the account after the
// link, because appconfig keys everything by uid only (users/{uid}/apps/...).
func TestOwnerKeptAcrossAppleLink(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	tokens := linkedTokens{"anon-token": "u-linked", "apple-token": "u-linked", "other-token": "u-other"}
	h := auth.Middleware(tokens, auth.ModeFirebase)(New(store.NewMemory()))
	do := func(token, method, path, body string, hdr map[string]string) resp {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		out := resp{code: rr.Code, hdr: rr.Header(), raw: rr.Body.Bytes()}
		_ = json.Unmarshal(out.raw, &out.body)
		return out
	}
	const app = "/v1/apps/anon-made-app"

	// anonymous: create the app and a record
	do("anon-token", http.MethodGet, app+"/dsl", "", nil)
	if r := do("anon-token", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("anon put: %d %s", r.code, r.raw)
	}
	if r := do("anon-token", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1200,"category":"食費","date":"2026-10-24"}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("anon record: %d %s", r.code, r.raw)
	}

	// after the link (new token, same uid): same app, same version, same records
	r := do("apple-token", http.MethodGet, "/v1/apps", "", nil)
	if r.code != http.StatusOK || !strings.Contains(string(r.raw), `"anon-made-app"`) {
		t.Fatalf("apple list: %d %s", r.code, r.raw)
	}
	if r = do("apple-token", http.MethodGet, app+"/dsl", "", nil); r.code != http.StatusOK || r.version() != 2 {
		t.Fatalf("apple dsl: %d %s", r.code, r.raw)
	}
	if r = do("apple-token", http.MethodGet, app+"/entities/expense/records", "", nil); r.code != http.StatusOK || !strings.Contains(string(r.raw), `1200`) {
		t.Fatalf("apple records: %d %s", r.code, r.raw)
	}

	// another uid does not get it (its GET /dsl starts its own copy at version 1)
	if r = do("other-token", http.MethodGet, "/v1/apps", "", nil); strings.Contains(string(r.raw), `"anon-made-app"`) {
		t.Fatalf("other uid sees the app: %s", r.raw)
	}
	if r = do("other-token", http.MethodGet, app+"/entities/expense/records", "", nil); strings.Contains(string(r.raw), `1200`) {
		t.Fatalf("other uid sees the records: %d %s", r.code, r.raw)
	}
}
