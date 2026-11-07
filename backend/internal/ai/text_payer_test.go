package ai

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// sharedAppFixture: "pro-owner" made app P (Pro), "free-owner" made app F (no
// Pro); "member" joined both, "outsider" joined neither.
func sharedAppFixture(t *testing.T, s interface {
	AppMembers
	ClaimApp(ctx context.Context, uid, appID string) (string, error)
	AddMember(ctx context.Context, appID string, m store.Member) (bool, error)
}, suffix string) (proApp, freeApp string) {
	t.Helper()
	ctx := context.Background()
	proApp, freeApp = "P"+suffix, "F"+suffix
	for app, owner := range map[string]string{proApp: "pro-owner", freeApp: "free-owner"} {
		if _, err := s.ClaimApp(ctx, owner, app); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddMember(ctx, app, store.Member{UID: "member", Via: store.MemberViaShare, InvitedBy: owner}); err != nil {
			t.Fatal(err)
		}
	}
	return proApp, freeApp
}

func textReason(t *testing.T, body []byte) string {
	t.Helper()
	var b struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		t.Fatalf("not an error body: %s", body)
	}
	r, _ := b.Error.Details["reason"].(string)
	return r
}

func runTextPayerCases(t *testing.T, members AppMembers, proApp, freeApp string) {
	cases := []struct {
		name, uid, appID string
		status           int
		reason           string
	}{
		{"member of a Pro creator's app", "member", proApp, http.StatusOK, ""},
		{"member of a creator without Pro", "member", freeApp, http.StatusPaymentRequired, reasonCreatorNeedsPro},
		{"non-member with someone else's appId", "outsider", proApp, http.StatusPaymentRequired, ""},
		{"creator without Pro, own app", "free-owner", freeApp, http.StatusPaymentRequired, ""},
		{"no appId", "member", "", http.StatusPaymentRequired, ""},
		{"unknown app", "member", "NOPE" + proApp, http.StatusPaymentRequired, ""},
		{"Pro caller", "pro-owner", freeApp, http.StatusOK, ""},
		{"invalid appId", "member", "a/b", http.StatusBadRequest, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, fc := textSvc(`{"text":"がんばったね"}`)
			svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, members
			body, _ := json.Marshal(map[string]any{"instruction": "励ましの一言", "appId": c.appID})
			rec := postText(t, NewHandler(svc), c.uid, string(body))
			if rec.Code != c.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if c.status == http.StatusOK {
				if len(fc.calls) != 1 {
					t.Fatalf("Claude calls %d", len(fc.calls))
				}
				return
			}
			if len(fc.calls) != 0 {
				t.Fatal("Claude must not be called when refused")
			}
			if c.status == http.StatusPaymentRequired {
				if errCode(t, rec) != httpx.CodeProRequired || !strings.Contains(rec.Body.String(), `"id":"onlineAI"`) {
					t.Fatalf("body %s", rec.Body.String())
				}
				if got := textReason(t, rec.Body.Bytes()); got != c.reason {
					t.Fatalf("reason %q, want %q", got, c.reason)
				}
			}
		})
	}
}

// A participant of a shared app is covered by the creator's Pro; membership
// is read on the server, so another user's appId does not help.
func TestTextCreatorPays(t *testing.T) {
	m := store.NewMemory()
	proApp, freeApp := sharedAppFixture(t, m, "")
	runTextPayerCases(t, m, proApp, freeApp)
}

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...).
func TestFirestoreTextCreatorPays(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := store.NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	proApp, freeApp := sharedAppFixture(t, f, "TXT"+time.Now().Format("150405.000000"))
	runTextPayerCases(t, f, proApp, freeApp)
}

// Without a membership lookup the caller's own Pro decides (as before).
func TestTextCreatorPaysWithoutMembers(t *testing.T) {
	m := store.NewMemory()
	proApp, _ := sharedAppFixture(t, m, "")
	svc, fc := textSvc(`{"text":"x"}`)
	svc.Pro, svc.ProMode = proSet{"pro-owner": true}, paid.ModeEnforce
	rec := postText(t, NewHandler(svc), "member", `{"instruction":"x","appId":"`+proApp+`"}`)
	if rec.Code != http.StatusPaymentRequired || len(fc.calls) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// PRO_GATE_MODE=log lets a participant of a creator without Pro through.
func TestTextCreatorPaysLogMode(t *testing.T) {
	m := store.NewMemory()
	_, freeApp := sharedAppFixture(t, m, "")
	svc, fc := textSvc(`{"text":"x"}`)
	svc.Pro, svc.ProMode, svc.Members = proSet{}, paid.ModeLog, m
	rec := postText(t, NewHandler(svc), "member", `{"instruction":"x","appId":"`+freeApp+`"}`)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// ai.usage of a participant's call names the creator as payer (hash only);
// the caller's own call has no payer field.
func TestTextCreatorPaysUsageLog(t *testing.T) {
	m := store.NewMemory()
	proApp, _ := sharedAppFixture(t, m, "")
	reply, _ := json.Marshal(map[string]any{
		"model":       "claude-test",
		"content":     []map[string]any{{"type": "text", "text": `{"text":"ok"}`}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
	})
	srv, _ := fakeAnthropic(t, [2]string{"200", string(reply)})
	client, buf := testClient(srv, "claude-test")
	svc := &Service{Claude: client, Verifier: auth.DevVerifier{}, Logger: slog.New(slog.NewJSONHandler(buf, nil)),
		Pro: proSet{"pro-owner": true}, ProMode: paid.ModeEnforce, Members: m}
	h := NewHandler(svc)
	for _, uid := range []string{"member", "pro-owner"} {
		if rec := postText(t, h, uid, `{"instruction":"x","appId":"`+proApp+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", uid, rec.Code, rec.Body.String())
		}
	}
	var lines []map[string]any
	for _, l := range usageLines(t, buf) {
		if l["msg"] == UsageEvent {
			lines = append(lines, l)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("usage lines %d: %s", len(lines), buf.String())
	}
	if lines[0]["uid_hash"] != UIDHash("member") || lines[0]["payer_uid_hash"] != UIDHash("pro-owner") {
		t.Fatalf("participant line: %v", lines[0])
	}
	if _, ok := lines[1]["payer_uid_hash"]; ok {
		t.Fatalf("own call has a payer: %v", lines[1])
	}
	if strings.Contains(buf.String(), `"pro-owner"`) || strings.Contains(buf.String(), `"member"`) {
		t.Fatalf("raw uid in the logs: %s", buf.String())
	}
}
