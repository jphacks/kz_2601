package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Clarify phase (Issue #74): a few short question/answer turns before the
// first generation. POST /v1/chat returns either one question with
// suggestions or `ready` with a brief and a transcript that iOS passes to the
// existing POST /v1/generate (mode auto/create). Conversation state lives in
// this process (map[appId], last 10 messages, 30 min TTL); persistence in
// appconfig is deferred.

const (
	chatMaxTurns     = 3  // Claude may ask on turns 1..3; turn 4 is forced ready
	chatMaxMessages  = 10 // messages kept per conversation
	chatTTL          = 30 * time.Minute
	chatMaxTokens    = 1024
	chatMaxSuggest   = 3
	chatReadyMaxJA   = 12 // runes: 「とりあえずこのまま作って」
	chatReadyMaxEN   = 32 // runes: "ok that's fine, just build it"
	chatMaxUserChars = 2000
)

// ChatMessage is one turn of the clarify conversation.
type ChatMessage struct {
	Role string `json:"role"` // user|assistant
	Text string `json:"text"`
}

// ChatRequest is POST /v1/chat. Messages may be the full history or only the
// newest user message; the server merges with its stored conversation.
type ChatRequest struct {
	AppID    string        `json:"appId"`
	Messages []ChatMessage `json:"messages"`
	Locale   Locale        `json:"locale,omitempty"`
}

// Brief is the structured understanding of the requested app.
type Brief struct {
	Name     string   `json:"name"`
	Purpose  string   `json:"purpose"`
	Entities []string `json:"entities"`
	Features []string `json:"features"`
	Native   []string `json:"native"` // camera|notification|calendar|map|share|location|...
}

// ChatResult is the 200 body of POST /v1/chat.
type ChatResult struct {
	Kind        string   `json:"kind"` // question|ready
	Text        string   `json:"text"`
	Suggestions []string `json:"suggestions,omitempty"`
	Turn        int      `json:"turn"` // 1-based count of user messages so far
	Brief       *Brief   `json:"brief,omitempty"`
	Transcript  string   `json:"transcript,omitempty"` // ready: the utterance for POST /v1/generate
	Locale      Locale   `json:"locale"`
	Source      string   `json:"source"` // claude|shortcut|forced
	LatencyMs   Latency  `json:"latencyMs"`
	Meta        Meta     `json:"meta"`
}

// chatReply is the structured output Claude returns.
type chatReply struct {
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	Suggestions []string `json:"suggestions"`
	Brief       Brief    `json:"brief"`
	Transcript  string   `json:"transcript"`
}

// ChatSchema is the (small) structured-output schema for the clarify turn.
// Every property is required and objects are closed, as the API's grammar
// compiler expects.
func ChatSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["kind", "text", "suggestions", "brief", "transcript"],
  "properties": {
    "kind": {"type": "string", "enum": ["question", "ready"]},
    "text": {"type": "string"},
    "suggestions": {"type": "array", "items": {"type": "string"}},
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

// conversation is the per-app clarify state.
type conversation struct {
	messages   []ChatMessage
	brief      *Brief
	transcript string
	updated    time.Time
}

// convStore is the in-process conversation map with lazy TTL sweeping.
type convStore struct {
	mu   sync.Mutex
	m    map[string]*conversation
	now  func() time.Time
	ttl  time.Duration
	keep int
}

func newConvStore() *convStore {
	return &convStore{m: map[string]*conversation{}, now: time.Now, ttl: chatTTL, keep: chatMaxMessages}
}

func (c *convStore) get(appID string) *conversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweep()
	if cv, ok := c.m[appID]; ok {
		cp := *cv
		cp.messages = append([]ChatMessage(nil), cv.messages...)
		return &cp
	}
	return nil
}

func (c *convStore) put(appID string, cv *conversation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := len(cv.messages); n > c.keep {
		cv.messages = append([]ChatMessage(nil), cv.messages[n-c.keep:]...)
	}
	cv.updated = c.now()
	c.m[appID] = cv
}

func (c *convStore) delete(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, appID)
}

// sweep drops expired conversations; called with mu held.
func (c *convStore) sweep() {
	cutoff := c.now().Add(-c.ttl)
	for id, cv := range c.m {
		if cv.updated.Before(cutoff) {
			delete(c.m, id)
		}
	}
}

func (s *Service) conversations() *convStore {
	s.convOnce.Do(func() {
		if s.conv == nil {
			s.conv = newConvStore()
		}
	})
	return s.conv
}

// ClearChat forgets the conversation for appID (DELETE /v1/chat/{appId}).
func (s *Service) ClearChat(appID string) { s.conversations().delete(appID) }

