package appconfig

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/moderation/textfilter"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// TEXT_FILTER: enforce refuses, log saves and logs the category only, off
// neither refuses nor logs.
func TestTextFilterModes(t *testing.T) {
	for _, tc := range []struct {
		mode    textfilter.Mode
		code    int
		logLine string
	}{
		{"", http.StatusUnprocessableEntity, "moderation: text refused"},
		{textfilter.ModeEnforce, http.StatusUnprocessableEntity, "moderation: text refused"},
		{textfilter.ModeLog, http.StatusCreated, "moderation: text would be refused"},
		{textfilter.ModeOff, http.StatusCreated, ""},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			var logs bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
			e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithTextFilter(tc.mode)))}
			const app = "/v1/apps/TEXT-MODE-1"
			e.ownApp("u1", app, sharedKakeibo(t))
			r := e.do("u1", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1,"memo":"死ね"}}`, nil)
			if tc.code == http.StatusUnprocessableEntity {
				wantTextNotAllowed(t, r, "memo", "memo")
			} else if r.code != tc.code {
				t.Fatalf("want %d, got %d %s", tc.code, r.code, r.raw)
			}

			out := logs.String()
			if strings.Contains(out, "死ね") {
				t.Errorf("log has the text: %s", out)
			}
			if tc.logLine == "" {
				if strings.Contains(out, "moderation: text") {
					t.Errorf("off: want no moderation log, got %s", out)
				}
				return
			}
			if !strings.Contains(out, tc.logLine) || !strings.Contains(out, "category=threat") {
				t.Errorf("want %q with category=threat, got %s", tc.logLine, out)
			}
		})
	}
}

func wantTextNotAllowed(t *testing.T, r resp, field, what string) {
	t.Helper()
	er, _ := r.body["error"].(map[string]any)
	details, _ := er["details"].(map[string]any)
	if r.code != http.StatusUnprocessableEntity || er["code"] != httpx.CodeTextNotAllowed || er["message"] != textNotAllowedMsg || details["field"] != field {
		t.Fatalf("%s: want 422 %s field=%s, got %d %s", what, httpx.CodeTextNotAllowed, field, r.code, r.raw)
	}
}

// #321: text other people see is filtered; the creator's private data is not.
func TestTextFilterSharedRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SHARED-TEXT-1"
	recs := app + "/entities/expense/records"
	e.ownApp("u1", app, sharedKakeibo(t))

	wantTextNotAllowed(t, e.do("u1", http.MethodPost, recs, `{"data":{"amount":1,"memo":"死ね"}}`, nil), "memo", "post memo")
	wantTextNotAllowed(t, e.do("u1", http.MethodPost, recs, `{"data":{"amount":1},"authorName":"キチガイ"}`, nil), "authorName", "post authorName")

	r := e.do("u1", http.MethodPost, recs, `{"data":{"amount":1,"memo":"パチンコ代"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("ordinary memo: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	wantTextNotAllowed(t, e.do("u1", http.MethodPatch, recs+"/"+id, `{"data":{"memo":"kill yourself"}}`, nil), "memo", "patch memo")
	if r = e.do("u1", http.MethodPatch, recs+"/"+id, `{"data":{"memo":"ランチ"}}`, nil); r.code != http.StatusOK {
		t.Fatalf("ordinary patch: %d %s", r.code, r.raw)
	}
}

func TestTextFilterSkipsPrivateRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/PRIVATE-TEXT-1"
	e.ownApp("u1", app, kakeibo(t))
	if r := e.do("u1", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1,"memo":"死ね"}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("private memo: %d %s", r.code, r.raw)
	}
}

func TestTextFilterListingAndMemberName(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/LISTING-TEXT-1"
	e.ownApp("owner", app, walk(t))
	wantTextNotAllowed(t, e.do("owner", http.MethodPost, app+"/publish", `{"name":"porn"}`, nil), "name", "listing name")
	wantTextNotAllowed(t, e.do("owner", http.MethodPost, app+"/publish", `{"description":"殺してやる"}`, nil), "description", "listing description")
	wantTextNotAllowed(t, e.do("owner", http.MethodPost, app+"/publish", `{"tags":["ok","fuck"]}`, nil), "tags", "listing tag")
	wantTextNotAllowed(t, e.do("owner", http.MethodPost, app+"/publish", `{"authorName":"kys"}`, nil), "authorName", "listing authorName")
	if r := e.do("owner", http.MethodPost, app+"/publish", `{"description":"毎日の散歩を記録"}`, nil); r.code != http.StatusOK {
		t.Fatalf("ordinary listing: %d %s", r.code, r.raw)
	}

	const shared = "/v1/apps/MEMBER-TEXT-1"
	e.ownApp("owner", shared, familyKakeibo(t))
	tok := e.share("owner", shared)
	wantTextNotAllowed(t, e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"死ね"}`, nil), "displayName", "member name")
	if r := e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"ともだち"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("ordinary member name: %d %s", r.code, r.raw)
	}
}

// #321 × #272/#316: a write the caller may not make is refused as such (403)
// before the text filter, so a 422 does not hide it; the author is still filtered.
func TestTextFilterAfterPermissionChecks(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SHIFT-TEXT-1"
	recs := app + "/entities/shift/records"
	joinShift(t, e, app)

	wantAdminsOnly(t, e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9,"confirmed":true,"memo":"死ね"}}`, nil), "create admin-only field")
	wantNotMember(t, e.do("stranger", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9,"memo":"死ね"}}`, nil), "stranger create")
	r := e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9,"memo":"朝"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("staff create: %d %s", r.code, r.raw)
	}
	one := recs + "/" + r.body["id"].(string)

	if r = e.do("boss", http.MethodPatch, one, `{"data":{"memo":"死ね"}}`, nil); r.code != http.StatusForbidden || r.errCode() != "forbidden" {
		t.Fatalf("non-author patch: want 403 forbidden, got %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("stranger", http.MethodPatch, one, `{"data":{"memo":"死ね"}}`, nil), "stranger patch")
	wantAdminsOnly(t, e.do("staff", http.MethodPatch, one, `{"data":{"confirmed":true,"memo":"死ね"}}`, nil), "author patches admin-only field")
	wantTextNotAllowed(t, e.do("staff", http.MethodPatch, one, `{"data":{"memo":"死ね"}}`, nil), "memo", "author patch")
}
