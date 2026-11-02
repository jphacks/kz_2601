package appconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func feedbackEnv(t *testing.T) (*env, *store.Memory) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("FEEDBACK_DAILY_LIMIT", "3")
	mem := store.NewMemory()
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(mem))}, mem
}

func onlyFeedback(t *testing.T, mem *store.Memory) *store.Feedback {
	t.Helper()
	items, err := mem.ListFeedback(context.Background(), store.FeedbackQuery{})
	if err != nil || len(items) != 1 {
		t.Fatalf("want 1 feedback, got %d %v", len(items), err)
	}
	fb, err := mem.GetFeedback(context.Background(), items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return fb
}

func TestFeedbackMessageOnlyByDefault(t *testing.T) {
	e, mem := feedbackEnv(t)
	r := e.do("u1", http.MethodPost, "/v1/feedback", `{"kind":"bug","message":"  ボタンが押せない  ","consentVersion":"feedback-2026-11"}`, nil)
	if r.code != http.StatusCreated || r.body["status"] != "received" || !strings.HasPrefix(r.body["id"].(string), "fb_") {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	att, _ := r.body["attachments"].(map[string]any)
	if att["screenshot"] != false || att["dsl"] != false || att["device"] != false {
		t.Fatalf("no attachments by default: %s", r.raw)
	}
	fb := onlyFeedback(t, mem)
	if fb.UID != "u1" || fb.Message != "ボタンが押せない" || fb.Kind != "bug" || fb.ConsentVersion != "feedback-2026-11" ||
		len(fb.Screenshot) != 0 || fb.DSL != nil || fb.Device != nil || len(fb.Attachments()) != 0 {
		t.Fatalf("stored: %+v", fb)
	}
}

func TestFeedbackValidation(t *testing.T) {
	e, mem := feedbackEnv(t)
	jpeg := base64.StdEncoding.EncodeToString(append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 16)...))
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n...."))
	big := base64.StdEncoding.EncodeToString(append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, maxFeedbackScreenshot)...))
	cases := []struct {
		name string
		body any
		code int
	}{
		{"empty message", `{"message":"  ","consentVersion":"v1"}`, http.StatusBadRequest},
		{"long message", map[string]any{"message": strings.Repeat("あ", maxFeedbackMessage+1), "consentVersion": "v1"}, http.StatusBadRequest},
		{"no consent", `{"message":"hi"}`, http.StatusBadRequest},
		{"unknown key (records)", `{"message":"hi","consentVersion":"v1","records":[{"a":1}]}`, http.StatusBadRequest},
		{"unknown attachment", `{"message":"hi","consentVersion":"v1","attachments":{"records":[]}}`, http.StatusBadRequest},
		{"not base64", `{"message":"hi","consentVersion":"v1","attachments":{"screenshot":"%%%"}}`, http.StatusBadRequest},
		{"png", map[string]any{"message": "hi", "consentVersion": "v1", "attachments": map[string]any{"screenshot": png}}, http.StatusBadRequest},
		{"too big", map[string]any{"message": "hi", "consentVersion": "v1", "attachments": map[string]any{"screenshot": big}}, http.StatusRequestEntityTooLarge},
		{"bad app", `{"message":"hi","consentVersion":"v1","attachments":{"app":{"appId":"null","dslVersion":1}}}`, http.StatusBadRequest},
		{"long device", map[string]any{"message": "hi", "consentVersion": "v1", "attachments": map[string]any{"device": map[string]any{"model": strings.Repeat("x", 65)}}}, http.StatusBadRequest},
	}
	for _, c := range cases {
		if r := e.do("u1", http.MethodPost, "/v1/feedback", c.body, nil); r.code != c.code {
			t.Errorf("%s: want %d, got %d %s", c.name, c.code, r.code, r.raw)
		}
	}
	if r := e.do("u1", http.MethodPost, "/v1/feedback", map[string]any{"message": "ok", "consentVersion": "v1", "attachments": map[string]any{"screenshot": jpeg}}, nil); r.code != http.StatusCreated {
		t.Fatalf("valid jpeg: %d %s", r.code, r.raw)
	}
	if fb := onlyFeedback(t, mem); !bytes.HasPrefix(fb.Screenshot, jpegMagic) || fb.Attachments()[0] != "screenshot" {
		t.Fatalf("screenshot stored: %v", fb.Attachments())
	}
}

