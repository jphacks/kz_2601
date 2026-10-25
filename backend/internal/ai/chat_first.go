package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Chat-first (#184 / #185): the chat is the main way to change an app, for
// creating and for editing. On top of question / ready the reply can be
//   - answer:   a reply to a question about the app or about what the DSL can
//     do (the app does not change)
//   - proposal: 1..3 ideas; each Instruction is what iOS passes to
//     POST /v1/generate when the user taps 「これでいく」
//
// Old iOS builds know only question / ready, so these kinds are returned only
// when the request carries Mode or Capabilities (answer / proposal); such
// requests are handled here, everything else stays in Chat's clarify path.

const (
	chatModeCreate = "create"
	chatModeEdit   = "edit"

	chatFirstMaxTokens = 2048
	chatMaxOptions     = 3
)

// ChatOption is one idea of a proposal.
type ChatOption struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Instruction string `json:"instruction"`
	// #77: same as ChatResult.RequiresPro, per idea.
	RequiresPro     bool          `json:"requiresPro,omitempty"`
	PaidFeatures    []PaidFeature `json:"paidFeatures,omitempty"`
	FreeInstruction string        `json:"freeInstruction,omitempty"`
}

// chatFirstReply is the structured output of a chat-first turn. Brief is
// only in the create schema.
type chatFirstReply struct {
	Kind        string         `json:"kind"`
	Text        string         `json:"text"`
	Suggestions []string       `json:"suggestions"`
	Options     []chatOptionIn `json:"options"`
	Brief       Brief          `json:"brief"`
	Transcript  string         `json:"transcript"`
}

type chatOptionIn struct {
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Instruction string `json:"instruction"`
}

const chatOptionsSchema = `"options": {"type": "array", "items": {
      "type": "object", "additionalProperties": false,
      "required": ["title", "detail", "instruction"],
      "properties": {"title": {"type": "string"}, "detail": {"type": "string"}, "instruction": {"type": "string"}}}}`

// ChatEditSchema is the structured output of an edit-mode turn. The fields
// that are spoken first (kind/text/suggestions/options) come first so the
// reply can be returned while transcript is still streaming.
func ChatEditSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["kind", "text", "suggestions", "options", "transcript"],
  "properties": {
    "kind": {"type": "string", "enum": ["answer", "question", "proposal", "ready"]},
    "text": {"type": "string"},
    "suggestions": {"type": "array", "items": {"type": "string"}},
    ` + chatOptionsSchema + `,
    "transcript": {"type": "string"}
  }
}`)
}

// ChatCreateSchema is ChatSchema plus answer / proposal and options.
func ChatCreateSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["kind", "text", "suggestions", "options", "brief", "transcript"],
  "properties": {
    "kind": {"type": "string", "enum": ["answer", "question", "proposal", "ready"]},
    "text": {"type": "string"},
    "suggestions": {"type": "array", "items": {"type": "string"}},
    ` + chatOptionsSchema + `,
    "brief": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "purpose", "entities", "features", "native"],
      "properties": {
        "name": {"type": "string"},
        "purpose": {"type": "string"},
        "entities": {"type": "array", "items": {"type": "string"}},
        "features": {"type": "array", "items": {"type": "string"}},
        "native": {"type": "array", "items": {"type": "string"}}
      }
    },
    "transcript": {"type": "string"}
  }
}`)
}

// wantsChatFirst reports whether the client opted in to answer / proposal.
func wantsChatFirst(req ChatRequest) bool {
	if strings.TrimSpace(req.Mode) != "" {
		return true
	}
	for _, c := range req.Capabilities {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "answer", "proposal":
			return true
		}
	}
	return false
}

