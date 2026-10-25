package appconfig

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// #206: type video の値("video:<端末内 id>|<サムネイル data URL>")を保存できる。
// normalizeValue に video の分岐が無いと ref の時と同じく 422 になる。
func TestRecordsVideoField(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/dev-app"
	b, err := os.ReadFile("testdata/form_video.json")
	if err != nil {
		t.Fatal(err)
	}
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(b), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	recs := app + "/entities/clips/records"
	val := "video:6F1C2B7A-3D4E-4F00-9A1B-2C3D4E5F6A7B|data:image/jpeg;base64,/9j/4AAQSkZJRg=="
	r := e.do("u1", http.MethodPost, recs, map[string]any{"data": map[string]any{"exercise": "スクワット", "video": val}, "clientId": "c-1"}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("create with video: %d %s", r.code, r.raw)
	}
	if got := r.body["data"].(map[string]any)["video"]; got != val {
		t.Errorf("video = %v, want %s", got, val)
	}
	id := r.body["id"].(string)

	// サムネイル無し(id だけ)も通る・PATCH で差し替えられる
	r = e.do("u1", http.MethodPatch, recs+"/"+id, map[string]any{"data": map[string]any{"video": "video:abc_123"}}, map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusOK {
		t.Fatalf("patch video: %d %s", r.code, r.raw)
	}

	for name, bad := range map[string]any{
		"not string":    123,
		"no prefix":     "abc",
		"bad id":        "video:../etc/passwd",
		"bad thumbnail": "video:abc|javascript:alert(1)",
		"too long":      "video:abc|data:image/jpeg;base64," + strings.Repeat("A", maxVideoLen),
	} {
		r := e.do("u1", http.MethodPost, recs, map[string]any{"data": map[string]any{"exercise": "x", "video": bad}}, nil)
		if r.code != http.StatusUnprocessableEntity {
			t.Errorf("%s: code = %d, want 422 (%s)", name, r.code, r.raw)
		}
	}
}

func TestCheckVideoValue(t *testing.T) {
	for _, ok := range []string{"video:a", "video:A-b_9|data:image/jpeg;base64,xx", "https://example.com/v.mp4"} {
		if err := checkVideoValue(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, ng := range []string{"", "video:", "video:a b", "data:image/jpeg;base64,xx", "http://x/v.mp4", "video:a|https://x"} {
		if err := checkVideoValue(ng); err == nil {
			t.Errorf("%q: want error", ng)
		}
	}
}