// Chat runs one clarify turn.
func (s *Service) Chat(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	started := time.Now()
	if req.AppID == "" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "appId is required"}
	}
	incoming, err := normalizeMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	store := s.conversations()
	cv := store.get(req.AppID)
	if cv == nil {
		cv = &conversation{}
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

	res := &ChatResult{Turn: turn, Locale: loc, Meta: Meta{Model: s.Model}}

	// Shortcut: 「もう作って」/ "just build it" after at least one Claude turn
	// -> ready without Claude, reusing the brief drafted on that turn. A
	// go-ahead as the very first utterance (「お任せで家計簿」) still needs
	// Claude once, so it falls through as a forced ready.
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

	force := turn > chatMaxTurns || goAhead
	ar := &attemptResult{}
	var reply *chatReply
	var problems []string
	for i := 0; i < 2; i++ {
		resp, cerr := s.callChat(ctx, chatUserMessage(cv.messages, loc, turn, force, problems), ar)
		if cerr != nil {
			return nil, cerr
		}
		var r chatReply
		if uerr := json.Unmarshal(extractJSON(resp.Text), &r); uerr != nil {
			problems = []string{"output is not valid JSON for the schema: " + uerr.Error()}
			continue
		}
		if r.Kind != "question" && r.Kind != "ready" {
			problems = []string{`kind must be "question" or "ready"`}
			continue
		}
		reply = &r
		break
	}
	if reply == nil {
		return nil, &Error{Status: http.StatusBadGateway, Code: "generation_failed",
			Message: "could not produce a clarify turn", Problems: problems, Attempts: ar.meta.Attempts}
	}
	res.Meta = ar.meta
	res.Source = "claude"
	brief := cleanBrief(reply.Brief)
	transcript := strings.TrimSpace(reply.Transcript)
	if transcript == "" {
		transcript = strings.Join(userTexts, "。")
	}
	cv.brief, cv.transcript = &brief, transcript

	if reply.Kind == "ready" || force {
		res.Kind = "ready"
		if force && reply.Kind != "ready" {
			res.Source = "forced"
		}
		res.Text = strings.TrimSpace(reply.Text)
		if res.Text == "" || reply.Kind != "ready" {
			res.Text = loc.text().readyText
		}
		res.Brief = &brief
		res.Transcript = transcript
	} else {
		res.Kind = "question"
		res.Text = strings.TrimSpace(reply.Text)
		res.Suggestions = cleanSuggestions(reply.Suggestions)
	}
	cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
	store.put(req.AppID, cv)
	res.LatencyMs = Latency{Claude: ar.latency.Milliseconds(), Total: time.Since(started).Milliseconds()}
	return res, nil
}

// callChat is call() with the small schema sent as output_config.format.
func (s *Service) callChat(ctx context.Context, user string, ar *attemptResult) (*Response, error) {
	timeout := s.CallTimeout
	if timeout == 0 {
		timeout = 40 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.Now()
	resp, err := s.Claude.Complete(cctx, Completion{
		System:     chatSystemPrompt,
		Messages:   []Message{{Role: "user", Content: user}},
		Schema:     ChatSchema(),
		Structured: true,
		MaxTokens:  chatMaxTokens,
	})
	ar.latency += time.Since(t)
	ar.meta.Model = s.Model
	ar.meta.Attempts++
	ar.meta.Regenerations = ar.meta.Attempts - 1
	if err != nil {
		return nil, mapClaudeErr(err)
	}
	ar.meta.Usage.Add(resp.Usage)
	if resp.StopReason == "refusal" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "refused", Message: "the model declined this request", Attempts: ar.meta.Attempts}
	}
	return resp, nil
}

// normalizeMessages validates roles, trims text and drops empty turns.
func normalizeMessages(in []ChatMessage) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(in))
	for _, m := range in {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		text := strings.TrimSpace(m.Text)
		if role != "user" && role != "assistant" {
			return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "message role must be user or assistant"}
		}
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > chatMaxUserChars {
			text = string(r[:chatMaxUserChars])
		}
		out = append(out, ChatMessage{Role: role, Text: text})
	}
	if len(out) == 0 {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "messages must contain at least one user message"}
	}
	return out, nil
}

// mergeHistory combines the stored conversation with the request: a request
// that carries more than one message is treated as the full history (it
// replaces the stored one); a single message is appended to the stored one.
func mergeHistory(stored, incoming []ChatMessage) []ChatMessage {
	if len(incoming) > 1 || len(stored) == 0 {
		return squash(incoming)
	}
	return squash(append(stored, incoming...))
}

// squash joins consecutive same-role turns so the transcript alternates.
func squash(ms []ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(ms))
	for _, m := range ms {
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Text += "\n" + m.Text
			continue
		}
		out = append(out, m)
	}
	return out
}

func cleanBrief(b Brief) Brief {
	b.Name = strings.TrimSpace(b.Name)
	b.Purpose = strings.TrimSpace(b.Purpose)
	b.Entities = cleanList(b.Entities, 0)
	b.Features = cleanList(b.Features, 0)
	nat := cleanList(b.Native, 0)
	for i := range nat {
		nat[i] = strings.ToLower(nat[i])
	}
	nat = cleanList(nat, 0)
	if len(nat) == 1 && nat[0] == "none" {
		nat = []string{}
	}
	b.Native = nat
	return b
}

func cleanSuggestions(in []string) []string { return cleanList(in, chatMaxSuggest) }