// chatMode resolves create|edit and, for edit, the current app.
func (s *Service) chatMode(ctx context.Context, req ChatRequest) (string, *dsl.App, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case "", "auto", chatModeCreate, chatModeEdit:
	default:
		return "", nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "mode must be create|edit"}
	}
	if mode == chatModeCreate || s.Store == nil {
		return chatModeCreate, nil, nil
	}
	env, err := s.Store.Get(ctx, req.AppID)
	if err != nil {
		if mode == chatModeEdit {
			return "", nil, mapStoreErr(err)
		}
		// auto: the chat still works without the store; assume a new app.
		log.Printf("chat: store get %s: %v (falling back to create)", req.AppID, err)
		return chatModeCreate, nil, nil
	}
	app, err := dsl.Parse(env.DSL)
	if err != nil {
		return "", nil, &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "current DSL is invalid: " + err.Error()}
	}
	if isInitial(app) {
		// nothing to edit yet: talk about the app to create.
		return chatModeCreate, nil, nil
	}
	return chatModeEdit, app, nil
}

// chatFirst runs one chat-first turn (create or edit).
func (s *Service) chatFirst(ctx context.Context, req ChatRequest, incoming []ChatMessage, started time.Time) (out *ChatResult, err error) {
	mode, app, err := s.chatMode(ctx, req)
	if err != nil {
		return nil, err
	}
	// #77: ready / proposal に有料機能の判定を付ける(edit で今のアプリが既に持つ機能は除く)
	defer func() {
		if err == nil {
			defaultPaidCatalog.annotate(out, app)
		}
	}()
	store := s.conversations()
	store.wait(ctx, req.AppID, s.chatTimeout())
	cv := store.get(req.AppID)
	if cv == nil || cv.mode != mode {
		// a create conversation does not carry over into editing (and back)
		cv = &conversation{mode: mode}
	}
	cv.messages = mergeHistory(cv.messages, incoming)
	if len(cv.messages) == 0 || cv.messages[len(cv.messages)-1].Role != "user" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "messages must end with a user message"}
	}
	userTexts := make([]string, 0, len(cv.messages))
	for _, m := range cv.messages {
		if m.Role == "user" {
			userTexts = append(userTexts, m.Text)
		}
	}
	turn := len(userTexts)
	loc, err := resolveLocale(req.Locale, strings.Join(userTexts, "\n"))
	if err != nil {
		return nil, err
	}
	last := userTexts[turn-1]
	cfg := s.ChatConfig.resolved()
	res := &ChatResult{Turn: turn, Locale: loc, Mode: mode, Meta: Meta{Model: cfg.Model}}

	var system string
	var schema json.RawMessage
	var prompt func(problems []string) string
	force := false
	noMoreQuestions := mode == chatModeCreate && cv.questions >= chatMaxTurns
	if mode == chatModeCreate {
		goAhead := wantsBuildNow(last)
		if goAhead && cv.brief != nil {
			res.Kind, res.Source = "ready", "shortcut"
			res.Text = loc.text().readyText
			res.Brief = cv.brief
			res.Transcript = cv.transcript
			cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
			store.put(req.AppID, cv)
			res.LatencyMs = Latency{Total: time.Since(started).Milliseconds()}
			return res, nil
		}
		force = goAhead
		system, schema = chatCreateSystemPrompt, ChatCreateSchema()
		msgs := cv.messages
		prompt = func(p []string) string { return chatCreateUserMessage(msgs, loc, force, noMoreQuestions, p) }
	} else {
		system, schema = chatEditSystemPrompt, ChatEditSchema()
		msgs := cv.messages
		summary := summarizeApp(app)
		prompt = func(p []string) string { return chatEditUserMessage(msgs, loc, summary, p) }
	}
	// First attempt in the background; an early answer / question / proposal
	// (kind, text, suggestions, options complete) wins the race (#159).
	ar := &attemptResult{}
	bgAR := &attemptResult{}
	early := make(chan earlyFirst, 1)
	done := make(chan firstAttempt, 1)
	bctx := context.WithoutCancel(ctx)
	callStart := time.Now()
	canEarly := !force
	go func() {
		var onEarly func(earlyFirst)
		if canEarly {
			onEarly = func(e earlyFirst) {
				// a question past the limit becomes a forced ready, which
				// needs the transcript: wait for the whole reply
				if !noMoreQuestions || e.reply.Kind != "question" {
					early <- e
				}
			}
		}
		done <- s.chatFirstAttempt(bctx, cfg, system, schema, prompt(nil), bgAR, onEarly)
	}()
	var att firstAttempt
	select {
	case e := <-early:
		s.applyFirstReply(res, &e.reply, mode, loc, userTexts, false, noMoreQuestions)
		res.Source = "claude"
		res.FirstTokenMs = e.ttft.Milliseconds()
		res.Meta = Meta{Model: cfg.Model, Attempts: 1}
		if res.Kind == "question" {
			cv.questions++
		}
		cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: historyText(res)})
		if mode == chatModeCreate {
			pending := store.begin(req.AppID)
			store.put(req.AppID, cv)
			go func() {
				a := <-done
				store.finish(req.AppID, pending, cv, func(c *conversation) {
					if a.reply != nil {
						b := cleanBrief(a.reply.Brief)
						c.brief, c.transcript = &b, transcriptOr(a.reply.Transcript, userTexts)
					}
				})
			}()
		} else {
			store.put(req.AppID, cv)
			go func() { <-done }()
		}
		res.LatencyMs = Latency{Claude: time.Since(callStart).Milliseconds(), Total: time.Since(started).Milliseconds()}
		return res, nil
	case att = <-done:
		*ar = *bgAR
	case <-ctx.Done():
		return nil, &Error{Status: http.StatusGatewayTimeout, Code: "timeout", Message: "chat turn cancelled"}
	}
	if att.err != nil {
		return nil, att.err
	}
	if att.reply == nil {
		att2 := s.chatFirstAttempt(ctx, cfg, system, schema, prompt(att.problems), ar, nil)
		if att2.err != nil {
			return nil, att2.err
		}
		if att2.reply == nil {
			return nil, &Error{Status: http.StatusBadGateway, Code: "generation_failed",
				Message: "could not produce a chat turn", Problems: att2.problems, Attempts: ar.meta.Attempts}
		}
		att = att2
	}
	res.FirstTokenMs = att.ttft.Milliseconds()
	res.Meta = ar.meta
	res.Source = "claude"
	s.applyFirstReply(res, att.reply, mode, loc, userTexts, force, noMoreQuestions)
	if mode == chatModeCreate {
		b := cleanBrief(att.reply.Brief)
		cv.brief, cv.transcript = &b, transcriptOr(att.reply.Transcript, userTexts)
	}
	if res.Kind == "question" {
		cv.questions++
	}
	cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: historyText(res)})
	store.put(req.AppID, cv)
	res.LatencyMs = Latency{Claude: ar.latency.Milliseconds(), Total: time.Since(started).Milliseconds()}
	return res, nil
}

