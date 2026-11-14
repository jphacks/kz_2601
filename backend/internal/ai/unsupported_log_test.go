package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

func TestParseUnsupported(t *testing.T) {
	cases := []struct {
		summary    string
		locale     Locale
		requested  string
		substitute string
	}{
		{"家計簿アプリです。カレンダー表示は日付順の一覧で代替しました。", LocaleJA, "カレンダー表示", "日付順の一覧"},
		{"在庫の通知は今はできないため、不足の一覧で代替しました。", LocaleJA, "在庫の通知", "不足の一覧"},
		{"写真を AI に見せる機能は今は使えないため、文字の項目で代替しました。", LocaleJA, "写真を AI に見せる機能", "文字の項目"},
		{"自動の採点を組めなかったため、点数は自分で入力する形で代替しました。", LocaleJA, "自動の採点", "点数は自分で入力する形"},
		{"トーナメント表はこの版では作れないため、出場者の一覧にしました。", LocaleJA, "トーナメント表", ""},
		{"順位は今はできません。", LocaleJA, "順位", ""},
		{"条件で知らせる通知は今はできないため、通知は入れていません。", LocaleJA, "条件で知らせる通知", ""},
		{"通知は入れていません。「毎朝8時に通知して」のように時刻を言えば足せます。", LocaleJA, "通知", ""},
		{"It keeps track of expenses. Calendar view was replaced with a date-sorted list.", LocaleEN, "Calendar view", "a date-sorted list"},
		{"Alerts triggered by a condition aren't possible yet, so no notification was added.", LocaleEN, "Alerts triggered by a condition", ""},
		{"支出を記録できます。毎朝8時に通知が届きます。", LocaleJA, "", ""},
	}
	for _, c := range cases {
		got := ParseUnsupported(c.summary, c.locale)
		if c.requested == "" {
			if len(got) != 0 {
				t.Errorf("%q: want nothing, got %+v", c.summary, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Requested != c.requested || got[0].Substitute != c.substitute {
			t.Errorf("%q: got %+v, want %q / %q", c.summary, got, c.requested, c.substitute)
		}
	}
	// one line per sentence, in order
	got := ParseUnsupported("Aは今はできません。BはCで代替しました。", LocaleJA)
	if len(got) != 2 || got[0].Requested != "A" || got[1].Requested != "B" || got[1].Substitute != "C" {
		t.Fatalf("two: %+v", got)
	}
}

// unsupportedLog runs fn and returns the ai.unsupported lines and the raw log.
func unsupportedLog(t *testing.T, svc *Service, fn func()) ([]map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	svc.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	fn()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		if m["event"] == UnsupportedEvent {
			lines = append(lines, m)
		}
	}
	return lines, buf.String()
}

func TestLogUnsupported_CreateDeclaration(t *testing.T) {
	var app map[string]any
	if err := json.Unmarshal(kakeibo(t), &app); err != nil {
		t.Fatal(err)
	}
	app["app"].(map[string]any)["description"] = "毎日の支出を記録する家計簿。カレンダー表示は日付順の一覧で代替しました。田中さん(090-1234-5678)への送金は今はできません。"
	kb, _ := json.Marshal(app)
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kb)}})
	ctx := WithClientPlatform(auth.ContextWithUID(context.Background(), rawUID), "ios")
	lines, raw := unsupportedLog(t, svc, func() {
		if _, err := svc.Generate(ctx, Request{AppID: "app-RAW-1", Transcript: secretPrompt}); err != nil {
			t.Fatal(err)
		}
	})
	if len(lines) != 2 {
		t.Fatalf("want 2 ai.unsupported lines, got %d: %s", len(lines), raw)
	}
	first := lines[0]
	want := map[string]any{"mode": "create", "locale": "ja", "platform": "ios", "requested": "カレンダー表示", "substitute": "日付順の一覧",
		"note": "カレンダー表示は日付順の一覧で代替しました。", "app_hash": metrics.AppHash("app-RAW-1")}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("%s = %v, want %v", k, first[k], v)
		}
	}
	// the second sentence is masked: no phone number, no name with honorific
	second := lines[1]
	if req, _ := second["requested"].(string); strings.Contains(req, "090") || strings.Contains(req, "田中") || second["substitute"] != "" {
		t.Errorf("second line not masked: %v", second)
	}
	for _, leak := range []string{"SECRET-PROMPT", "app-RAW-1", rawUID, "毎日の支出", "entities", "090-1234"} {
		if strings.Contains(raw, leak) {
			t.Errorf("log leaks %q: %s", leak, raw)
		}
	}
}

func TestLogUnsupported_NothingWithoutDeclaration(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kakeibo(t))}})
	lines, _ := unsupportedLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(lines) != 0 {
		t.Fatalf("lines without a declaration: %v", lines)
	}
	// platform outside HTTP is "unknown"
	var app map[string]any
	_ = json.Unmarshal(kakeibo(t), &app)
	app["app"].(map[string]any)["description"] = "順位は合計の降順の表で代替しました。"
	kb, _ := json.Marshal(app)
	svc, _ = newSvc(&fakeCaller{replies: []string{string(kb)}})
	lines, _ = unsupportedLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "ランキングのアプリ"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(lines) != 1 || lines[0]["platform"] != "unknown" || lines[0]["requested"] != "順位" {
		t.Fatalf("lines: %v", lines)
	}
}