func TestFeedbackDSLSnapshotFromOwnTree(t *testing.T) {
	e, mem := feedbackEnv(t)
	e.ownApp("owner", "/v1/apps/OWNER-KAKEIBO", kakeibo(t))
	e.do("owner", http.MethodPost, "/v1/apps/OWNER-KAKEIBO/entities/expense/records", `{"data":{"amount":1200,"note":"秘密のメモ"}}`, nil)

	// someone else's appId: nothing attached (their tree has no such app)
	body := `{"message":"x","consentVersion":"v1","attachments":{"app":{"appId":"OWNER-KAKEIBO","dslVersion":2},"device":{"appVersion":"1.0","build":"12","os":"iOS 26.1","model":"iPhone17,1","locale":"ja"}}}`
	r := e.do("other", http.MethodPost, "/v1/feedback", body, nil)
	if att, _ := r.body["attachments"].(map[string]any); r.code != http.StatusCreated || att["dsl"] != false || att["device"] != true {
		t.Fatalf("other's app must not attach: %d %s", r.code, r.raw)
	}
	r = e.do("owner", http.MethodPost, "/v1/feedback", body, nil)
	if att, _ := r.body["attachments"].(map[string]any); r.code != http.StatusCreated || att["dsl"] != true {
		t.Fatalf("own app: %d %s", r.code, r.raw)
	}
	items, _ := mem.ListFeedback(context.Background(), store.FeedbackQuery{})
	var fb *store.Feedback
	for _, it := range items {
		if it.UID == "owner" {
			fb, _ = mem.GetFeedback(context.Background(), it.ID)
		}
	}
	if fb == nil || fb.AppID != "OWNER-KAKEIBO" || fb.DSLVersion != 2 || fb.Device.Model != "iPhone17,1" {
		t.Fatalf("stored: %+v", fb)
	}
	var d map[string]any
	if err := json.Unmarshal(fb.DSL, &d); err != nil || d["initialScreen"] == nil {
		t.Fatalf("dsl snapshot: %v %s", err, fb.DSL)
	}
	if strings.Contains(string(fb.DSL), "秘密のメモ") {
		t.Fatal("record contents must never be attached")
	}
}

func TestFeedbackDailyLimit(t *testing.T) {
	e, _ := feedbackEnv(t) // FEEDBACK_DAILY_LIMIT=3
	for i := range 3 {
		if r := e.do("u1", http.MethodPost, "/v1/feedback", `{"message":"m","consentVersion":"v1"}`, nil); r.code != http.StatusCreated {
			t.Fatalf("#%d: %d %s", i, r.code, r.raw)
		}
	}
	r := e.do("u1", http.MethodPost, "/v1/feedback", `{"message":"m","consentVersion":"v1"}`, nil)
	errBody, _ := r.body["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	if r.code != http.StatusTooManyRequests || errBody["code"] != "rate_limited" || details["scope"] != "user" || details["limit"] != float64(3) {
		t.Fatalf("4th: %d %s", r.code, r.raw)
	}
	// per uid
	if r := e.do("u2", http.MethodPost, "/v1/feedback", `{"message":"m","consentVersion":"v1"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("other user: %d %s", r.code, r.raw)
	}
}

func TestFeedbackPurgedWithAccount(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemory()
	for _, uid := range []string{"gone", "gone", "stay"} {
		if _, err := mem.CreateFeedback(ctx, store.Feedback{UID: uid, Message: "m", ConsentVersion: "v1"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := mem.DeactivateUser(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	res, err := mem.PurgeAccount(ctx, "gone")
	if err != nil || res.Feedback != 2 {
		t.Fatalf("purge: %+v %v", res, err)
	}
	items, _ := mem.ListFeedback(ctx, store.FeedbackQuery{})
	if len(items) != 1 || items[0].UID != "stay" {
		t.Fatalf("left: %+v", items)
	}
}