// applyFirstReply maps the model reply onto the response.
func (s *Service) applyFirstReply(res *ChatResult, r *chatFirstReply, mode string, loc Locale, userTexts []string, force, noMoreQuestions bool) {
	kind := r.Kind
	text := strings.TrimSpace(r.Text)
	opts := cleanOptions(r.Options)
	if kind == "proposal" && len(opts) == 0 {
		kind = "answer" // ideas without a usable instruction: keep the text
	}
	if mode == chatModeCreate && (force || (noMoreQuestions && kind == "question")) && kind != "ready" {
		kind = "ready"
		res.Source = "forced"
		text = ""
	}
	res.Kind = kind
	switch kind {
	case "ready":
		if mode == chatModeCreate {
			if text == "" {
				text = loc.text().readyText
			}
			b := cleanBrief(r.Brief)
			res.Brief = &b
			res.Transcript = transcriptOr(r.Transcript, userTexts)
		} else {
			if text == "" {
				text = loc.text().editReadyText
			}
			res.Transcript = strings.TrimSpace(r.Transcript)
			if res.Transcript == "" {
				res.Transcript = userTexts[len(userTexts)-1]
			}
		}
	case "proposal":
		if text == "" {
			text = loc.text().proposalText
		}
		res.Options = opts
	default: // question | answer
		res.Suggestions = cleanSuggestions(r.Suggestions)
	}
	res.Text = text
	if res.Source == "" {
		res.Source = "claude"
	}
}

