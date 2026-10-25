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

// ChatConfig is the model setup of the clarify turn, separate from
// generation (env AI_CHAT_MODEL / AI_CHAT_EFFORT / AI_CHAT_THINKING). Empty
// fields fall back to the chat defaults below, not to the generate model.
type ChatConfig struct {
	Model    string
	Effort   string
	Thinking string // adaptive|disabled|between_tools
}

// Chat defaults (Issue #147 eval, docs/backend/ai-eval/2026-10-24-chat-latency.md).
const (
	DefaultChatModel    = "claude-sonnet-5-5"
	DefaultChatEffort   = "low"
	DefaultChatThinking = ""
)

func (c ChatConfig) resolved() ChatConfig {
	if c.Model == "" {
		c.Model = DefaultChatModel
	}
	if c.Effort == "" {
		c.Effort = DefaultChatEffort
	}
	if c.Thinking == "" {
		c.Thinking = DefaultChatThinking
	}
	return c
}

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
	// Mode (#185): create|edit; empty = edit when appId already has a
	// generated DSL. Sending Mode (or Capabilities) opts in to the chat-first
	// kinds answer / proposal.
	Mode string `json:"mode,omitempty"`
	// Capabilities lists the reply kinds the client can show beyond
	// question / ready ("answer", "proposal"). Old clients send neither Mode
	// nor Capabilities and get the clarify behaviour unchanged.
	Capabilities []string `json:"capabilities,omitempty"`
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
	Kind        string   `json:"kind"` // question|ready|answer|proposal
	Text        string   `json:"text"`
	Suggestions []string `json:"suggestions,omitempty"`
	// Options: proposal only, 1..3 ideas; Instruction goes to POST /v1/generate.
	Options []ChatOption `json:"options,omitempty"`
	// MultiSelect (#220): proposal options / question suggestions can be
	// combined (pick several). Omitted = pick one (old iOS behaviour).
	MultiSelect bool `json:"multiSelect,omitempty"`
	// Mode: create|edit, set only for chat-first requests (#185).
	Mode       string  `json:"mode,omitempty"`
	Turn       int     `json:"turn"` // 1-based count of user messages so far
	Brief      *Brief  `json:"brief,omitempty"`
	Transcript string  `json:"transcript,omitempty"` // ready: the utterance for POST /v1/generate
	Locale     Locale  `json:"locale"`
	Source     string  `json:"source"` // claude|shortcut|forced
	LatencyMs  Latency `json:"latencyMs"`
	// FirstTokenMs: time from the last Claude request to its first output
	// token (diagnostics; 0 for shortcut turns).
	FirstTokenMs int64 `json:"firstTokenMs,omitempty"`
	Meta         Meta  `json:"meta"`
	// RequiresPro (#77): ready whose transcript needs Tailor It Pro
	// (paid_features.json). FreeTranscript = the transcript without those
	// features, for 「この機能なしで作る」. Advisory only (no server gate).
	RequiresPro    bool          `json:"requiresPro,omitempty"`
	PaidFeatures   []PaidFeature `json:"paidFeatures,omitempty"`
	FreeTranscript string        `json:"freeTranscript,omitempty"`
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
	mode       string // chat-first: create|edit ("" = legacy clarify)
	questions  int    // chat-first create: questions asked so far
}

// convStore is the in-process conversation map with lazy TTL sweeping.
type convStore struct {
	mu   sync.Mutex
	m    map[string]*conversation
	now  func() time.Time
	ttl  time.Duration
	keep int
	// pending: an early-returned question whose brief/transcript are still
	// being generated (closed by finish).
	pending map[string]chan struct{}
}

func newConvStore() *convStore {
	return &convStore{m: map[string]*conversation{}, now: time.Now, ttl: chatTTL, keep: chatMaxMessages,
		pending: map[string]chan struct{}{}}
}

// begin marks appID as having a background update in flight.
func (c *convStore) begin(appID string) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan struct{})
	c.pending[appID] = ch
	return ch
}

// finish applies fn to the stored conversation if it is still cv (not
// cleared or replaced), then releases waiters.
func (c *convStore) finish(appID string, ch chan struct{}, cv *conversation, fn func(*conversation)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m[appID] == cv {
		fn(cv)
	}
	if c.pending[appID] == ch {
		delete(c.pending, appID)
	}
	close(ch)
}