// cleanList trims, dedupes (keeping order) and caps; never returns nil so
// the JSON stays an array.
func cleanList(in []string, limit int) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

// buildNowPhrases end the clarify phase without asking Claude. Matched by
// substring on a short utterance only (ja ≤ 14 runes, en ≤ 32), so 「家計簿
// アプリを作って」 (a request, not a go-ahead) never matches.
var buildNowPhrases = []string{
	"もう作って", "もうつくって", "作っていい", "作ってください", "作ってよ", "早く作って", "とりあえず作って", "このまま作って", "それで作って",
	"それでいい", "それでok", "それで大丈夫", "それでお願い", "これでいい", "これでお願い", "もういい", "いいから作", "はい作って", "うん作って",
	"お任せ", "おまかせ", "任せる", "まかせる", "任せた", "よろしく",
	// en (spaces and punctuation are stripped before matching)
	"justmakeit", "justbuildit", "buildit", "makeitnow", "goahead", "that'sfine", "thatsfine", "that'sit", "thatsit",
	"uptoyou", "youdecide", "looksgood", "soundsgood", "goodenough", "okgo", "okaygo", "let'sgo", "letsgo", "doit",
}

// wantsBuildNow reports whether the utterance is a go-ahead.
func wantsBuildNow(t string) bool {
	s := strings.ToLower(strings.TrimSpace(t))
	limit := chatReadyMaxJA
	if !hasJapanese(s) {
		limit = chatReadyMaxEN
	}
	if s == "" || len([]rune(s)) > limit {
		return false
	}
	// 「はい、作って！」 -> はい作って: punctuation and spaces do not matter.
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("。．.!！?？、,…　 ", r) {
			return -1
		}
		return r
	}, s)
	for _, p := range buildNowPhrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// chatSystemPrompt is static (prompt-cacheable). The locale, the turn
// counter and the transcript travel in the user turn.
var chatSystemPrompt = strings.TrimSpace(`
あなたは「声で作る自分専用アプリ」の要件ヒアリング担当です。ユーザーは iOS 上で小さな記録アプリ(家計簿・筋トレ記録・日記・買い物リストなど)を声で作ろうとしています。会話履歴を読み、JSON オブジェクト 1 つだけを返します。

## 判定
- 要件が十分なら kind="ready"。十分 = 次の 4 点が推定できること: (1) 目的 (2) 記録する項目 (3) 見たい一覧・集計 (4) 必要なネイティブ機能(camera / notification / calendar / map / location / share、無ければ none)。常識で埋められる点は聞かずに埋める(例: 家計簿なら金額・カテゴリ・日付、合計と一覧)。
- 1 語や漠然とした発話で、聞かないとアプリが大きく変わる点があれば kind="question"。質問は**最も重要な 1 点だけ**・20 字程度の短い一文・はい/いいえか選択肢で答えられる形。suggestions に 2〜3 個の短い選択肢(各 10 字以内)を入れる。同じことを二度聞かない。
- 「## 強制 ready」がある時、またはユーザーが「もう作って」「お任せ」のように進行を求めた時は必ず kind="ready"。

## 毎回埋めるもの
- brief: 現時点の理解。name(アプリ名)、purpose(一文)、entities(記録する項目名)、features(見たい一覧・集計・ボタンなど)、native(上記キーワードの英小文字配列。無ければ空配列)。
- transcript: 要件を 1 段落にまとめた、アプリ生成 AI に渡す一人称の依頼文。「家計簿アプリを作って。金額とカテゴリと日付を記録して、今月の合計と支出一覧を見たい。レシートの写真も残したい。」のように書く。**ユーザーが言っていない機能(通知・グラフ・写真など)は足さない**。question の時も現時点の理解で書く。
- text: question なら質問文そのもの。ready なら短い一言(ja「では作りますね。」/ en "Got it, I'll build it now.")。

## 言語
user ターンの locale に従う(ja = 日本語、en = 英語)。text / suggestions / brief.name / purpose / entities / features / transcript はその言語で書く。brief.native だけは常に英小文字キーワード。
`)

// chatUserMessage renders the history for one clarify call.
func chatUserMessage(ms []ChatMessage, loc Locale, turn int, force bool, problems []string) string {
	var b strings.Builder
	writeLocale(&b, loc)
	fmt.Fprintf(&b, "## ラリー\n%d / %d\n\n", min(turn, chatMaxTurns+1), chatMaxTurns)
	if force {
		b.WriteString("## 強制 ready\n上限に達したので、これ以上質問せず kind=\"ready\" で現時点の理解をまとめる。\n\n")
	}
	b.WriteString("## 会話\n")
	for _, m := range ms {
		role := "ユーザー"
		if m.Role == "assistant" {
			role = "あなた"
		}
		fmt.Fprintf(&b, "%s: %s\n", role, m.Text)
	}
	b.WriteString("\n")
	writeProblems(&b, problems)
	b.WriteString("## 出力\n{kind, text, suggestions, brief, transcript} の JSON。")
	return b.String()
}
