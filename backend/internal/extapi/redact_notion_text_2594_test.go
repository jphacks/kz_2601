package extapi

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Secrets / Token never print or marshal their values.
func TestRedactedStrings2594(t *testing.T) {
	s := Secrets{APIKey: "v1", ClientID: "v2", ClientSecret: "v3"}
	tok := Token{AccessToken: "v4", RefreshToken: "v5", Expiry: time.Unix(0, 0), Scopes: []string{"read"}}

	cases := []struct {
		name string
		got  string
	}{
		{"Secrets.String", s.String()},
		{"Secrets %v", fmt.Sprintf("%v", s)},
		{"Secrets %+v", fmt.Sprintf("%+v", s)},
		{"Secrets LogValue", s.LogValue().String()},
		{"Token LogValue", tok.LogValue().String()},
		{"Token %v", fmt.Sprintf("%v", tok)},
		{"Token %#v", fmt.Sprintf("%#v", tok)},
	}
	for _, c := range cases {
		if c.got != "[redacted]" {
			t.Errorf("%s = %q, want [redacted]", c.name, c.got)
		}
	}

	b, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if string(b) != `"[redacted]"` {
		t.Errorf("json(Token) = %s", b)
	}
	// Nested in a struct (a log payload / response by mistake).
	b, err = json.Marshal(struct {
		T  Token  `json:"t"`
		TP *Token `json:"tp"`
	}{tok, &tok})
	if err != nil {
		t.Fatalf("marshal wrapper: %v", err)
	}
	if string(b) != `{"t":"[redacted]","tp":"[redacted]"}` {
		t.Errorf("json(wrapper) = %s", b)
	}
}

func TestNotionValueText2594(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"title joins rich text", `{"type":"title","title":[{"plain_text":"買い"},{"plain_text":"物"}]}`, "買い物"},
		{"title empty", `{"type":"title","title":[]}`, ""},
		{"rich_text", `{"type":"rich_text","rich_text":[{"plain_text":"a"},{"plain_text":" b"}]}`, "a b"},
		{"rich_text missing", `{"type":"rich_text"}`, ""},
		{"number", `{"type":"number","number":1.5}`, "1.5"},
		{"number null", `{"type":"number","number":null}`, ""},
		{"checkbox true", `{"type":"checkbox","checkbox":true}`, "✓"},
		{"checkbox false", `{"type":"checkbox","checkbox":false}`, "−"},
		{"select", `{"type":"select","select":{"name":"高"}}`, "高"},
		{"select null", `{"type":"select","select":null}`, ""},
		{"status", `{"type":"status","status":{"name":"進行中"}}`, "進行中"},
		{"multi_select", `{"type":"multi_select","multi_select":[{"name":"a"},{"name":"b"}]}`, "a, b"},
		{"multi_select empty", `{"type":"multi_select","multi_select":[]}`, ""},
		{"date", `{"type":"date","date":{"start":"2026-11-13"}}`, "2026-11-13"},
		{"date range", `{"type":"date","date":{"start":"2026-11-13","end":"2026-11-14"}}`, "2026-11-13 → 2026-11-14"},
		{"url", `{"type":"url","url":"https://example.com"}`, "https://example.com"},
		{"email", `{"type":"email","email":"a@example.com"}`, "a@example.com"},
		{"phone", `{"type":"phone_number","phone_number":"000-0000"}`, "000-0000"},
		{"created_time", `{"type":"created_time","created_time":"2026-01-01T00:00:00Z"}`, "2026-01-01T00:00:00Z"},
		{"last_edited_time", `{"type":"last_edited_time","last_edited_time":"2026-01-02T00:00:00Z"}`, "2026-01-02T00:00:00Z"},
		{"unique_id", `{"type":"unique_id","unique_id":{"prefix":null,"number":7}}`, "7"},
		{"unique_id prefix", `{"type":"unique_id","unique_id":{"prefix":"TASK","number":7}}`, "TASK-7"},
		{"unique_id empty prefix", `{"type":"unique_id","unique_id":{"prefix":"","number":7}}`, "7"},
		{"unique_id no number", `{"type":"unique_id","unique_id":{"prefix":"TASK","number":null}}`, ""},
		{"formula string", `{"type":"formula","formula":{"type":"string","string":"x"}}`, "x"},
		{"formula number", `{"type":"formula","formula":{"type":"number","number":2}}`, "2"},
		{"formula boolean", `{"type":"formula","formula":{"type":"boolean","boolean":true}}`, "✓"},
		{"formula date", `{"type":"formula","formula":{"type":"date","date":{"start":"2026-11-13"}}}`, "2026-11-13"},
		{"formula empty", `{"type":"formula","formula":{"type":"string","string":null}}`, ""},
		{"unknown type", `{"type":"relation","relation":[{"id":"x"}]}`, ""},
		{"no type", `{}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var v notionValue
			if err := json.Unmarshal([]byte(c.raw), &v); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := v.text(); got != c.want {
				t.Errorf("text() = %q, want %q", got, c.want)
			}
		})
	}
}