// cleanOptions trims, drops ideas without a title or instruction, caps at 3
// and numbers them.
func cleanOptions(in []chatOptionIn) []ChatOption {
	out := make([]ChatOption, 0, len(in))
	for _, o := range in {
		t, d, ins := strings.TrimSpace(o.Title), strings.TrimSpace(o.Detail), strings.TrimSpace(o.Instruction)
		if t == "" || ins == "" {
			continue
		}
		out = append(out, ChatOption{ID: fmt.Sprintf("opt%d", len(out)+1), Title: t, Detail: d, Instruction: ins})
		if len(out) == chatMaxOptions {
			break
		}
	}
	return out
}

// historyText is what the next turn sees of this reply: proposals keep their
// ideas so 「2 番目で」 can be resolved.
func historyText(res *ChatResult) string {
	if res.Kind != "proposal" || len(res.Options) == 0 {
		return res.Text
	}
	var b strings.Builder
	b.WriteString(res.Text)
	for i, o := range res.Options {
		fmt.Fprintf(&b, "\n案%d「%s」: %s(指示: %s)", i+1, o.Title, o.Detail, o.Instruction)
	}
	return b.String()
}

type earlyFirst struct {
	reply chatFirstReply
	ttft  time.Duration
}

type firstAttempt struct {
	reply    *chatFirstReply
	problems []string
	ttft     time.Duration
	err      error
}

func (s *Service) chatFirstAttempt(ctx context.Context, cfg ChatConfig, system string, schema json.RawMessage, user string, ar *attemptResult, onEarly func(earlyFirst)) firstAttempt {
	var buf strings.Builder
	sent := false
	start := time.Now()
	var first time.Duration
	resp, ttft, err := s.callChatWith(ctx, cfg, system, schema, chatFirstMaxTokens, user, ar, func(delta string) {
		if first == 0 {
			first = time.Since(start)
		}
		if onEarly == nil || sent {
			return
		}
		buf.WriteString(delta)
		if r, ok := partialFirstReply(buf.String()); ok {
			sent = true
			onEarly(earlyFirst{reply: r, ttft: first})
		}
	})
	if err != nil {
		return firstAttempt{err: err}
	}
	var r chatFirstReply
	if uerr := json.Unmarshal(extractJSON(resp.Text), &r); uerr != nil {
		return firstAttempt{problems: []string{"output is not valid JSON for the schema: " + uerr.Error()}, ttft: ttft}
	}
	switch r.Kind {
	case "answer", "question", "proposal", "ready":
	default:
		return firstAttempt{problems: []string{`kind must be "answer", "question", "proposal" or "ready"`}, ttft: ttft}
	}
	return firstAttempt{reply: &r, ttft: ttft}
}

// partialFirstReply reports an answer / question / proposal whose kind, text,
// suggestions and options are complete in the streamed JSON prefix. A ready
// waits for the transcript (the last field).
func partialFirstReply(prefix string) (chatFirstReply, bool) {
	f := completeFields(prefix)
	var r chatFirstReply
	if f["kind"] == nil || f["text"] == nil || f["suggestions"] == nil || f["options"] == nil {
		return r, false
	}
	if json.Unmarshal(f["kind"], &r.Kind) != nil {
		return r, false
	}
	switch r.Kind {
	case "answer", "question", "proposal":
	default:
		return r, false
	}
	if json.Unmarshal(f["text"], &r.Text) != nil || strings.TrimSpace(r.Text) == "" {
		return r, false
	}
	if json.Unmarshal(f["suggestions"], &r.Suggestions) != nil || json.Unmarshal(f["options"], &r.Options) != nil {
		return r, false
	}
	if r.Kind == "proposal" && len(cleanOptions(r.Options)) == 0 {
		return r, false
	}
	return r, true
}

