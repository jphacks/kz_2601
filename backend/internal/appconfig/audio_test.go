package appconfig

import (
	"net/http"
	"strings"
	"testing"
)

const voiceMemoDSL = `{
  "schemaVersion": 1,
  "app": { "name": "ボイスメモ", "icon": "mic", "voiceEntry": "floating" },
  "initialScreen": "home",
  "entities": [{ "id": "memos", "name": "メモ", "titleField": "title", "fields": [
    { "id": "title", "name": "タイトル", "type": "string", "required": true },
    { "id": "voice", "name": "録音", "type": "audio" }
  ]}],
  "screens": [
    { "id": "home", "title": "ボイスメモ",
      "toolbar": [{ "label": "追加", "icon": "plus", "action": { "type": "navigate", "screen": "form", "presentation": "sheet" } }],
      "components": [{ "type": "list", "entity": "memos", "item": { "title": "title", "trailing": "voice" },
        "onItemTap": { "type": "navigate", "screen": "form" } }] },
    { "id": "form", "title": "録音", "entity": "memos",
      "toolbar": [{ "label": "保存", "action": { "type": "saveRecord", "entity": "memos" } }],
      "components": [
        { "type": "textField", "field": "title", "label": "タイトル" },
        { "type": "audioRecorder", "field": "voice", "label": "録音", "maxDuration": 600 }
      ] }
  ]
}`

// #420: type audio の値("audio:<端末内 id>|<秒>")を保存できる。
func TestRecordsAudioField(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/dev-app"
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody([]byte(voiceMemoDSL)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	recs := app + "/entities/memos/records"
	val := "audio:6F1C2B7A-3D4E-4F00-9A1B-2C3D4E5F6A7B|83"
	r := e.do("u1", http.MethodPost, recs, map[string]any{"data": map[string]any{"title": "定例", "voice": val}, "clientId": "c-1"}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("create with audio: %d %s", r.code, r.raw)
	}
	if got := r.body["data"].(map[string]any)["voice"]; got != val {
		t.Errorf("voice = %v, want %s", got, val)
	}
	for name, bad := range map[string]any{
		"not string": 123,
		"video":      "video:abc",
		"bad id":     "audio:../etc/passwd",
	} {
		r := e.do("u1", http.MethodPost, recs, map[string]any{"data": map[string]any{"title": "x", "voice": bad}}, nil)
		if r.code != http.StatusUnprocessableEntity {
			t.Errorf("%s: code = %d, want 422 (%s)", name, r.code, r.raw)
		}
	}
}

func TestCheckAudioValue(t *testing.T) {
	for _, ok := range []string{"audio:a", "audio:A-b_9|0", "audio:abc|7200", "https://example.com/a.m4a"} {
		if err := checkAudioValue(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, ng := range []string{"", "audio:", "audio:a b", "audio:a|", "audio:a|-1", "audio:a|1.5", "audio:a|99999", "http://x/a.m4a", "audio:a|" + strings.Repeat("9", maxAudioLen)} {
		if err := checkAudioValue(ng); err == nil {
			t.Errorf("%q: want error", ng)
		}
	}
}
