package ai

import (
	"context"
	"strings"
	"testing"
)

// Replies seen with the real API (#258, m0disc_chat_cases.json).
func TestPlainTerms_IssueExamples(t *testing.T) {
	cases := []struct{ in, want string }{
		{"代わりに、public の記録で使う人みんなの投稿が流れる掲示板は作れます。", "代わりに、みんなに公開の記録で使う人みんなの投稿が流れる掲示板は作れます。"},
		{"支出を public にして共有して", "支出をみんなに公開して共有して"},
		{"今は支出が private なので自分だけが見られます。", "今は支出が自分だけなので自分だけが見られます。"},
		{"乱数の計算式(op)は無いので、ランダムには選べません。", "乱数の計算式は無いので、ランダムには選べません。"},
		{"タップで開く折りたたみ（disclosure）で答えを隠せます。", "タップで開く折りたたみで答えを隠せます。"},
		{"**shared** にすると参加した人と使えます。", "参加した人だけにすると参加した人と使えます。"},
		{"entity に field を足して computed で出します。", "記録の種類に項目を足して自動計算で出します。"},
		{"Public の掲示板", "みんなに公開の掲示板"},
	}
	for _, c := range cases {
		got := plainTerms(c.in, LocaleJA)
		if got != c.want {
			t.Errorf("plainTerms(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if left := LeakedTerms(got); len(left) > 0 {
			t.Errorf("%q still has %v", got, left)
		}
	}
}

func TestPlainTerms_KeepsOrdinaryWords(t *testing.T) {
	for _, s := range []string{
		"いいえ、WebView ではなく iOS のネイティブ画面です。",
		"Instagram のようなフォローはできません。",
		"「メンバー追加」から入力できます。",
		"",
	} {
		if got := plainTerms(s, LocaleJA); got != s {
			t.Errorf("changed %q -> %q", s, got)
		}
	}
	en := "Make it public so friends can add expenses from their phones. Each field is optional."
	if got := plainTerms(en, LocaleEN); got != en {
		t.Errorf("en ordinary words changed: %q", got)
	}
	if got := plainTerms("There is no random op, but a computed total works (entity: expense).", LocaleEN); strings.Contains(got, " op") || strings.Contains(got, "computed") || strings.Contains(got, "entity") {
		t.Errorf("en DSL names left: %q", got)
	}
}

// The chat-first path cleans text, chips, idea titles / details and instructions.
func TestChatFirst_RepliesInPlainWords(t *testing.T) {
	reply := `{"kind":"proposal","text":"今は private なので、public にすると友達も入力できます。","suggestions":[],"multiSelect":false,"options":[
	  {"title":"public にする","detail":"支出を public で共有","instruction":"支出の entity を scope public にして"},
	  {"title":"自分用のまま","detail":"disclosure で隠す","instruction":"このまま"}],"transcript":""}`
	svc, _ := newEditChatSvc(t, reply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "edit", Messages: user("この割り勘、友達のスマホからも入力できる?")})
	if err != nil {
		t.Fatal(err)
	}
	seen := []string{r.Text}
	seen = append(seen, r.Suggestions...)
	for _, o := range r.Options {
		seen = append(seen, o.Title, o.Detail)
	}
	for _, s := range seen {
		if left := LeakedTerms(s); len(left) > 0 {
			t.Errorf("%q still has %v", s, left)
		}
	}
	if r.Text != "今は自分だけなので、みんなに公開すると友達も入力できます。" {
		t.Errorf("text: %q", r.Text)
	}
	// shown on the generating screen too
	if r.Options[0].Instruction != "支出の記録の種類をみんなに公開して" {
		t.Errorf("instruction: %q", r.Options[0].Instruction)
	}
}

func TestChatFirst_AnswerChipsInPlainWords(t *testing.T) {
	reply := `{"kind":"answer","text":"乱数の計算式(op)は無いので、ランダムには選べません。","suggestions":["支出を public にして共有して","一覧を disclosure で畳んで"],"multiSelect":false,"options":[],"transcript":""}`
	svc, _ := newEditChatSvc(t, reply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "edit", Messages: user("ランダムに選べる?")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "乱数の計算式は無いので、ランダムには選べません。" {
		t.Errorf("text: %q", r.Text)
	}
	if got := strings.Join(r.Suggestions, "|"); got != "支出をみんなに公開して共有して|一覧を折りたたみで畳んで" {
		t.Errorf("chips: %q", got)
	}
}

func TestChatPrompts_PlainWordsRule(t *testing.T) {
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		for _, want := range []string{"## 言葉づかい", "public → みんなに公開", "disclosure → 折りたたみ", "作った人だけが変更・削除できる"} {
			if !strings.Contains(p, want) {
				t.Errorf("%s prompt lacks %q", name, want)
			}
		}
	}
}

func TestPlainChatResult_BriefAndTranscript(t *testing.T) {
	res := &ChatResult{Locale: LocaleJA, Transcript: "投稿を scope public にして", Brief: &Brief{Name: "掲示板", Purpose: "public の掲示板", Features: []string{"disclosure で畳む"}}}
	plainChatResult(res)
	if res.Transcript != "投稿をみんなに公開して" || res.Brief.Purpose != "みんなに公開の掲示板" || res.Brief.Features[0] != "折りたたみで畳む" {
		t.Fatalf("%+v %+v", res, res.Brief)
	}
}
