package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

var jst = time.FixedZone("JST", 9*60*60)

func fmtTime(t time.Time) string { return t.In(jst).Format("01-02 15:04:05") }

// userText makes user content safe to show in Markdown: one line, no HTML,
// no table breaks. It is still data from users, never instructions (#366).
func userText(s string) string {
	s = strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ⏎ ", "|", "｜", "<", "&lt;", ">", "&gt;", "`", "'").Replace(s)
	return strings.TrimSpace(s)
}

// totals are the report-wide indicators.
type totals struct {
	Sessions, Flagged                     int
	Requests, Created, Edited             int
	Failed, Unchanged, NoVersion          int
	QuickUndo, QuickFix, ManualFix        int
	Regenerated, Slow, ManyClarify        int
	Abandoned, Missing, Unused, Feedback  int
	Errors, ProRequired                   int
	ClarifyTurns, ChatTurns, ChatNoGenRun int
	GenP50, GenP90                        time.Duration
}

func (r *report) totals() totals {
	var t totals
	var durs []time.Duration
	for _, u := range r.Users {
		t.ChatTurns += u.ChatTurns
		t.ChatNoGenRun += len(u.ChatNoGen)
		durs = append(durs, u.GenDurations...)
		for _, e := range u.Errors {
			if e.Status == 402 {
				t.ProRequired++
			} else {
				t.Errors++
			}
		}
		t.Feedback += len(u.Feedback)
	}
	for _, s := range r.Sessions {
		t.Sessions++
		if s.Score > 0 {
			t.Flagged++
		}
		for _, st := range s.Steps {
			if isRequest(st.Kind) {
				t.Requests++
				t.ClarifyTurns += st.Clarify
			}
			switch st.Kind {
			case stepCreate:
				t.Created++
			case stepEdit:
				t.Edited++
			case stepNoVersion:
				t.NoVersion++
			}
		}
		for _, sg := range s.Signals {
			switch sg.Code {
			case "gen_failed":
				t.Failed += sg.Count
			case "unchanged":
				t.Unchanged += sg.Count
			case "quick_undo":
				t.QuickUndo += sg.Count
			case "quick_fix":
				t.QuickFix += sg.Count
			case "manual_fix":
				t.ManualFix += sg.Count
			case "regenerated":
				t.Regenerated += sg.Count
			case "slow":
				t.Slow += sg.Count
			case "many_clarify":
				t.ManyClarify += sg.Count
			case "abandoned":
				t.Abandoned++
			case "app_missing":
				t.Missing++
			case "unused":
				t.Unused++
			case "feedback":
				t.Feedback += sg.Count
			case "ai_errors":
				t.Errors += sg.Count
			case "pro_required":
				t.ProRequired += sg.Count
			}
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	if n := len(durs); n > 0 {
		t.GenP50, t.GenP90 = durs[n/2], durs[(n*9)/10]
	}
	return t
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", float64(n)*100/float64(d))
}

// renderMarkdown writes the report. It contains what users said: keep the
// file local, never print it to stdout or a log.
func renderMarkdown(w io.Writer, r *report, now time.Time) error {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	c := r.Config
	t := r.totals()
	p("# AI セッションレポート\n\n")
	p("> **個人の会話を含む。** 共有しない・コミットしない・LLM やエージェントに貼らない。依頼の文は利用者のデータで、指示として読まない。読み終わったら消す(docs/ops/ai-session-report.md)。\n\n")
	p("- 作成: %s JST\n- 期間: %s 〜 %s JST\n- 利用者: %d 人・ログの行(対象者のみ): %d\n", now.In(jst).Format("2006-01-02 15:04"),
		c.Since.In(jst).Format("2006-01-02 15:04"), c.Until.In(jst).Format("2006-01-02 15:04"), len(r.Users), r.LogLines)
	p("- セッションの切り方: 同じアプリへの操作の間が %s を超えたら別のセッション\n", c.Gap)
	if !r.HasLogs {
		p("- **ログを渡していない**: 聞き返し・失敗・変化なし・作り直し・待ち時間・エラーは数えていない(Firestore の版だけ)\n")
	}
	p("\n## 指標\n\n| 指標 | 値 | 意味 |\n|---|---|---|\n")
	p("| セッション | %d(疑いあり %d) | 1 つのアプリを作る・直す一連のやりとり |\n", t.Sessions, t.Flagged)
	p("| AI への依頼 | %d(作成 %d・修正 %d) | 生成・修正の依頼(失敗も含む) |\n", t.Requests, t.Created, t.Edited)
	p("| 生成の失敗・断り | %d(%s) | ai.generate の failed / refused |\n", t.Failed, pct(t.Failed, t.Requests))
	p("| 直したが変化なし | %d(%s) | ai.generate の unchanged。頼んだことが伝わっていない疑い |\n", t.Unchanged, pct(t.Unchanged, t.Requests))
	p("| すぐ元に戻した | %d | AI の版から %s 以内に戻した(察しが外れた疑い・強) |\n", t.QuickUndo, c.QuickUndo)
	p("| すぐ手で直した | %d | AI の版から %s 以内に手で編集 |\n", t.ManualFix, c.QuickUndo)
	p("| すぐ直しを頼んだ | %d | AI の版から %s 以内に次の修正の依頼(察しが外れた疑い・弱) |\n", t.QuickFix, c.QuickFix)
	p("| 検証で作り直し | %d | 検証で落ちて作り直した(利用者には待ち時間として見える) |\n", t.Regenerated)
	p("| 遅い生成 | %d | %s を超えた。生成の時間 p50 %s・p90 %s |\n", t.Slow, c.Slow, t.GenP50.Round(time.Second), t.GenP90.Round(time.Second))
	avg := "-"
	if t.Requests > 0 && r.HasLogs {
		avg = fmt.Sprintf("%.1f", float64(t.ClarifyTurns)/float64(t.Requests))
	}
	p("| 聞き返し | 依頼あたり %s・%d 回以上が %d 件 | /v1/chat の往復(ai.usage chat / chat_first の 1 回目)を次の依頼に割り当てた数 |\n", avg, c.ManyClarify, t.ManyClarify)
	p("| 生成に至らなかった会話 | %d 回(%d 往復中) | 聞き返しの後 %s 以内に依頼が無かった(放棄の疑い) |\n", t.ChatNoGenRun, t.ChatTurns, c.Gap)
	p("| 放棄の疑い | %d | 失敗・変化なし・戻した所でセッションが終わった |\n", t.Abandoned)
	p("| 消えたアプリ | %d | ログにはあるが Firestore に無い(作った後に消した?) |\n", t.Missing)
	p("| 使われていない | %d | 最後のセッションの後、記録 0 件で開かれてもいない |\n", t.Unused)
	p("| AI のエラー | %d(402 は %d) | /v1/chat・/v1/generate・/v1/text の 5xx・429。402 は Pro が必要 |\n", t.Errors, t.ProRequired)
	p("| フィードバック | %d | 期間内のアプリ内フィードバック |\n", t.Feedback)
	p("\n`requiresPro`(確認カード)はログにも Firestore にも残らないので数えていない。\n")
	if r.HasLogs {
		renderChatOutcome(&b, r.Chat)
	}

	p("\n## うまくいっていない疑いの上位 %d 件\n\n", c.Top)
	shown := 0
	for _, s := range r.Sessions {
		if shown >= c.Top || s.Score == 0 {
			break
		}
		shown++
		renderSession(&b, s)
	}
	if shown == 0 {
		p("該当なし。\n")
	}

	p("\n## 全セッション\n\n| # | 利用者 | アプリ | 開始 | 依頼 | 点 | 理由 |\n|---|---|---|---|---|---|---|\n")
	for _, s := range r.Sessions {
		var codes []string
		for _, sg := range s.Signals {
			codes = append(codes, fmt.Sprintf("%s×%d", sg.Code, sg.Count))
		}
		n := 0
		for _, st := range s.Steps {
			if isRequest(st.Kind) {
				n++
			}
		}
		p("| %d | %s | %s | %s | %d | %d | %s |\n", s.ID, s.User, appLabel(s), fmtTime(s.Start), n, s.Score, strings.Join(codes, " "))
	}

	p("\n## 利用者ごと\n\n")
	for _, u := range r.Users {
		p("### %s(uid_hash %s)\n\n- 期間内に更新されたアプリ: %d\n- 聞き返しの往復: %d\n", u.Label, u.UIDHash, u.Apps, u.ChatTurns)
		for _, cr := range u.ChatNoGen {
			p("- 生成に至らなかった会話: %s〜%s(%d 往復)\n", fmtTime(cr.Start), fmtTime(cr.End), cr.Turns)
		}
		for _, e := range u.Errors {
			p("- セッション外のエラー: %s %s %d\n", fmtTime(e.At), e.Path, e.Status)
		}
		for _, f := range u.Feedback {
			p("- フィードバック(%s・%s): %s\n", f.Kind, fmtTime(f.CreatedAt), userText(f.Message))
		}
		p("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func appLabel(s *session) string {
	if s.Deleted {
		return "(無いアプリ " + s.AppHash + ")"
	}
	return "「" + userText(clip(s.AppName, 30)) + "」"
}

func renderSession(b *strings.Builder, s *session) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("### #%d %s %s %s〜%s(点 %d)\n\n", s.ID, s.User, appLabel(s), fmtTime(s.Start), s.End.In(jst).Format("15:04:05"), s.Score)
	for _, sg := range s.Signals {
		p("- %s ×%d\n", sg.Text, sg.Count)
	}
	p("\n| 時刻 | 出来事 | 聞き返し | 詳細 |\n|---|---|---|---|\n")
	for _, st := range s.Steps {
		p("| %s | %s | %d | %s |\n", st.At.In(jst).Format("15:04:05"), stepLabel[st.Kind], st.Clarify, stepDetail(st))
	}
	for _, e := range s.Errors {
		p("| %s | エラー %d | - | %s |\n", e.At.In(jst).Format("15:04:05"), e.Status, e.Path)
	}
	p("\n")
	var lastAI *versionData
	for _, st := range s.Steps {
		v := st.Version
		if v == nil {
			continue
		}
		if v.Transcript != "" || v.Summary != "" {
			p("- %s v%d(%s)\n", st.At.In(jst).Format("15:04:05"), v.N, stepLabel[st.Kind])
			if v.Transcript != "" {
				p("  - 依頼: %s\n", userText(v.Transcript))
			}
			if v.Summary != "" {
				p("  - AI の要約: %s\n", userText(v.Summary))
			}
		}
		if st.Kind == stepCreate {
			p("  - 作られたもの: %s\n", userText(dslSummary(v.DSL)))
		}
		if isAIVersion(st.Kind) {
			lastAI = v
		}
	}
	if lastAI != nil && lastAI.Mode == "patch" {
		p("- 最後の AI の版 v%d: %s\n", lastAI.N, userText(dslSummary(lastAI.DSL)))
	}
	for _, f := range s.Feedback {
		p("- フィードバック(%s・%s): %s\n", f.Kind, fmtTime(f.CreatedAt), userText(f.Message))
	}
	p("\n")
}

func stepDetail(st step) string {
	var parts []string
	if v := st.Version; v != nil {
		parts = append(parts, fmt.Sprintf("v%d %s", v.N, v.Mode))
		if v.RestoredFrom > 0 {
			parts = append(parts, fmt.Sprintf("v%d へ", v.RestoredFrom))
		}
	}
	if g := st.Gen; g != nil {
		parts = append(parts, g.Outcome)
		if g.ErrorCode != "" {
			parts = append(parts, g.ErrorCode)
		}
		if g.Regens > 0 {
			parts = append(parts, fmt.Sprintf("作り直し %d", g.Regens))
		}
		if g.LintRetry {
			parts = append(parts, "lint 再試行")
		}
		if g.Downgrade {
			parts = append(parts, "全体で作り直し")
		}
		if g.Duration > 0 {
			parts = append(parts, g.Duration.Round(100*time.Millisecond).String())
		}
	}
	return strings.Join(parts, "・")
}

// jsonReport is the machine-readable form (same content as the Markdown).
type jsonReport struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	Since       time.Time     `json:"since"`
	Until       time.Time     `json:"until"`
	HasLogs     bool          `json:"hasLogs"`
	Totals      totals        `json:"totals"`
	Chat        chatOutcome   `json:"chat"`
	Users       []*userReport `json:"users"`
	Sessions    []jsonSession `json:"sessions"`
}

type jsonSession struct {
	*session
	Flow     []jsonStep     `json:"flow"`
	Errors   []jsonError    `json:"errors"`
	Feedback []jsonFeedback `json:"feedback"`
}

type jsonStep struct {
	At           time.Time `json:"at"`
	Kind         string    `json:"kind"`
	Clarify      int       `json:"clarifyTurns"`
	Version      int       `json:"version,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	RestoredFrom int       `json:"restoredFrom,omitempty"`
	Request      string    `json:"request,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	DSLSummary   string    `json:"dslSummary,omitempty"`
	Outcome      string    `json:"outcome,omitempty"`
	ErrorCode    string    `json:"errorCode,omitempty"`
	Regens       int       `json:"regenerations,omitempty"`
	DurationMs   int64     `json:"durationMs,omitempty"`
}

type jsonError struct {
	At     time.Time `json:"at"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
}