// summarizeApp renders the current app as a compact outline for the edit
// chat: app / tabs / notifications, each entity with its fields, then each
// screen with its components (one compact JSON line each, children indented).
func summarizeApp(a *dsl.App) string {
	var b strings.Builder
	fmt.Fprintf(&b, "アプリ名: %s\n", a.App.Name)
	if a.App.Description != "" {
		fmt.Fprintf(&b, "説明: %s\n", a.App.Description)
	}
	if a.App.Theme != nil {
		if raw, err := json.Marshal(a.App.Theme); err == nil {
			fmt.Fprintf(&b, "テーマ: %s\n", raw)
		}
	}
	if len(a.App.Tabs) > 0 {
		tabs := make([]string, 0, len(a.App.Tabs))
		for _, t := range a.App.Tabs {
			tabs = append(tabs, fmt.Sprintf("%s(画面 %s)", t.Title, t.Screen))
		}
		fmt.Fprintf(&b, "タブ: %s\n", strings.Join(tabs, " / "))
	} else {
		b.WriteString("タブ: なし\n")
	}
	fmt.Fprintf(&b, "最初の画面: %s\n", a.InitialScreen)
	if len(a.App.Notifications) > 0 {
		raw, _ := json.Marshal(a.App.Notifications)
		fmt.Fprintf(&b, "通知: %s\n", raw)
	} else {
		b.WriteString("通知: なし\n")
	}
	b.WriteString("\n### 記録の種類(entity)\n")
	if len(a.Entities) == 0 {
		b.WriteString("なし\n")
	}
	for _, e := range a.Entities {
		scope := string(e.EffectiveScope())
		fmt.Fprintf(&b, "- %s「%s」(scope %s)\n", e.ID, e.Name, scope)
		for _, f := range e.Fields {
			raw, _ := json.Marshal(f)
			fmt.Fprintf(&b, "  - %s\n", raw)
		}
	}
	b.WriteString("\n### 画面(部品は area の行が小さいほど上、列 A が左。area なしは上から積む)\n")
	for _, sc := range a.Screens {
		kind := ""
		if sc.Entity != "" {
			kind = fmt.Sprintf("・%s の入力フォーム", sc.Entity)
		}
		fmt.Fprintf(&b, "- 画面 %s「%s」%s\n", sc.ID, sc.Title, kind)
		for _, t := range sc.Toolbar {
			raw, _ := json.Marshal(t)
			fmt.Fprintf(&b, "  - ツールバー %s\n", raw)
		}
		writeComponents(&b, sc.Components, "  ")
	}
	return b.String()
}

func writeComponents(b *strings.Builder, cs []dsl.Component, indent string) {
	for _, c := range cs {
		children := c.Children
		c.Children = nil
		raw, err := json.Marshal(c)
		if err != nil {
			raw = []byte(`{"type":"` + string(c.Type) + `"}`)
		}
		fmt.Fprintf(b, "%s- %s\n", indent, raw)
		if len(children) > 0 {
			writeComponents(b, children, indent+"  ")
		}
	}
}

