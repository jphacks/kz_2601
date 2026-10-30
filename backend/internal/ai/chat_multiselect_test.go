package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// #220: multiSelect is an additive field, present only when true.
func TestChatFirst_MultiSelectContract(t *testing.T) {
	combinable := `{"kind":"proposal","text":"案です。","suggestions":[],"multiSelect":true,"options":[
	  {"title":"グラフを足す","detail":"d","instruction":"ホームに円グラフを追加して"},
	  {"title":"予算バーを足す","detail":"d","instruction":"ホームに予算の進捗バーを追加して"}],"transcript":""}`
	exclusive := `{"kind":"proposal","text":"案です。","suggestions":[],"multiSelect":false,"options":[
	  {"title":"色を青に","detail":"d","instruction":"テーマ色を青にして"},
	  {"title":"色を赤に","detail":"d","instruction":"テーマ色を赤にして"}],"transcript":""}`
	single := `{"kind":"proposal","text":"案です。","suggestions":[],"multiSelect":true,"options":[
	  {"title":"グラフを足す","detail":"d","instruction":"ホームに円グラフを追加して"}],"transcript":""}`
	question := `{"kind":"question","text":"どの項目を一覧に出しますか?","suggestions":["金額","日付","メモ"],"multiSelect":true,"options":[],"transcript":""}`
	answer := `{"kind":"answer","text":"できます。","suggestions":["a","b"],"multiSelect":true,"options":[],"transcript":""}`
	cases := []struct {
		name, reply string
		want        bool
	}{
		{"combinable proposal", combinable, true},
		{"exclusive proposal", exclusive, false},
		{"one option is never multi", single, false},
		{"multi question", question, true},
		{"answer is never multi", answer, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := newEditChatSvc(t, c.reply)
			r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "edit", Messages: user("x")})
			if err != nil {
				t.Fatal(err)
			}
			if r.MultiSelect != c.want {
				t.Fatalf("multiSelect=%v want %v: %+v", r.MultiSelect, c.want, r)
			}
			raw, _ := json.Marshal(r)
			if has := strings.Contains(string(raw), `"multiSelect":true`); has != c.want {
				t.Fatalf("json multiSelect present=%v want %v: %s", has, c.want, raw)
			}
			if !c.want && strings.Contains(string(raw), "multiSelect") {
				t.Fatalf("multiSelect must be omitted when false: %s", raw)
			}
		})
	}
}

func TestChatFirst_MultiSelectSchemaAndPrompt(t *testing.T) {
	for _, s := range []json.RawMessage{ChatEditSchema(), ChatCreateSchema()} {
		var m map[string]any
		if err := json.Unmarshal(s, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["properties"].(map[string]any)["multiSelect"]; !ok {
			t.Fatalf("schema lacks multiSelect: %s", s)
		}
	}
	for _, p := range []string{chatEditSystemPrompt, chatCreateSystemPrompt} {
		if !strings.Contains(p, "multiSelect=true") || !strings.Contains(p, "迷ったら false") {
			t.Fatal("prompt lacks the multiSelect rule")
		}
	}
	r, ok := partialFirstReply(`{"kind":"question","text":"どれ?","suggestions":["a","b"],"multiSelect":true,"options":[],`)
	if !ok || !r.MultiSelect {
		t.Fatalf("partial: %+v %v", r, ok)
	}
}
