package appconfig

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func putLoopNotice(t *testing.T, ls loop.Store, uid, draft string, at time.Time) loop.Notice {
	t.Helper()
	n := loop.Notice{ID: loop.NoticeID(draft, uid), UID: uid, DraftID: draft, Title: "直りました: " + draft,
		Body: "あなたの声で直りました", CreatedAt: at, ExpiresAt: at.Add(loop.Retention)}
	if _, err := ls.PutNotice(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n
}

func noticeIDs(r resp) []string {
	var out []string
	items, _ := r.body["items"].([]any)
	for _, it := range items {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

// #374: GET /v1/me/notices is the caller's own, newest first, paged; read
// marks only the caller's own (someone else's id is 404 like a missing one).
func TestLoopNoticesAPI(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		// Firestore: the handler reads loop_notices next to the store (the default)
		ls, ok := defaultNotices(st).(loop.Store)
		if !ok {
			t.Fatal("default notices is not a loop.Store")
		}
		if _, isMem := st.(*store.Memory); isMem {
			// memory: the handler's own loop store is not reachable; give it one we seed
			m := loop.NewMemory()
			ls = m
			e = &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st, WithLoopNotices(m)))}
		}
		a, b := "ntc-a"+sfx, "ntc-b"+sfx
		base := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
		var ids []string
		for i := range 3 {
			ids = append(ids, putLoopNotice(t, ls, a, "drf_"+strconv.Itoa(i)+sfx, base.Add(time.Duration(i)*time.Minute)).ID)
		}
		bn := putLoopNotice(t, ls, b, "drf_b"+sfx, base)
		// expired, waiting for purge: not shown
		old := loop.Notice{ID: loop.NoticeID("drf_old"+sfx, a), UID: a, Title: "old", CreatedAt: base.Add(-100 * 24 * time.Hour), ExpiresAt: base.Add(-time.Hour)}
		if _, err := ls.PutNotice(context.Background(), old); err != nil {
			t.Fatal(err)
		}

		r := e.do(a, http.MethodGet, "/v1/me/notices?pageSize=2", nil, nil)
		if r.code != http.StatusOK {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}
		if got := noticeIDs(r); len(got) != 2 || got[0] != ids[2] || got[1] != ids[1] {
			t.Fatalf("page 1 (newest first): %v want %v", got, ids)
		}
		if r.body["unreadCount"] != float64(3) {
			t.Fatalf("unreadCount: %v", r.body["unreadCount"])
		}
		first := r.body["items"].([]any)[0].(map[string]any)
		if first["title"] == "" || first["body"] != "あなたの声で直りました" || first["createdAt"] == nil || first["readAt"] != nil {
			t.Fatalf("item: %v", first)
		}
		if _, leaked := first["uid"]; leaked {
			t.Fatalf("uid in response: %v", first)
		}
		tok, _ := r.body["nextPageToken"].(string)
		r = e.do(a, http.MethodGet, "/v1/me/notices?pageSize=2&pageToken="+tok, nil, nil)
		if got := noticeIDs(r); len(got) != 1 || got[0] != ids[0] || r.body["nextPageToken"] != nil {
			t.Fatalf("page 2: %v %s", got, r.raw)
		}

		// b sees only b's; b cannot mark a's (404, same as missing)
		if got := noticeIDs(e.do(b, http.MethodGet, "/v1/me/notices", nil, nil)); len(got) != 1 || got[0] != bn.ID {
			t.Fatalf("b's list: %v", got)
		}
		if r := e.do(b, http.MethodPost, "/v1/me/notices/"+ids[0]+"/read", nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("b marked a's: %d %s", r.code, r.raw)
		}
		if r := e.do(a, http.MethodPost, "/v1/me/notices/ntc_000000000000000000000000/read", nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("missing: %d", r.code)
		}
		if r := e.do(a, http.MethodPost, "/v1/me/notices/not-an-id/read", nil, nil); r.code != http.StatusBadRequest {
			t.Fatalf("bad id: %d", r.code)
		}

		r = e.do(a, http.MethodPost, "/v1/me/notices/"+ids[0]+"/read", nil, nil)
		if r.code != http.StatusOK || r.body["readAt"] == nil || r.body["id"] != ids[0] {
			t.Fatalf("read: %d %s", r.code, r.raw)
		}
		readAt := r.body["readAt"]
		if r := e.do(a, http.MethodPost, "/v1/me/notices/"+ids[0]+"/read", nil, nil); r.code != http.StatusOK || r.body["readAt"] != readAt {
			t.Fatalf("read again moved readAt: %s", r.raw)
		}
		if r := e.do(a, http.MethodGet, "/v1/me/notices", nil, nil); r.body["unreadCount"] != float64(2) {
			t.Fatalf("unread after read: %v", r.body["unreadCount"])
		}
		if r := e.do("", http.MethodGet, "/v1/me/notices", nil, nil); r.code != http.StatusUnauthorized {
			t.Fatalf("no auth: %d", r.code)
		}
	})
}

// The consent is stored where cmd/feedback-loop fixed reads it
// (users/{uid}/notifyConsent/_feedback_fixed), per caller.
func TestFeedbackFixedConsentAPI(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		ctx := context.Background()
		a, b := "ffc-a"+sfx, "ffc-b"+sfx
		const path = "/v1/me/notify-consent/feedback-fixed"
		if r := e.do(a, http.MethodGet, path, nil, nil); r.code != http.StatusOK || r.body["enabled"] != false {
			t.Fatalf("default: %d %s", r.code, r.raw)
		}
		r := e.do(a, http.MethodPut, path, `{"enabled":true}`, nil)
		if r.code != http.StatusOK || r.body["enabled"] != true || r.body["updatedAt"] == nil {
			t.Fatalf("put: %d %s", r.code, r.raw)
		}
		if ok, err := st.NotifyAllowed(ctx, a, loop.FeedbackFixedConsentID); err != nil || !ok {
			t.Fatalf("stored: %v %v", ok, err)
		}
		if ok, _ := st.NotifyAllowed(ctx, b, loop.FeedbackFixedConsentID); ok {
			t.Fatal("b's consent set by a")
		}
		if r := e.do(a, http.MethodGet, path, nil, nil); r.body["enabled"] != true {
			t.Fatalf("get after put: %s", r.raw)
		}
		if r := e.do(a, http.MethodPut, path, `{"enabled":false}`, nil); r.code != http.StatusOK || r.body["enabled"] != false {
			t.Fatalf("off: %d %s", r.code, r.raw)
		}
		if ok, _ := st.NotifyAllowed(ctx, a, loop.FeedbackFixedConsentID); ok {
			t.Fatal("still on")
		}
		for _, body := range []string{`{}`, `{"allowed":true}`, `{"enabled":"yes"}`} {
			if r := e.do(a, http.MethodPut, path, body, nil); r.code != http.StatusBadRequest {
				t.Fatalf("%s: %d %s", body, r.code, r.raw)
			}
		}
	})
}