// chatCapabilities is what the DSL can and cannot do, for answers. Hand
// written: the catalog lists the parts, this lists the facts users ask about.
var chatCapabilities = strings.TrimSpace(`
## このアプリの仕組み(質問への答えの根拠)
- アプリは iOS の SwiftUI でネイティブに描画している(Server-Driven UI: 画面の設計図 JSON を AI が作り、iOS がそれを部品に組み立てる)。**WebView ではない**。webView 部品がある画面のその枠だけ WKWebView で https のページを表示する。
- 変えられる単位: 画面(最大 12)・下のタブ(2〜5)・記録の種類 entity(最大 6、項目は 20 まで)・部品・計算・通知・テーマ色・アイコン・アプリ名。声やチャットで頼むと AI が設計図を書き換える。
- 部品の大きさ・位置: 各画面は横 A〜F の 6 列 × 縦 12 行のグリッドで、部品の枠(area)を広げる・狭める・動かすことができる。文字の大きさは text の style(title / headline / body / caption の 4 段)。画像の高さは small / medium / large。**フォントのポイント数・好きなフォント・アニメーション・自由なレイアウト(重ね合わせ)は指定できない**。色はテーマ色と決まった色名(semantic color)だけ。
- 記録と表示: 入力(文字・数字・金額・日付・時刻・選択肢・オンオフ・写真・スライダー・星・色など)、一覧(並べ替え・絞り込み・検索・スワイプ削除・タップで編集・ワンタップのチェック)、表・タイムライン・月のカレンダー表示。
- 集計と計算: 合計・件数・平均・最大・最小・最新・最初、今日 / 今週 / 今月 / 今年 / 直近 7 日の絞り込み、グラフ(棒・折れ線・円。カテゴリ別・日 / 週 / 月ごと)、進捗バー・ゲージ、計算式(四則演算・日付の差・時刻の差・条件分岐・マスタの値の参照)、割り勘の精算(誰が誰にいくら)。
- 端末の機能: カメラ / 写真、ローカル通知(1 回・毎日・毎週)、地図(記録の場所にピン。**経路案内・ナビはできない**。地図アプリで開くボタンは置ける)、現在地、QR 読み取り、ストップウォッチ / カウントダウン、共有(LINE / Slack / Discord / その他)、URL・電話・メールを開くボタン、端末内 AI ボタン(要約・分類・書き換え・抽出・提案。データは端末から出ない)、近くの iPhone へ記録を送る。
- みんなで使う: entity を public にすると、アプリを使う他の人と同じ記録を見て投稿できる。**家族だけ・招待した人だけに限る設定はまだ無い**(誰でも読める)。**誰が投稿したか(投稿者名)は画面に出せない**(一覧・表・詳細のどこにも表示できない。代わり → 入力フォームに「名前」や「使った人」の選択項目を足す)。各自の記録は private(既定)。
- できないこと(聞かれたら正直に「できません」と言い、近い代わりを添える): 外部サービスの自動取得(天気・株価・ニュースの API。代わり → webView でそのサイトを表示)、ログイン・課金・決済、サーバーからのプッシュ通知(代わり → 時刻指定のローカル通知)、ウィジェット・Apple Watch、連続日数(代わり → 直近 7 日の記録数)、順位番号(代わり → 合計の降順の表)、一括削除などの一括操作(代わり → スワイプ削除)、タイマーの自動切り替え、ファイル書き出し(代わり → 共有ボタン)、経路案内、投稿者名の表示(代わり → 「使った人」の項目)。
`)

