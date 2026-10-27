package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func bearer(r *http.Request, token string) *http.Request {
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// startChat runs one POST /v1/chat turn on appID as uid ("" = no token).
func startChat(t *testing.T, h http.Handler, uid, appID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"appId":"`+appID+`","messages":[{"role":"user","text":"家計簿"}]}`))
	if uid != "" {
		bearer(req, uid)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat as %q: %d %s", uid, rec.Code, rec.Body.String())
	}
}

// #286: DELETE /v1/chat/{appId} needs a token and clears only the caller's
// own conversation.
func TestChatDelete_Auth(t *testing.T) {
	tests := []struct {
		name       string
		token      string // "" = no Authorization header
		wantStatus int
		wantCode   string
		ownerKept  bool // u-owner's conversation on a1 survives
	}{
		{name: "no token", wantStatus: http.StatusUnauthorized, wantCode: httpx.CodeUnauthenticated, ownerKept: true},
		{name: "invalid token", token: "not a uid!", wantStatus: http.StatusUnauthorized, wantCode: httpx.CodeUnauthenticated, ownerKept: true},
		{name: "another user", token: "u-other", wantStatus: http.StatusForbidden, wantCode: httpx.CodeForbidden, ownerKept: true},
		{name: "owner", token: "u-owner", wantStatus: http.StatusNoContent, ownerKept: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newChatSvc(questionReply)
			svc.Verifier = auth.DevVerifier{}
			h := NewHandler(svc)
			startChat(t, h, "u-owner", "a1")

			req := httptest.NewRequest(http.MethodDelete, "/v1/chat/a1", nil)
			if tt.token != "" {
				bearer(req, tt.token)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var body httpx.ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != tt.wantCode {
					t.Fatalf("code: %s (err %v), want %s", rec.Body.String(), err, tt.wantCode)
				}
			}
			if kept := svc.conv.get(convKey("u-owner", "a1")) != nil; kept != tt.ownerKept {
				t.Fatalf("owner conversation kept = %v, want %v", kept, tt.ownerKept)
			}
		})
	}
}

func TestChatDelete_NothingToClearIs204(t *testing.T) {
	svc, _ := newChatSvc()
	svc.Verifier = auth.DevVerifier{}
	rec := httptest.NewRecorder()
	NewHandler(svc).ServeHTTP(rec, bearer(httptest.NewRequest(http.MethodDelete, "/v1/chat/none", nil), "u1"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatDelete_NoVerifierIs401(t *testing.T) {
	svc, _ := newChatSvc()
	rec := httptest.NewRecorder()
	NewHandler(svc).ServeHTTP(rec, bearer(httptest.NewRequest(http.MethodDelete, "/v1/chat/a1", nil), "u1"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

// Each caller has their own conversation on an appId: another user's turn
// neither continues nor replaces the owner's.
func TestChat_ConversationIsPerCaller(t *testing.T) {
	svc, _ := newChatSvc(questionReply, questionReply)
	svc.Verifier = auth.DevVerifier{}
	h := NewHandler(svc)
	startChat(t, h, "u1", "a1")
	startChat(t, h, "u2", "a1")
	for _, uid := range []string{"u1", "u2"} {
		cv := svc.conv.get(convKey(uid, "a1"))
		if cv == nil || len(cv.messages) != 2 {
			t.Fatalf("%s: conversation %+v, want its own 2 messages", uid, cv)
		}
	}
	if svc.conv.get("a1") != nil {
		t.Fatal("verified callers must not use the bare appId key")
	}
}

func TestConvStoreClear(t *testing.T) {
	st := newConvStore()
	st.put(convKey("u1", "a1"), &conversation{})
	st.put("a2", &conversation{}) // tokenless
	if st.clear("u2", "a1") != true {
		t.Fatal("u2 on a1 should be foreign")
	}
	if st.clear("u2", "a2") != true {
		t.Fatal("a tokenless conversation is not u2's either")
	}
	if st.clear("u1", "a1") || st.get(convKey("u1", "a1")) != nil {
		t.Fatal("u1 clears its own")
	}
	if st.clear("u1", "a1") {
		t.Fatal("nothing left: not foreign")
	}
}