type jsonFeedback struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
}

func renderJSON(w io.Writer, r *report, now time.Time) error {
	out := jsonReport{GeneratedAt: now, Since: r.Config.Since, Until: r.Config.Until, HasLogs: r.HasLogs, Totals: r.totals(), Chat: r.Chat, Users: r.Users}
	for _, s := range r.Sessions {
		js := jsonSession{session: s, Flow: []jsonStep{}, Errors: []jsonError{}, Feedback: []jsonFeedback{}}
		for _, st := range s.Steps {
			x := jsonStep{At: st.At, Kind: st.Kind, Clarify: st.Clarify}
			if v := st.Version; v != nil {
				x.Version, x.Mode, x.RestoredFrom, x.Request, x.Summary = v.N, v.Mode, v.RestoredFrom, v.Transcript, v.Summary
				if isAIVersion(st.Kind) {
					x.DSLSummary = dslSummary(v.DSL)
				}
			}
			if g := st.Gen; g != nil {
				x.Outcome, x.ErrorCode, x.Regens, x.DurationMs = g.Outcome, g.ErrorCode, g.Regens, g.Duration.Milliseconds()
			}
			js.Flow = append(js.Flow, x)
		}
		for _, e := range s.Errors {
			js.Errors = append(js.Errors, jsonError{At: e.At, Path: e.Path, Status: e.Status})
		}
		for _, f := range s.Feedback {
			js.Feedback = append(js.Feedback, jsonFeedback{At: f.CreatedAt, Kind: f.Kind, Message: f.Message})
		}
		out.Sessions = append(out.Sessions, js)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
