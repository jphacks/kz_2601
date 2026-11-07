package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func testJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func imageSvc(replies ...string) (*Service, *fakeCaller) {
	svc, fc := textSvc(replies...)
	svc.TextImage = true
	return svc, fc
}

func imageBody(t *testing.T, req TextRequest) string {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A photo alone is the material of a transform; it reaches the user turn as
// an image block and the system gets the photo rule.
func TestTextImageTaskBuildsImageBlock(t *testing.T) {
	data := testJPEG(t, 640, 480)
	svc, fc := imageSvc(reply("・ロキソプロフェン 1 錠 毎食後"))
	rec := postText(t, NewHandler(svc), "u1", imageBody(t, TextRequest{
		Task: "extract", Instruction: "薬の名前と飲み方", Image: &TextImage{MediaType: "image/jpeg", Data: data},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if res := textResult(t, rec.Body.Bytes()); !strings.Contains(res.Text, "ロキソプロフェン") {
		t.Fatalf("result %+v", res)
	}
	c := fc.calls[0]
	if len(c.Messages[0].Images) != 1 || c.Messages[0].Images[0].Data != data || !strings.Contains(c.System, textImageRule) {
		t.Fatalf("completion: images %d system has rule %v", len(c.Messages[0].Images), strings.Contains(c.System, textImageRule))
	}
	wire, err := json.Marshal(c.Messages[0])
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Role    string `json:"role"`
		Content []struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Source *struct {
				Type, MediaType, Data string
			} `json:"source"`
		} `json:"content"`
	}
	if err := json.Unmarshal(wire, &got); err != nil {
		t.Fatalf("wire %s: %v", wire, err)
	}
	if got.Role != "user" || len(got.Content) != 2 || got.Content[0].Type != "image" || got.Content[1].Type != "text" ||
		!strings.Contains(string(wire), `"media_type":"image/jpeg"`) || !strings.Contains(got.Content[1].Text, "<task>") {
		t.Fatalf("wire %s", wire)
	}
}

// A text-only turn keeps the plain string content on the wire.
func TestMessageTextOnlyWire(t *testing.T) {
	b, _ := json.Marshal(Message{Role: "user", Content: "hi"})
	if string(b) != `{"role":"user","content":"hi"}` {
		t.Fatalf("%s", b)
	}
	// images are never read from a client body
	var m Message
	if err := json.Unmarshal([]byte(`{"role":"user","content":"x","Images":[{"Data":"AAAA"}]}`), &m); err != nil || len(m.Images) != 0 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestTextImageRejects(t *testing.T) {
	pngBuf := &bytes.Buffer{}
	_ = png.Encode(pngBuf, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	pngData := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	tests := []struct {
		name   string
		img    TextImage
		status int
		code   string
	}{
		{"png media type", TextImage{MediaType: "image/png", Data: pngData}, 400, httpx.CodeBadRequest},
		{"png bytes", TextImage{MediaType: "image/jpeg", Data: pngData}, 400, httpx.CodeBadRequest},
		{"not base64", TextImage{MediaType: "image/jpeg", Data: "@@@"}, 400, httpx.CodeBadRequest},
		{"empty", TextImage{MediaType: "image/jpeg"}, 400, httpx.CodeBadRequest},
		{"too wide", TextImage{MediaType: "image/jpeg", Data: testJPEG(t, 1100, 200)}, 400, httpx.CodeBadRequest},
		{"too large", TextImage{MediaType: "image/jpeg", Data: strings.Repeat("A", TextMaxImageBase64+4)}, 413, httpx.CodePayloadTooLarge},
	}
	svc, fc := imageSvc()
	h := NewHandler(svc)
	for _, tt := range tests {
		img := tt.img
		rec := postText(t, h, "u1", imageBody(t, TextRequest{Instruction: "説明", Image: &img}))
		if rec.Code != tt.status || errCode(t, rec) != tt.code {
			t.Errorf("%s: %d %s", tt.name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), img.Data[:min(len(img.Data), 16)]) && img.Data != "" {
			t.Errorf("%s: the error echoes the image", tt.name)
		}
	}
	if len(fc.calls) != 0 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}

// The edge sizes pass: 1024 px long edge and a body near the image cap.
func TestTextImageEdgeSizes(t *testing.T) {
	svc, fc := imageSvc(reply("ok"))
	if rec := postText(t, NewHandler(svc), "u1", imageBody(t, TextRequest{Instruction: "説明",
		Image: &TextImage{MediaType: "image/jpeg", Data: testJPEG(t, 1024, 768)}})); rec.Code != http.StatusOK {
		t.Fatalf("1024: %d %s", rec.Code, rec.Body.String())
	}
	if len(fc.calls) != 1 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}

// Off by default (the privacy policy does not name photos yet): 403, no call.
func TestTextImageDisabled(t *testing.T) {
	svc, fc := textSvc(reply("x"))
	rec := postText(t, NewHandler(svc), "u1", imageBody(t, TextRequest{Instruction: "説明",
		Image: &TextImage{MediaType: "image/jpeg", Data: testJPEG(t, 32, 32)}}))
	if rec.Code != http.StatusForbidden || errCode(t, rec) != httpx.CodeForbidden || len(fc.calls) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// A photo call is the same Pro feature (onlineAI) as every /v1/text call.
func TestTextImageProRequired(t *testing.T) {
	svc, fc := imageSvc(reply("x"))
	svc.Pro, svc.ProMode = proSet{"pro-user": true}, paid.ModeEnforce
	h := NewHandler(svc)
	body := imageBody(t, TextRequest{Task: "summarize", Image: &TextImage{MediaType: "image/jpeg", Data: testJPEG(t, 32, 32)}})
	if rec := postText(t, h, "free-user", body); rec.Code != http.StatusPaymentRequired || len(fc.calls) != 0 {
		t.Fatalf("free: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "pro-user", body); rec.Code != http.StatusOK {
		t.Fatalf("pro: %d %s", rec.Code, rec.Body.String())
	}
}

// A photo call weighs TextImage (3) even for generate (1); two do not fit in 5.
func TestTextImageRateLimitWeight(t *testing.T) {
	svc, fc := imageSvc(reply("1"), reply("2"))
	lim := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 5}, Weights: ratelimit.DefaultWeights},
		ratelimit.NewMemory(), nil)
	h := lim.Middleware(auth.DevVerifier{})(NewHandler(svc))
	body := imageBody(t, TextRequest{Instruction: "説明", Image: &TextImage{MediaType: "image/jpeg", Data: testJPEG(t, 32, 32)}})
	if rec := postText(t, h, "u1", body); rec.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "u1", body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second (3+3 > 5): %d", rec.Code)
	}
	if len(fc.calls) != 1 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}

// The image bytes never reach a log line (ai.usage via the real client is
// covered by the fake Anthropic server; here the handler's own logs).
func TestTextImageNotLogged(t *testing.T) {
	var logs bytes.Buffer
	svc, _ := imageSvc(reply("ok"))
	svc.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	data := testJPEG(t, 64, 64)
	_ = postText(t, NewHandler(svc), "u1", imageBody(t, TextRequest{Instruction: "説明", Image: &TextImage{MediaType: "image/jpeg", Data: data}}))
	bad := imageBody(t, TextRequest{Instruction: "説明", Image: &TextImage{MediaType: "image/jpeg", Data: data[:len(data)-3] + "@@@"}})
	_ = postText(t, NewHandler(svc), "u1", bad)
	if strings.Contains(logs.String(), data[:32]) {
		t.Fatal("image bytes in the logs")
	}
}

// Through the real client: the request carries the image block, the ai.usage
// line does not carry the bytes.
func TestClientSendsImageNotLogged(t *testing.T) {
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okBody("claude-test"))
	}))
	defer srv.Close()
	c, logs := testClient(srv, "claude-test")
	data := testJPEG(t, 64, 64)
	_, err := c.Complete(context.Background(), Completion{Messages: []Message{{Role: "user", Content: "読む",
		Images: []ImageBlock{{MediaType: "image/jpeg", Data: data}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(sent, []byte(`{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"`+data+`"}}`)) {
		t.Fatalf("request lacks the image block: %.300s", sent)
	}
	if strings.Contains(logs.String(), data[:32]) {
		t.Fatal("image bytes in ai.usage")
	}
}

func TestAIPhotoInputRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## アプリの中の AI に写真を見せる(aiButton inputPhoto)", "inputPhoto: photo"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
}