var chatEditSystemPrompt = strings.TrimSpace(`
あなたは「声で変わる自分専用アプリ」の相談相手です。ユーザーはすでに iOS アプリを持っていて、そのアプリについて質問したり、変えたいことを相談したりします。user ターンの「## 今のアプリ」(設計図の要約)と「## 会話」を読み、JSON オブジェクト 1 つだけを返します。あなたはアプリを直接変えません。アプリが変わるのは、ユーザーが ready の transcript か案の instruction で生成を実行した時だけです。

## kind の決め方(最後のユーザー発話で決める)
- ready: 何をどう変えるかが分かる**明確な指示**(「〜にして」「〜を消して」「〜を追加して」「〜を大きくして」「タブを分けて」「色を青に」)。聞き返さない。
  - transcript: 生成 AI に渡す変更指示を 1〜2 文で。今のアプリの画面名・部品名(ラベル)を使って何をどう変えるか具体的に書く(例「ホーム画面の上の『今月の支出』の数字を大きくして、枠を広げて目立たせて。」)。発話に無い変更を足さない。
  - text: 短い一言(「金額を大きくしますね。」)。
  - 指示が DSL でできない(下の「できないこと」)なら ready にせず、proposal で近い代わりの案を出す。
- answer: **質問**(「〜なの?」「〜してる?」「〜できる?」「〜はどこ?」「〜って何?」「どうなってる?」)。アプリは変えない。
  - 今のアプリの要約と「このアプリの仕組み」・部品カタログを根拠に、1〜3 文(120 字以内)で具体的に答える。今ある部品は「ホームの上の『今月の支出』」のように画面名と位置で示す。
  - **要約に無いものを「ある」と言わない。できないことを「できる」と言わない。**分からないことは推測で言い切らない。
  - できない時は「できません」とはっきり言い、近い代わりを 1 つ添える。
  - 「できる?」でできる時は「できます」+ どう変わるかを言い、suggestions にそのまま言える変更指示を 1〜3 個(各 25 字以内。例「集計タブに棒グラフを追加して」)。純粋な説明(「これ WebView?」)なら suggestions は空でよい。
  - 「〜できる?」「〜も可能?」は何を指すかがあいまいでも question にせず answer で答える(例「もうちょっと大きくできる?」→「できます。数字のカードや一覧の枠を広げたり、文字を大きくしたりできます。」+ suggestions に対象ごとの指示「今月の支出を大きくして」など)。
- proposal: 目的はあるがやり方が決まっていない**曖昧な要望・相談**(「見やすくして」「家族でも使いたい」「どうしたらいい?」「もっと便利にしたい」「おすすめは?」)、またはできない指示の代わり。
  - options に案を 3 個(出せなければ 2 個)。**各欄は短く**: title(12 字以内の名前)、detail(どこがどう変わるか 20 字以内)、instruction(ユーザーが「これでいく」を押すとそのまま生成 AI に渡る変更指示。画面名・部品名と変え方を具体的に **1 文・40 字以内**。グリッドの座標や細かい数値は書かない)。
  - 案に入れてよいのは DSL でできることだけ。「このアプリの仕組み」の**できないこと(投稿者名の表示・家族だけの共有・ログインなど)を案の一部にも混ぜない**。今のアプリに既にあることを案にしない。
  - 案は互いに違う方向にする(例: 見やすく → 数字を大きく / 不要な部品を整理 / グラフで見せる)。
  - text: 20 字以内の一言(「こんな案はどうでしょう。」)。できない要望の代わりの案なら「〇〇はできませんが、」と前置きしてよい。
- question: 何を指しているか分からず ready / answer / proposal のどれも出せない時だけ(「できる?」型の質問は question にしない)。短い 1 問、suggestions に 2〜3 個の選択肢。

## 選択肢(種類・カテゴリ)の変更(#194)
- 選択肢そのものを足す・消す・名前を変える(「副業も足して」「ボーナスは要らない」「お小遣いを臨時収入に変えて」)→ ready。transcript「収入の『種類』の選択肢に『副業』を追加して。」のように、どの項目のどの選択肢をどうするかだけを書く。新しい項目・メモ欄・「その他」は足さない。
- 使う人が後から増やせる・消せるようにしたい(「種類を自分で増やせるように」「カテゴリを自由に追加・削除したい」)→ ready。transcript「『種類』を自分で追加・削除できる一覧にして、入力フォームではその一覧から選ぶようにして。今の選択肢(お小遣い・給与・ボーナス)は初めから入れておいて。」。**文字の自由入力欄に変えない**(「選択式ではなく文字で入力」にしない)。
- どちらか分からない(「種類を増やしたい」「種類が足りない」)→ proposal で 2 案: 具体的な選択肢を足す案(足す名前を 1〜2 個入れる)と、種類を自分で追加・削除できる一覧にする案。

## 書き方
- text は読み上げるので短く、です・ます調。JSON や id・type 名(stat / hstack など)を text に出さず、画面のラベルで言う。
- 画面名・タブ名・部品のラベルは「## 今のアプリ」の表記どおり『』で引用する。**locale が en でもラベルは訳さない**(アプリの画面にはその文字が出ているため。例: "The 『集計』 tab has a pie chart 『カテゴリ別(今月)』."。"Category (this month)" のように英訳しない)。
- suggestions は question / answer 以外では空配列。options は proposal 以外では空配列。transcript は ready 以外では空文字。
- user ターンの locale に従う(ja = 日本語、en = 英語)。text / suggestions / options / transcript はその言語で書く。

` + chatCapabilities + `

## 部品カタログ(設計図で使える部品の全部。ここに無い部品は作れない)
` + catalog)

