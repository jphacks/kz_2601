package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// chatReplyKinds is the display order of ai.chat reply_kind.
var chatReplyKinds = []string{"question", "ready", "answer", "proposal", "other"}

// chatTurnBuckets labels conversation lengths (turns); the last one is open.
var chatTurnBuckets = []string{"1", "2", "3", "4", "5+"}

// chatOutcome aggregates the ai.chat labels of the listed users.
type chatOutcome struct {
	Turns         int            `json:"turns"`                   // replied turns
	Failed        int            `json:"failedTurns"`             // outcome failed / bad_request
	ReplyKinds    map[string]int `json:"replyKinds"`              // reply_kind -> turns
	Conversations int            `json:"conversations"`           // runs of turns on one app
	TurnsPerConv  map[string]int `json:"turnsPerConversation"`    // chatTurnBuckets -> conversations
	EndedReady    int            `json:"conversationsEndedReady"` // last reply was ready
	ClarifyBuilt  int            `json:"clarifyEndedInBuild"`     // ready after >= 1 earlier turn
}

func turnBucket(n int) string {
	if n >= len(chatTurnBuckets) {
		return chatTurnBuckets[len(chatTurnBuckets)-1]
	}
	return chatTurnBuckets[n-1]
}

// chatOutcomes reads the ai.chat lines in [since, until]. A conversation is
// the turns on one app (uid_hash + app_hash) until the turn number goes back
// (a new conversation started) or a pause longer than gap.
func chatOutcomes(logs []logEvent, since, until time.Time, gap time.Duration) chatOutcome {
	o := chatOutcome{ReplyKinds: map[string]int{}, TurnsPerConv: map[string]int{}}
	type conv struct {
		last     time.Time
		turn     int
		lastKind string
	}
	open := map[string]*conv{}
	closeConv := func(c *conv) {
		o.Conversations++
		o.TurnsPerConv[turnBucket(c.turn)]++
		if c.lastKind == "ready" {
			o.EndedReady++
		}
	}
	evs := make([]logEvent, 0, len(logs))
	for _, e := range logs {
		if e.Event == evChat && !e.At.Before(since) && !e.At.After(until) {
			evs = append(evs, e)
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At.Before(evs[j].At) })
	for _, e := range evs {
		if e.ReplyKind == "" || e.Turn < 1 {
			o.Failed++
			continue
		}
		o.Turns++
		k := e.ReplyKind
		if !contains(chatReplyKinds, k) {
			k = "other"
		}
		o.ReplyKinds[k]++
		if e.Built {
			o.ClarifyBuilt++
		}
		key := e.UIDHash + "/" + e.AppHash
		c := open[key]
		if c != nil && (e.Turn <= c.turn || e.At.Sub(c.last) > gap) {
			closeConv(c)
			c = nil
		}
		if c == nil {
			c = &conv{}
			open[key] = c
		}
		c.last, c.turn, c.lastKind = e.At, e.Turn, k
	}
	keys := make([]string, 0, len(open))
	for k := range open {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		closeConv(open[k])
	}
	return o
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// renderChatOutcome writes the ai.chat section (labels only, no user text).
func renderChatOutcome(b *strings.Builder, o chatOutcome) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("\n## 会話の結果(ai.chat)\n\n")
	if o.Turns == 0 && o.Failed == 0 {
		p("ai.chat の行が無い(このログを出す前の期間、またはログを渡していない)。\n")
		return
	}
	p("| 返答の種類 | 往復 | 割合 |\n|---|---|---|\n")
	for _, k := range chatReplyKinds {
		p("| %s | %d | %s |\n", k, o.ReplyKinds[k], pct(o.ReplyKinds[k], o.Turns))
	}
	p("\n- 失敗した往復: %d\n- 聞き返しの後に作る(ready)に至った: %d\n", o.Failed, o.ClarifyBuilt)
	p("- 会話: %d(最後が ready で終わった %d・%s)\n\n", o.Conversations, o.EndedReady, pct(o.EndedReady, o.Conversations))
	p("| 会話の往復数 | 会話 |\n|---|---|\n")
	for _, k := range chatTurnBuckets {
		p("| %s | %d |\n", k, o.TurnsPerConv[k])
	}
	p("\n「できない・代わりに〜」の返答はまだ印が無いので数えていない(プロンプトとスキーマの変更が要る)。\n")
}
