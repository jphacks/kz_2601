package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
)

// serverNeed (AI_SERVER_RULES=1 only): the main create / edit call says, as
// the last root key, whether the request needs an automatic server action.
// It decides whether the server-rules pass runs, together with the
// wantsServerRules regex (safety net, also covers a missing field). The key
// lives only in the AI layer: it is added to the generation schemas per
// request (schema.go addServerNeed) and stripped before dsl.Parse; the
// canonical DSL schema, iOS and docs/dsl/schema never see it.

const serverNeedKey = "serverNeed"

// serverNeedValues are the enum of the field ("none" = no server action).
var serverNeedValues = []string{"none", "push", "timed", "record"}

// serverNeedSection is added to block 1 (systemPrompt) of create and edit.
var serverNeedSection = strings.TrimSpace(`
## サーバーの自動の動き(serverNeed)
- 出力の最後に "serverNeed" を 1 つ書く。依頼に次が含まれればその種類、無ければ "none"。
  - "push": 他の人の操作(記録・投稿・出欠・予約・メッセージ)で、作った人・参加者・宛先の人に届く通知
  - "timed": 決まった時刻(毎朝・毎週・月末)にサーバーで動く通知・まとめ・記録(期限の知らせ・順位・まだ〜ない物を選ぶ・名簿との照合)
  - "record": 記録の追加・変更をきっかけに数える・別の記録を書く・状態を残す・合計が○○を超えたら知らせる・似ている記録を探す
- 自分の端末だけで済む時刻の通知(app.notifications・自分へのリマインド)と、画面の計算で済む集計は "none"。
- 外部への送信など作れない依頼でも、通知や自動の動きを頼まれていれば種類を書く(断る文は後の工程が書く)。
- 編集で今の自動の動き(serverRules)をやめる・変える・オン/オフする依頼も、その種類を書く。
`)

// insertServerNeedSection puts the section after the section that holds the
// push line (anchor), i.e. before the next "\n## " heading, or at the end.
// It stays inside systemPrompt's bytes, so the create / edit shared prefix
// (#782) holds; editRules (block 2) is untouched.
func insertServerNeedSection(s, anchor string) string {
	i := strings.Index(s, anchor)
	if i < 0 {
		return s
	}
	rest := s[i+len(anchor):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		at := i + len(anchor) + j
		// keep the blank line before the next heading where there was one
		at = len(strings.TrimRight(s[:at], "\n"))
		return s[:at] + "\n\n" + serverNeedSection + s[at:]
	}
	return s + "\n\n" + serverNeedSection
}

// serverNeedAsked: the model said the request needs an automatic action.
func serverNeedAsked(need string) bool { return need != "" && need != "none" }

// mergeServerNeed ORs two answers (lint retries): a non-none answer wins.
func mergeServerNeed(a, b string) string {
	if serverNeedAsked(a) {
		return a
	}
	if serverNeedAsked(b) || a == "" {
		return b
	}
	return a
}

// takeServerNeed removes the root serverNeed key from a create output. The
// bytes are returned as is when the key is absent (flag off: byte-identical).
// An unknown value is kept as "" (seen) so the regex decides.
func takeServerNeed(raw []byte) (out []byte, need string, seen bool) {
	if !bytes.Contains(raw, []byte(`"`+serverNeedKey+`"`)) {
		return raw, "", false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, "", false
	}
	v, ok := root[serverNeedKey]
	if !ok {
		return raw, "", false
	}
	delete(root, serverNeedKey)
	out, err := json.Marshal(root)
	if err != nil {
		return raw, "", false
	}
	return out, validServerNeed(v), true
}

// editServerNeed reads serverNeed beside the PatchDocument of an edit output.
func editServerNeed(raw []byte) (need string, seen bool) {
	var w struct {
		ServerNeed json.RawMessage `json:"serverNeed"`
	}
	if json.Unmarshal(raw, &w) != nil || w.ServerNeed == nil {
		return "", false
	}
	return validServerNeed(w.ServerNeed), true
}

func validServerNeed(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) != nil {
		return ""
	}
	s = strings.ToLower(strings.TrimSpace(s))
	for _, ok := range serverNeedValues {
		if s == ok {
			return s
		}
	}
	return ""
}

// serverRulesTrigger is the gate of the server-rules pass when serverRulesOn:
// "model" (serverNeed), "regex" (wantsServerRules), "both", or "" (no pass).
func serverRulesTrigger(need, transcript string) string {
	m, r := serverNeedAsked(need), wantsServerRules(transcript)
	switch {
	case m && r:
		return "both"
	case m:
		return "model"
	case r:
		return "regex"
	}
	return ""
}

var serverNeedSchemaMemo sync.Map // schema -> schema with serverNeed

// serverNeedSchema adds serverNeed to the create / edit output schema when
// ctx says serverRulesOn. The logic pass (also PatchGenerationSchema), the
// server-rules pass and the flag-off path get the schema unchanged.
func serverNeedSchema(ctx context.Context, schema json.RawMessage) json.RawMessage {
	if !serverPushFrom(ctx) {
		return schema
	}
	if op := operationOf(ctx); op != OpGenerate && op != OpPatch {
		return schema
	}
	k := string(schema)
	if v, ok := serverNeedSchemaMemo.Load(k); ok {
		return v.(json.RawMessage)
	}
	v, _ := serverNeedSchemaMemo.LoadOrStore(k, addServerNeed(schema))
	return v.(json.RawMessage)
}

// logServerNeedMissing notes a create / edit output without serverNeed while
// the flag is on (the regex alone decided).
func (s *Service) logServerNeedMissing(ctx context.Context, mode string) {
	l := s.Logger
	if l == nil {
		l = slog.Default()
	}
	l.WarnContext(ctx, "ai.server_need missing", slog.String("event", "ai.server_need"), slog.String("mode", mode))
}
