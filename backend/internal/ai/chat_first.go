package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
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
	MultiSelect bool           `json:"multiSelect"` // #220
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
  "required": ["kind", "text", "suggestions", "multiSelect", "options", "transcript"],
  "properties": {
    "kind": {"type": "string", "enum": ["answer", "question", "proposal", "ready"]},
    "text": {"type": "string"},
    "suggestions": {"type": "array", "items": {"type": "string"}},
    "multiSelect": {"type": "boolean"},
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
  "required": ["kind", "text", "suggestions", "multiSelect", "options", "brief", "transcript"],
  "properties": {
    "kind": {"type": "string", "enum": ["answer", "question", "proposal", "ready"]},
    "text": {"type": "string"},
    "suggestions": {"type": "array", "items": {"type": "string"}},
    "multiSelect": {"type": "boolean"},
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
		if err == nil && !restrictionFrom(ctx).NoSharedData() { // #451 G2 / #450 G1
			defaultPaidCatalog.annotate(out, app)
		}
	}()
	store, key := s.conversations(), convKey(auth.UID(ctx), req.AppID)
	store.wait(ctx, key, s.chatTimeout())
	cv := store.get(key)
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
			plainChatResult(res) // #258
			cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
			store.put(key, cv)
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
			pending := store.begin(key)
			store.put(key, cv)
			go func() {
				a := <-done
				store.finish(key, pending, cv, func(c *conversation) {
					if a.reply != nil {
						b := cleanBrief(a.reply.Brief)
						c.brief, c.transcript = &b, transcriptOr(a.reply.Transcript, userTexts)
					}
				})
			}()
		} else {
			store.put(key, cv)
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
	guardPublicPII(res, cv.messages, app != nil && anyShared(app), loc) // #253
	if mode == chatModeCreate {
		b := cleanBrief(att.reply.Brief)
		cv.brief, cv.transcript = &b, transcriptOr(att.reply.Transcript, userTexts)
	}
	if res.Kind == "question" {
		cv.questions++
	}
	cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: historyText(res)})
	store.put(key, cv)
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
		res.MultiSelect = r.MultiSelect && len(opts) > 1 // #220
	default: // question | answer
		res.Suggestions = cleanSuggestions(r.Suggestions)
		res.MultiSelect = r.MultiSelect && kind == "question" && len(res.Suggestions) > 1 // #220
	}
	res.Text = text
	if res.Source == "" {
		res.Source = "claude"
	}
	plainChatResult(res) // #258
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
	if f["multiSelect"] != nil {
		_ = json.Unmarshal(f["multiSelect"], &r.MultiSelect) // #220
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
		fmt.Fprintf(&b, "- %s「%s」(scope %s・項目 %d/%d)\n", e.ID, e.Name, scope, len(e.Fields), dsl.MaxFieldsPerEntity)
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

// chatCapabilities is the hand-written part of what the DSL can and cannot
// do: how the app works and its limits (#230: the list of parts, ops and
// actions is not written here; it comes from the schema, see
// chatCapabilityIndex / catalog, so it never falls behind the generator).
var chatCapabilities = strings.TrimSpace(`
## このアプリの仕組み(質問への答えの根拠)
- アプリは iOS の SwiftUI でネイティブに描画している(Server-Driven UI: 画面の設計図 JSON を AI が作り、iOS がそれを部品に組み立てる)。**WebView ではない**。webView 部品がある画面のその枠だけ WKWebView で https のページを表示する。
- 変えられる単位: 画面(最大 12)・下のタブ(2〜5)・記録の種類 entity(最大 6、項目は 40 まで)・部品・計算・通知・テーマ色・アイコン・アプリ名。声やチャットで頼むと AI が設計図を書き換える。
- 項目数の上限(#221): 今のアプリの entity 行の「項目 n/40」に足すと 40 を超える追加は ready にしない(生成 AI が既存の項目を消して収めてしまう)。proposal で 2 案: どれかの項目を消して足す案(消す項目名を入れる)と、足す項目を別の記録の種類(画面)に分ける案。
- 部品の大きさ・位置: 各画面は横 A〜F の 6 列 × 縦 12 行のグリッドで、部品の枠(area)を広げる・狭める・動かすことができる。文字の大きさは text の style(title / headline / body / caption の 4 段)。画像の高さは small / medium / large。色はテーマ色と決まった色名(semantic color)だけ。
- できる・できないの決め方: 部品・計算式の op・ボタンの動作・項目の型は「できることの索引」と「部品カタログ」(どちらもスキーマから自動生成)が正典。そこにある部品や op で組み立てられることは「できます」と答える(例: 連続日数 = streak、直近 n 日にやった日数 = doneInDays、動画 = videoField、歩数 = healthStat、場所の検索 = locationPicker、道のりの記録 = locationTracker)。毎日の通知はアプリの通知設定(1 回・毎日・毎週のローカル通知)。
- みんなで使う: entity を public(ユーザーには「みんなに公開」と言う)にすると、アプリを使う他の人と同じ記録を見て投稿できる(誰でも読める)。誰が投稿したか(投稿者名)は一覧・表・タイムラインに出せる($author: 各自がアプリの設定で決めた表示名・未設定は「匿名」。名前の入力欄は要らない)。「自分の投稿だけ」のタブや件数も作れる。家族・サークル・チームなど決まった人だけで使うなら shared(ユーザーには「参加した人だけ」と言う。作った人とシェアリンクで参加した人だけが見て投稿できる。「家族だけ」「招待した人だけ」はこれでできる)。各自の記録は private(「自分だけ」・既定)。**みんなに公開・参加した人だけの記録は、既定では作った人だけが変更・削除できる**。家事の完了・出欠の〇×・買い物リストのチェックのように他の人も同じ記録を変えたい時は「参加者みんなが変更できる」にできる(削除は作った人だけ)。「店長だけが確定」のような役割・権限は今はできない。
` + chatLimitsText())

// chatPlainWords (#258): the user is not an engineer; the catalog names below
// are for reasoning only (plainTerms in chat_terms.go is the safety net).
var chatPlainWords = strings.TrimSpace(`
## 言葉づかい(#258・text / suggestions / 案の title・detail・instruction / transcript に必ず適用)
- 使う人はエンジニアではない。**設計図の英語の名前(scope 名・型名・部品の type 名・計算式の op 名・entity / field / computed など)を一切書かない**。括弧で添える(「折りたたみ(disclosure)」)のも不可。画面に見える言葉と普通の日本語で言う。
- 言い換え: public → みんなに公開 / private → 自分だけ / shared → 参加した人だけ / entity → 記録の種類 / field → 項目 / computed・op → 計算 / disclosure → 折りたたみ / stat → 数字のカード / list → 一覧 / ref → ほかの記録から選ぶ項目 / notifications → 通知。
- 例: ×「public の記録で投稿が流れる掲示板」→ ○「みんなに公開の記録で、投稿が流れる掲示板」。×「支出を public にして」→ ○「支出をみんなに公開して」。×「乱数の計算式(op)は無いので」→ ○「ランダムに選ぶ計算は無いので」。
- locale en も同じ(record type / item / calculation / everyone can see / only me。"entity" "field" "op" "scope" と書かない)。案の instruction と transcript も生成中の画面に表示されるので同じ(「支出をみんなに公開して」のように画面の言葉で書けば生成 AI に伝わる)。
`)

var chatEditSystemPrompt = strings.TrimSpace(`
あなたは「声で変わる自分専用アプリ」の相談相手です。ユーザーはすでに iOS アプリを持っていて、そのアプリについて質問したり、変えたいことを相談したりします。user ターンの「## 今のアプリ」(設計図の要約)と「## 会話」を読み、JSON オブジェクト 1 つだけを返します。あなたはアプリを直接変えません。アプリが変わるのは、ユーザーが ready の transcript か案の instruction で生成を実行した時だけです。

## kind の決め方(最後のユーザー発話で決める)
- ready: 何をどう変えるかが分かる**明確な指示**(「〜にして」「〜を消して」「〜を追加して」「〜を大きくして」「タブを分けて」「色を青に」)。聞き返さない。ただし「グラフを足して」「項目を足して」「通知して」のように**足す物の中身(何の数字・何の項目・いつ)が決まらない**ものは下の question。直前にあなたが聞き返し、ユーザーが答えた(チップを押した)なら ready。
  - transcript: 生成 AI に渡す変更指示を 1〜2 文で。今のアプリの画面名・部品名(ラベル)を使って何をどう変えるか具体的に書く(例「ホーム画面の上の『今月の支出』の数字を大きくして、枠を広げて目立たせて。」)。発話に無い変更を足さない。
  - text: 短い一言(「金額を大きくしますね。」)。
  - 指示が DSL でできない(下の「できないこと」)なら ready にせず、proposal で近い代わりの案を出す。
- answer: **質問**(「〜なの?」「〜してる?」「〜できる?」「〜はどこ?」「〜って何?」「どうなってる?」)。アプリは変えない。
  - 今のアプリの要約と「このアプリの仕組み」・部品カタログを根拠に、1〜3 文(120 字以内)で具体的に答える。今ある部品は「ホームの上の『今月の支出』」のように画面名と位置で示す。
  - **要約に無いものを「ある」と言わない。できないことを「できる」と言わない。**分からないことは推測で言い切らない。
  - できない時は「できません」とはっきり言い、近い代わりを 1 つ添える。
  - 「できる?」でできる時は「できます」+ どう変わるかを言い、suggestions にそのまま言える変更指示を 1〜3 個(各 25 字以内。例「集計タブに棒グラフを追加して」)。純粋な説明(「これ WebView?」)なら suggestions は空でよい。
  - 「〜できる?」「〜も可能?」は何を指すかがあいまいでも question にせず answer で答える(例「もうちょっと大きくできる?」→「できます。数字のカードや一覧の枠を広げたり、文字を大きくしたりできます。」+ suggestions に対象ごとの指示「今月の支出を大きくして」など)。
- proposal: 目的はあるがやり方が決まっていない**曖昧な要望・相談**(「見やすくして」「家族でも使いたい」「どうしたらいい?」「もっと便利にしたい」「おすすめは?」)、またはできない指示の代わり。**何を足すか(グラフ・項目・通知など)は決まっていて中身だけが足りない依頼は proposal ではなく下の question**。
  - options に案を 3 個(出せなければ 2 個)。**各欄は短く**: title(12 字以内の名前)、detail(どこがどう変わるか 20 字以内)、instruction(ユーザーが「これでいく」を押すとそのまま生成 AI に渡る変更指示。画面名・部品名と変え方を具体的に **1 文・40 字以内**。グリッドの座標や細かい数値は書かない)。
  - 案に入れてよいのは DSL でできることだけ。「このアプリの仕組み」の**できないこと(ログインなど)を案の一部にも混ぜない**。今のアプリに既にあることを案にしない。
  - 案は互いに違う方向にする(例: 見やすく → 数字を大きく / 不要な部品を整理 / グラフで見せる)。
  - text: 20 字以内の一言(「こんな案はどうでしょう。」)。できない要望の代わりの案なら「〇〇はできませんが、」と前置きしてよい。
- question: **何を作る・足すかは分かるが、作るのに要る中身が決まっていない**依頼(#211)。そのまま生成すると AI が中身を勝手に決めるか、何も変わらずに終わるもの。
  - 例:「グラフを足して」(何の数字を・どう分けて並べるか)、「項目を足して」(何の項目か)、「通知して」(いつ・何を知らせるか)、「並べ替えて」(何の順か)、「色を変えて」(どこを何色に)、「何か消して」(どれを)。
  - text は**何が足りないか**と**なぜそれが要るか**を 1〜2 文(70 字以内)で:「グラフを足すには、何の数字を(金額・件数)、どう分けて(日ごと・カテゴリごと)並べるかを決めてください。」「通知を作るには、いつ(毎日・毎週)何を知らせるかが要ります。何時に知らせますか?」
  - suggestions に**押すだけで答えになる**選択肢を 2〜3 個(各 20 字以内)。**「## 今のアプリ」の entity の項目(field の name)・画面名から作る**(例: 家計簿に『金額』『カテゴリ』『日付』があれば「金額をカテゴリごとに」「金額を日ごとに」「件数を月ごとに」)。今のアプリに無い項目名を作らない。一般論(「おまかせ」「その他」)にしない。
  - 聞くのは 1 回に 1 点(足りない点が 2 つなら 1 つの選択肢に両方入れる。上の「金額をカテゴリごとに」のように)。答えをもらったら、残りは常識で埋めて **ready** にする(同じ依頼で 2 回続けて聞かない)。
  - **足りているものは聞かない**: 「金額を大きくして」「メモ欄を消して」「カテゴリに外食を足して」「集計タブに月ごとの金額の棒グラフを足して」のように何をどう変えるか分かる指示は今まで通り ready。対象がアプリに 1 つしか無い(数字のカードが 1 つだけ等)なら聞かずに ready。
  - 目的だけの相談(「見やすくして」「もっと便利にしたい」)は question にせず proposal で案を出す。

## 複数選択(multiSelect、#220)
- proposal: 案を**いくつ選んでも同時に反映できる**(足す系で互いに邪魔しない。例「グラフを足す」「予算バーを足す」「並べ替えを足す」)なら multiSelect=true。**どれか 1 つしか選べない**(同じ所を別の形に変える・両立しない。例「レイアウト A / B」「色を青に / 赤に」「数字を大きく / 小さく」)なら false。迷ったら false。
- question: suggestions が**複数当てはまる**問い(「どの項目を一覧に出しますか?」「何を記録しますか?」)なら multiSelect=true。**1 つだけ選ぶ**問い(「どの単位で集計しますか?」「毎日と毎週どちら?」)なら false。
- answer / ready では常に false。

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

` + chatPlainWords + `

## 部品カタログ(設計図で使える部品の全部。ここに無い部品は作れない)
` + catalog)

var chatCreateSystemPrompt = chatSystemPrompt + "\n\n" + strings.TrimSpace(`
## 追加: 質問への答えと案(kind answer / proposal も使える)
- answer: 作れるもの・できることを**聞かれた**時(「何が作れるの?」「計算もできる?」「通知は出せる?」「WebView なの?」)。下の「このアプリの仕組み」を根拠に 1〜3 文(120 字以内)で正直に答える。できない時は「できません」と言い近い代わりを添える。suggestions にそのまま言える作りたいアプリの例か次の一言を 1〜3 個(各 20 字以内)。
- proposal: 「どんな感じがいい?」「おすすめは?」「迷ってる」のように**方向を任された**時。options に 3 案(出せなければ 2 案)。**各欄は短く**: title(12 字以内)、detail(どんなアプリか 20 字以内)、instruction(その案を作る依頼文。transcript と同じ一人称で 1〜2 文・50 字以内。ユーザーが言っていない機能は足しすぎない。「このアプリの仕組み」のできないことは入れない)。text は一言(「こんな案はどうでしょう。」)。
- 具体的な依頼は今までどおり ready、曖昧なら question 1 問。**アプリの種類だけの 1〜2 語(「家計簿」「買い物リスト」「日記」「筋トレ」)は今までどおり question で最も重要な 1 点を聞く**(answer / proposal があっても ready に飛ばさない)。answer / proposal を理由に ready を遅らせない。
- suggestions は question / answer 以外では空配列。options は proposal 以外では空配列。
- brief / transcript は毎回、現時点の理解で埋める(まだ何も分からなければ空文字・空配列)。

## 複数選択(multiSelect、#220)
- proposal: 案を**いくつ選んでも同時に反映できる**(足す系で互いに邪魔しない。例「グラフを足す」「予算バーを足す」「並べ替えを足す」)なら multiSelect=true。**どれか 1 つしか選べない**(同じ所を別の形に変える・両立しない。例「レイアウト A / B」「色を青に / 赤に」「数字を大きく / 小さく」)なら false。迷ったら false。
- question: suggestions が**複数当てはまる**問い(「どの項目を一覧に出しますか?」「何を記録しますか?」)なら multiSelect=true。**1 つだけ選ぶ**問い(「どの単位で集計しますか?」「毎日と毎週どちら?」)なら false。
- answer / ready では常に false。

`+chatCapabilities+"\n\n"+chatPlainWords+"\n\n"+chatCapabilityIndex)

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
	b.WriteString("## 出力\n{kind, text, suggestions, multiSelect, options, brief, transcript} の JSON。")
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
	b.WriteString("## 出力\n{kind, text, suggestions, multiSelect, options, transcript} の JSON。")
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