// wait blocks until the background update for appID (if any) has finished,
// the context ends or max elapses.
func (c *convStore) wait(ctx context.Context, appID string, max time.Duration) {
	c.mu.Lock()
	ch := c.pending[appID]
	c.mu.Unlock()
	if ch == nil {
		return
	}
	t := time.NewTimer(max)
	defer t.Stop()
	select {
	case <-ch:
	case <-ctx.Done():
	case <-t.C:
	}
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
//
// Latency (#147): a question needs only kind/text/suggestions, which come
// first in ChatSchema. The first Claude call streams, and as soon as those
// three fields are complete the question is returned; brief/transcript (the
// material for a later shortcut ready) finish in the background and are
// stored then. The next turn on the same appId waits for that (the user is
// still answering, so in practice it is already done).
func (s *Service) Chat(ctx context.Context, req ChatRequest) (out *ChatResult, err error) {
	started := time.Now()
	if req.AppID == "" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "appId is required"}
	}
	incoming, err := normalizeMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	if wantsChatFirst(req) {
		return s.chatFirst(ctx, req, incoming, started)
	}
	// #77: ready は transcript から有料機能を判定する(旧 clarify は create のみ)
	defer func() {
		if err == nil {
			defaultPaidCatalog.annotate(out, nil)
		}
	}()
	store := s.conversations()
	store.wait(ctx, req.AppID, s.chatTimeout())
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

	cfg := s.ChatConfig.resolved()
	res := &ChatResult{Turn: turn, Locale: loc, Meta: Meta{Model: cfg.Model}}

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
		plainChatResult(res) // #258
		cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
		store.put(req.AppID, cv)
		res.LatencyMs = Latency{Total: time.Since(started).Milliseconds()}
		return res, nil
	}

	force := turn > chatMaxTurns || goAhead
	ar := &attemptResult{}
	var att chatAttempt
	if !force {
		// First attempt in the background; an early question wins the race.
		bgAR := &attemptResult{}
		questions := make(chan earlyQuestion, 1)
		done := make(chan chatAttempt, 1)
		bctx := context.WithoutCancel(ctx)
		prompt := chatUserMessage(cv.messages, loc, turn, false, nil)
		callStart := time.Now()
		go func() {
			done <- s.chatAttempt(bctx, cfg, prompt, bgAR, func(q earlyQuestion) { questions <- q })
		}()
		select {
		case q := <-questions:
			res.Kind, res.Source = "question", "claude"
			res.Text = strings.TrimSpace(q.reply.Text)
			res.Suggestions = cleanSuggestions(q.reply.Suggestions)
			plainChatResult(res) // #258
			res.FirstTokenMs = q.ttft.Milliseconds()
			res.Meta = Meta{Model: cfg.Model, Attempts: 1}
			cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
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
			res.LatencyMs = Latency{Claude: time.Since(callStart).Milliseconds(), Total: time.Since(started).Milliseconds()}
			return res, nil
		case att = <-done:
			*ar = *bgAR
		case <-ctx.Done():
			return nil, &Error{Status: http.StatusGatewayTimeout, Code: "timeout", Message: "clarify turn cancelled"}
		}
	} else {
		att = s.chatAttempt(ctx, cfg, chatUserMessage(cv.messages, loc, turn, true, nil), ar, nil)
	}
	if att.err != nil {
		return nil, att.err
	}
	if att.reply == nil {
		// one retry with the problems of the first output
		att2 := s.chatAttempt(ctx, cfg, chatUserMessage(cv.messages, loc, turn, force, att.problems), ar, nil)
		if att2.err != nil {
			return nil, att2.err
		}
		if att2.reply == nil {
			return nil, &Error{Status: http.StatusBadGateway, Code: "generation_failed",
				Message: "could not produce a clarify turn", Problems: att2.problems, Attempts: ar.meta.Attempts}
		}
		att = att2
	}
	reply := att.reply
	res.FirstTokenMs = att.ttft.Milliseconds()
	res.Meta = ar.meta
	res.Source = "claude"
	brief := cleanBrief(reply.Brief)
	transcript := transcriptOr(reply.Transcript, userTexts)
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
	plainChatResult(res) // #258
	cv.messages = append(cv.messages, ChatMessage{Role: "assistant", Text: res.Text})
	store.put(req.AppID, cv)
	res.LatencyMs = Latency{Claude: ar.latency.Milliseconds(), Total: time.Since(started).Milliseconds()}
	return res, nil
}

func transcriptOr(t string, userTexts []string) string {
	if t = strings.TrimSpace(t); t != "" {
		return t
	}
	return strings.Join(userTexts, "。")
}

func (s *Service) chatTimeout() time.Duration {
	if s.CallTimeout == 0 {
		return 40 * time.Second
	}
	return s.CallTimeout
}