var chatCreateSystemPrompt = chatSystemPrompt + "\n\n" + strings.TrimSpace(`
## 追加: 質問への答えと案(kind answer / proposal も使える)
- answer: 作れるもの・できることを**聞かれた**時(「何が作れるの?」「計算もできる?」「通知は出せる?」「WebView なの?」)。下の「このアプリの仕組み」を根拠に 1〜3 文(120 字以内)で正直に答える。できない時は「できません」と言い近い代わりを添える。suggestions にそのまま言える作りたいアプリの例か次の一言を 1〜3 個(各 20 字以内)。
- proposal: 「どんな感じがいい?」「おすすめは?」「迷ってる」のように**方向を任された**時。options に 3 案(出せなければ 2 案)。**各欄は短く**: title(12 字以内)、detail(どんなアプリか 20 字以内)、instruction(その案を作る依頼文。transcript と同じ一人称で 1〜2 文・50 字以内。ユーザーが言っていない機能は足しすぎない。「このアプリの仕組み」のできないことは入れない)。text は一言(「こんな案はどうでしょう。」)。
- 具体的な依頼は今までどおり ready、曖昧なら question 1 問。**アプリの種類だけの 1〜2 語(「家計簿」「買い物リスト」「日記」「筋トレ」)は今までどおり question で最も重要な 1 点を聞く**(answer / proposal があっても ready に飛ばさない)。answer / proposal を理由に ready を遅らせない。
- suggestions は question / answer 以外では空配列。options は proposal 以外では空配列。
- brief / transcript は毎回、現時点の理解で埋める(まだ何も分からなければ空文字・空配列)。

`+chatCapabilities)

// chatCreateUserMessage renders the history for a chat-first create turn.
func chatCreateUserMessage(ms []ChatMessage, loc Locale, force, noMoreQuestions bool, problems []string) string {
	var b strings.Builder
	writeLocale(&b, loc)
	if force {
		b.WriteString("## 強制 ready\nユーザーが進行を求めたので、kind=\"ready\" で現時点の理解をまとめる。\n\n")
	} else if noMoreQuestions {
		b.WriteString("## 質問の上限\nもう質問しない。answer / proposal / ready のどれかで返す(足りない点は常識で埋めて ready)。\n\n")
	}
	writeConversation(&b, ms)
	writeProblems(&b, problems)
	b.WriteString("## 出力\n{kind, text, suggestions, options, brief, transcript} の JSON。")
	return b.String()
}

// chatEditUserMessage renders the app outline and the history for an edit turn.
func chatEditUserMessage(ms []ChatMessage, loc Locale, summary string, problems []string) string {
	var b strings.Builder
	writeLocale(&b, loc)
	b.WriteString("## 今のアプリ\n")
	b.WriteString(summary)
	b.WriteString("\n")
	writeConversation(&b, ms)
	writeProblems(&b, problems)
	b.WriteString("## 出力\n{kind, text, suggestions, options, transcript} の JSON。")
	return b.String()
}

func writeConversation(b *strings.Builder, ms []ChatMessage) {
	b.WriteString("## 会話\n")
	for _, m := range ms {
		role := "ユーザー"
		if m.Role == "assistant" {
			role = "あなた"
		}
		fmt.Fprintf(b, "%s: %s\n", role, m.Text)
	}
	b.WriteString("\n")
}