// chatAttempt is one Claude call plus parsing. onQuestion, when set, is called
// once (from the streaming goroutine) as soon as a complete question has
// streamed.
type earlyQuestion struct {
	reply chatReply
	ttft  time.Duration
}

type chatAttempt struct {
	reply    *chatReply
	problems []string
	ttft     time.Duration
	err      error
}

func (s *Service) chatAttempt(ctx context.Context, cfg ChatConfig, user string, ar *attemptResult, onQuestion func(earlyQuestion)) chatAttempt {
	var buf strings.Builder
	sent := false
	start := time.Now()
	var first time.Duration
	resp, ttft, err := s.callChat(ctx, cfg, user, ar, func(delta string) {
		if first == 0 {
			first = time.Since(start)
		}
		if onQuestion == nil || sent {
			return
		}
		buf.WriteString(delta)
		if q, ok := partialQuestion(buf.String()); ok {
			sent = true
			onQuestion(earlyQuestion{reply: q, ttft: first})
		}
	})
	if err != nil {
		return chatAttempt{err: err}
	}
	var r chatReply
	if uerr := json.Unmarshal(extractJSON(resp.Text), &r); uerr != nil {
		return chatAttempt{problems: []string{"output is not valid JSON for the schema: " + uerr.Error()}, ttft: ttft}
	}
	if r.Kind != "question" && r.Kind != "ready" {
		return chatAttempt{problems: []string{`kind must be "question" or "ready"`}, ttft: ttft}
	}
	return chatAttempt{reply: &r, ttft: ttft}
}

// partialQuestion reports a question whose kind, text and suggestions are
// complete in the streamed JSON prefix.
func partialQuestion(prefix string) (chatReply, bool) {
	f := completeFields(prefix)
	var q chatReply
	if f["kind"] == nil || f["text"] == nil || f["suggestions"] == nil {
		return q, false
	}
	if json.Unmarshal(f["kind"], &q.Kind) != nil || q.Kind != "question" {
		return q, false
	}
	if json.Unmarshal(f["text"], &q.Text) != nil || strings.TrimSpace(q.Text) == "" {
		return q, false
	}
	if json.Unmarshal(f["suggestions"], &q.Suggestions) != nil {
		return q, false
	}
	return q, true
}

// completeFields returns the top-level members of a JSON object prefix whose
// values are already complete (a cut-off string or array is left out).
func completeFields(prefix string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	dec := json.NewDecoder(strings.NewReader(prefix))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return out
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return out
		}
		key, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return out
		}
		out[key] = v
	}
	return out
}

// callChat is call() with the small schema sent as output_config.format. It
// always streams: onText (optional) sees each delta, and the time to the
// first output token is returned.
func (s *Service) callChat(ctx context.Context, cfg ChatConfig, user string, ar *attemptResult, onText func(string)) (*Response, time.Duration, error) {
	return s.callChatWith(ctx, cfg, chatSystemPrompt, ChatSchema(), chatMaxTokens, user, ar, onText)
}

// callChatWith is callChat with an explicit system prompt, schema and token cap.
func (s *Service) callChatWith(ctx context.Context, cfg ChatConfig, system string, schema json.RawMessage, maxTokens int, user string, ar *attemptResult, onText func(string)) (*Response, time.Duration, error) {
	cctx, cancel := context.WithTimeout(ctx, s.chatTimeout())
	defer cancel()
	t := time.Now()
	var ttft time.Duration
	resp, err := s.Claude.Complete(cctx, Completion{
		System:     system,
		Messages:   []Message{{Role: "user", Content: user}},
		Schema:     schema,
		Structured: true,
		MaxTokens:  maxTokens,
		Model:      cfg.Model,
		Effort:     cfg.Effort,
		Thinking:   cfg.Thinking,
		OnText: func(d string) {
			if ttft == 0 {
				ttft = time.Since(t)
			}
			if onText != nil {
				onText(d)
			}
		},
	})
	ar.latency += time.Since(t)
	ar.meta.Model = cfg.Model
	ar.meta.Attempts++
	ar.meta.Regenerations = ar.meta.Attempts - 1
	if err != nil {
		return nil, 0, mapClaudeErr(err)
	}
	ar.meta.Usage.Add(resp.Usage)
	if resp.StopReason == "refusal" {
		return nil, 0, &Error{Status: http.StatusBadRequest, Code: "refused", Message: "the model declined this request", Attempts: ar.meta.Attempts}
	}
	return resp, ttft, nil
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
