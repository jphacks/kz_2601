package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// config is the period and the thresholds of the signals.
type config struct {
	Since, Until time.Time
	Gap          time.Duration // a pause longer than this starts a new session
	QuickUndo    time.Duration // restore / manual edit this soon after an AI version = miss
	QuickFix     time.Duration // AI edit this soon after an AI version = possible miss
	Slow         time.Duration // generation slower than this
	ManyClarify  int           // clarify turns before one generation
	Top          int
	RecordLimit  int // records counted per app (ids only)
}

// Step kinds of a session's timeline.
const (
	stepSeed      = "seed"          // blank app (system v1)
	stepCreate    = "ai_create"     // AI built the app (full DSL)
	stepEdit      = "ai_edit"       // AI changed the app
	stepRestore   = "restore"       // back to an older version (undo / history)
	stepManual    = "manual"        // edited by hand in the app
	stepRename    = "rename"        // renamed (manual, not a miss)
	stepImport    = "import"        // copy / share import
	stepFailed    = "gen_failed"    // generation failed (validation / upstream)
	stepRefused   = "gen_refused"   // the model refused
	stepUnchanged = "gen_unchanged" // edit request that changed nothing
	stepBadReq    = "gen_bad_request"
	stepNoVersion = "gen_no_version" // generation logged, version not found (app deleted?)
)

var stepLabel = map[string]string{
	stepSeed: "空のアプリを作成", stepCreate: "AI が作成", stepEdit: "AI が修正", stepRestore: "元に戻した",
	stepManual: "手で編集", stepRename: "名前を変更", stepImport: "取り込み・複製", stepFailed: "生成に失敗",
	stepRefused: "AI が断った", stepUnchanged: "修正したが変化なし", stepBadReq: "不正な依頼(4xx)",
	stepNoVersion: "生成した(版が見つからない)",
}

// isRequest reports a step that answered a user's request to the AI.
func isRequest(kind string) bool {
	switch kind {
	case stepCreate, stepEdit, stepFailed, stepRefused, stepUnchanged, stepBadReq, stepNoVersion:
		return true
	}
	return false
}

func isAIVersion(kind string) bool { return kind == stepCreate || kind == stepEdit }

type step struct {
	At      time.Time    `json:"at"`
	Kind    string       `json:"kind"`
	Version *versionData `json:"-"`
	Gen     *logEvent    `json:"-"`
	Clarify int          `json:"clarifyTurns"` // chat turns before this request
}

type hint struct {
	Code   string `json:"code"`
	Count  int    `json:"count"`
	Weight int    `json:"weight"`
	Text   string `json:"text"`
}

type session struct {
	ID          int            `json:"id"`
	User        string         `json:"user"`
	AppID       string         `json:"appId,omitempty"`
	AppName     string         `json:"appName,omitempty"`
	AppHash     string         `json:"appHash"`
	Deleted     bool           `json:"appMissing"`
	Start       time.Time      `json:"start"`
	End         time.Time      `json:"end"`
	Steps       []step         `json:"-"`
	Errors      []logEvent     `json:"-"`
	Feedback    []feedbackData `json:"-"`
	OpenedAfter int            `json:"openedAfter"`
	Records     int            `json:"records"`
	Last        bool           `json:"lastOfApp"`
	Signals     []hint         `json:"signals"`
	Score       int            `json:"score"`
	app         *appData
}

type chatRun struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Turns int       `json:"turns"`
}

type userReport struct {
	Label        string          `json:"label"`
	UIDHash      string          `json:"uidHash"`
	Apps         int             `json:"apps"`
	ChatNoGen    []chatRun       `json:"chatWithoutGeneration"`
	Errors       []logEvent      `json:"-"`
	Feedback     []feedbackData  `json:"-"`
	ChatTurns    int             `json:"chatTurns"`
	ChatLatency  []int64         `json:"-"`
	GenDurations []time.Duration `json:"-"`
}

type report struct {
	Config   config
	Users    []*userReport
	Sessions []*session
	LogLines int
	HasLogs  bool
	Chat     chatOutcome
	Retry    retryBreakdown
}

// signal weights: a stronger hint of "the AI guessed wrong" weighs more.
var signalDefs = map[string]struct {
	weight int
	text   string
}{
	"quick_undo":   {4, "AI の版のすぐ後に元に戻した"},
	"gen_failed":   {3, "生成に失敗した・断られた"},
	"unchanged":    {3, "直しを頼んだが何も変わらなかった"},
	"abandoned":    {3, "うまくいかない所で終わった(失敗・変化なし・戻した後に続きが無い)"},
	"app_missing":  {3, "このアプリはもう無い(作った後に消した?)"},
	"manual_fix":   {2, "AI の版のすぐ後に手で直した"},
	"many_clarify": {2, "聞き返しが多かった"},
	"ai_errors":    {2, "AI のエラー(5xx・429)"},
	"pro_required": {1, "Pro が必要で止まった(402)"},
	"feedback":     {2, "フィードバックが届いた"},
	"unused":       {2, "作った後に記録が 0 件で、開かれてもいない"},
	"quick_fix":    {1, "AI の版のすぐ後にまた直しを頼んだ"},
	"regenerated":  {1, "検証で落ちて作り直した"},
	"slow":         {1, "生成が遅かった"},
}

// signalOrder is the display order.
var signalOrder = []string{"quick_undo", "gen_failed", "unchanged", "abandoned", "app_missing", "manual_fix",
	"many_clarify", "ai_errors", "pro_required", "feedback", "unused", "quick_fix", "regenerated", "slow"}

// buildReport reads uids' data and logs and cuts it into sessions.
func buildReport(ctx context.Context, rd reader, uids []string, logs []logEvent, hasLogs bool, cfg config) (*report, error) {
	rep := &report{Config: cfg, LogLines: len(logs), HasLogs: hasLogs, Chat: chatOutcomes(logs, cfg.Since, cfg.Until, cfg.Gap),
		Retry: retryBreakdowns(logs, cfg.Since, cfg.Until)}
	for i, uid := range uids {
		u := &userReport{Label: fmt.Sprintf("U%d", i+1), UIDHash: httpx.UIDHash(uid)}
		rep.Users = append(rep.Users, u)
		var evs []logEvent
		for _, e := range logs {
			if e.UIDHash == u.UIDHash && !e.At.Before(cfg.Since.Add(-cfg.Gap)) && !e.At.After(cfg.Until) {
				evs = append(evs, e)
			}
		}
		apps, err := rd.Apps(ctx, uid, cfg.Since)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u.Label, err)
		}
		u.Apps = len(apps)
		for j := range apps {
			a := &apps[j]
			if a.Versions, err = rd.Versions(ctx, uid, a.ID, cfg.Since); err != nil {
				return nil, fmt.Errorf("%s: %w", u.Label, err)
			}
			if ents := entityIDs(a.DSL); len(ents) > 0 {
				if a.Records, err = rd.RecordCount(ctx, uid, a.ID, ents, cfg.RecordLimit); err != nil {
					return nil, fmt.Errorf("%s: %w", u.Label, err)
				}
			} else {
				a.Records = 0
			}
		}
		fb, err := rd.Feedback(ctx, uid, cfg.Since)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u.Label, err)
		}
		rep.Sessions = append(rep.Sessions, userSessions(u, apps, evs, fb, cfg)...)
	}
	sort.SliceStable(rep.Sessions, func(i, j int) bool {
		a, b := rep.Sessions[i], rep.Sessions[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Start.Before(b.Start)
	})
	for i, s := range rep.Sessions {
		s.ID = i + 1
	}
	return rep, nil
}

func userSessions(u *userReport, apps []appData, evs []logEvent, fb []feedbackData, cfg config) []*session {
	inWindow := func(t time.Time) bool { return !t.Before(cfg.Since) && !t.After(cfg.Until) }
	byHash := map[string]*appData{}
	for i := range apps {
		byHash[metrics.AppHash(apps[i].ID)] = &apps[i]
	}
	genByApp := map[string][]*logEvent{}
	opened := map[string][]time.Time{}
	var chats []logEvent
	for i := range evs {
		e := &evs[i]
		switch {
		case e.Event == evGenerate && e.AppHash != "" && inWindow(e.At):
			genByApp[e.AppHash] = append(genByApp[e.AppHash], e)
			if e.Duration > 0 {
				u.GenDurations = append(u.GenDurations, e.Duration)
			}
		case e.Event == evOpened && e.AppHash != "":
			opened[e.AppHash] = append(opened[e.AppHash], e.At)
		case e.isChatTurn():
			chats = append(chats, *e)
			if inWindow(e.At) {
				u.ChatTurns++
				u.ChatLatency = append(u.ChatLatency, e.LatencyMs)
			}
		case e.isAIError() && inWindow(e.At):
			u.Errors = append(u.Errors, *e)
		}
	}

	// timeline per app
	type appTL struct {
		hash  string
		app   *appData
		steps []step
	}
	var tls []*appTL
	for i := range apps {
		a := &apps[i]
		h := metrics.AppHash(a.ID)
		tls = append(tls, &appTL{hash: h, app: a, steps: appSteps(a, genByApp[h], inWindow)})
	}
	var missing []string
	for h := range genByApp {
		if byHash[h] == nil {
			missing = append(missing, h)
		}
	}
	sort.Strings(missing)
	for _, h := range missing {
		tls = append(tls, &appTL{hash: h, steps: appSteps(nil, genByApp[h], inWindow)})
	}

	// clarify turns go to the user's next request within cfg.Gap
	var reqs []*step
	for _, tl := range tls {
		for i := range tl.steps {
			if isRequest(tl.steps[i].Kind) {
				reqs = append(reqs, &tl.steps[i])
			}
		}
	}
	sort.SliceStable(reqs, func(i, j int) bool { return reqs[i].At.Before(reqs[j].At) })
	var orphan []time.Time
	ri := 0
	for _, c := range chats {
		for ri < len(reqs) && reqs[ri].At.Before(c.At) {
			ri++
		}
		if ri < len(reqs) && reqs[ri].At.Sub(c.At) <= cfg.Gap {
			if inWindow(reqs[ri].At) {
				reqs[ri].Clarify++
			}
			continue
		}
		if inWindow(c.At) {
			orphan = append(orphan, c.At)
		}
	}
	for _, t := range orphan {
		if n := len(u.ChatNoGen); n > 0 && t.Sub(u.ChatNoGen[n-1].End) <= cfg.Gap {
			u.ChatNoGen[n-1].End = t
			u.ChatNoGen[n-1].Turns++
			continue
		}
		u.ChatNoGen = append(u.ChatNoGen, chatRun{Start: t, End: t, Turns: 1})
	}

	// cut sessions
	var out []*session
	for _, tl := range tls {
		var cur *session
		var appSess []*session
		for _, st := range tl.steps {
			if cur == nil || st.At.Sub(cur.End) > cfg.Gap {
				cur = &session{User: u.Label, AppHash: tl.hash, Start: st.At, Records: -1, app: tl.app}
				if tl.app != nil {
					cur.AppID, cur.AppName, cur.Records = tl.app.ID, tl.app.Name, tl.app.Records
				} else {
					cur.Deleted = true
				}
				appSess = append(appSess, cur)
			}
			cur.Steps = append(cur.Steps, st)
			cur.End = st.At
		}
		// a session of only a seed / rename / import is not an AI exchange
		var kept []*session
		for _, s := range appSess {
			if hasRequest(s) || hasKind(s, stepRestore) {
				kept = append(kept, s)
			}
		}
		if n := len(kept); n > 0 {
			kept[n-1].Last = true
		}
		for _, s := range kept {
			for _, t := range opened[tl.hash] {
				if t.After(s.End.Add(time.Minute)) {
					s.OpenedAfter++
				}
			}
		}
		out = append(out, kept...)
	}

	// errors and feedback go to the session they happened in
	var rest []logEvent
	for _, e := range u.Errors {
		if s := sessionAt(out, e.At); s != nil {
			s.Errors = append(s.Errors, e)
			continue
		}
		rest = append(rest, e)
	}
	u.Errors = rest
	for _, f := range fb {
		var hit *session
		for _, s := range out {
			if s.AppID != "" && s.AppID == f.AppID && !f.CreatedAt.Before(s.Start) && f.CreatedAt.Before(s.End.Add(24*time.Hour)) {
				hit = s // the latest session before the feedback
			}
		}
		if hit != nil {
			hit.Feedback = append(hit.Feedback, f)
			continue
		}
		u.Feedback = append(u.Feedback, f)
	}
	for _, s := range out {
		score(s, cfg)
	}
	return out
}

// sessionAt returns the first session around t: from 10 minutes before its
// first step (a chat error comes before the generation) to 5 minutes after its last.
func sessionAt(ss []*session, t time.Time) *session {
	for _, s := range ss {
		if !t.Before(s.Start.Add(-10*time.Minute)) && !t.After(s.End.Add(5*time.Minute)) {
			return s
		}
	}
	return nil
}

func hasRequest(s *session) bool {
	for _, st := range s.Steps {
		if isRequest(st.Kind) {
			return true
		}
	}
	return false
}

func hasKind(s *session, kind string) bool {
	for _, st := range s.Steps {
		if st.Kind == kind {
			return true
		}
	}
	return false
}

// appSteps merges an app's versions with its ai.generate lines: a successful
// generation is matched to the AI version saved just before it was logged;
// failures and no-op edits make a step of their own (they save no version).
func appSteps(a *appData, gens []*logEvent, inWindow func(time.Time) bool) []step {
	var steps []step
	used := map[int]bool{}
	if a != nil {
		for i := range a.Versions {
			v := &a.Versions[i]
			if !inWindow(v.CreatedAt) {
				continue
			}
			steps = append(steps, step{At: v.CreatedAt, Kind: versionKind(v), Version: v})
		}
	}
	for _, g := range gens {
		var kind string
		switch g.Outcome {
		case "validated", "regenerated":
			if i := matchVersion(steps, used, g.At); i >= 0 {
				used[i] = true
				steps[i].Gen = g
				continue
			}
			kind = stepNoVersion
		case "restored":
			continue // the restore version is the record
		case "unchanged":
			kind = stepUnchanged
		case "refused":
			kind = stepRefused
		case "bad_request":
			kind = stepBadReq
		default:
			kind = stepFailed
		}
		steps = append(steps, step{At: g.At, Kind: kind, Gen: g})
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].At.Before(steps[j].At) })
	return steps
}

// matchVersion finds the unmatched AI version saved within 2 minutes before
// (or 10 seconds after) the ai.generate line at t, closest first.
func matchVersion(steps []step, used map[int]bool, t time.Time) int {
	best, bestD := -1, time.Duration(0)
	for i, s := range steps {
		if used[i] || !isAIVersion(s.Kind) {
			continue
		}
		d := t.Sub(s.At)
		if d < -10*time.Second || d > 2*time.Minute {
			continue
		}
		if d < 0 {
			d = -d
		}
		if best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func versionKind(v *versionData) string {
	switch v.Kind {
	case "ai":
		if v.Mode == "patch" || v.N > 2 {
			return stepEdit
		}
		return stepCreate
	case "restore":
		return stepRestore
	case "manual":
		if strings.HasPrefix(v.Summary, "名前を") {
			return stepRename
		}
		return stepManual
	case "import":
		return stepImport
	}
	return stepSeed
}

// score sets the signals and the total of a session.
func score(s *session, cfg config) {
	counts := map[string]int{}
	var lastAI time.Time
	for _, st := range s.Steps {
		switch st.Kind {
		case stepFailed, stepRefused, stepNoVersion:
			if st.Kind != stepNoVersion {
				counts["gen_failed"]++
			}
		case stepUnchanged:
			counts["unchanged"]++
		case stepRestore:
			if !lastAI.IsZero() && st.At.Sub(lastAI) <= cfg.QuickUndo {
				counts["quick_undo"]++
			}
		case stepManual:
			if !lastAI.IsZero() && st.At.Sub(lastAI) <= cfg.QuickUndo {
				counts["manual_fix"]++
			}
		case stepEdit:
			if !lastAI.IsZero() && st.At.Sub(lastAI) <= cfg.QuickFix {
				counts["quick_fix"]++
			}
		}
		if isAIVersion(st.Kind) {
			lastAI = st.At
		}
		if g := st.Gen; g != nil {
			if g.Regens > 0 || g.AreaFB || g.Downgrade || g.LintRetry {
				counts["regenerated"]++
			}
			if cfg.Slow > 0 && g.Duration > cfg.Slow {
				counts["slow"]++
			}
		}
		if cfg.ManyClarify > 0 && st.Clarify >= cfg.ManyClarify {
			counts["many_clarify"]++
		}
	}
	for _, e := range s.Errors {
		if e.Status == 402 {
			counts["pro_required"]++
		} else {
			counts["ai_errors"]++
		}
	}
	if n := len(s.Steps); n > 0 {
		switch s.Steps[n-1].Kind {
		case stepFailed, stepRefused, stepUnchanged, stepBadReq, stepRestore:
			counts["abandoned"] = 1
		}
	}
	if s.Deleted {
		counts["app_missing"] = 1
	}
	if s.Last && !s.Deleted && s.Records == 0 && s.OpenedAfter == 0 && (hasKind(s, stepCreate) || hasKind(s, stepEdit)) {
		counts["unused"] = 1
	}
	if len(s.Feedback) > 0 {
		counts["feedback"] = len(s.Feedback)
	}
	s.Signals, s.Score = nil, 0
	for _, code := range signalOrder {
		n := counts[code]
		if n == 0 {
			continue
		}
		d := signalDefs[code]
		s.Signals = append(s.Signals, hint{Code: code, Count: n, Weight: d.weight, Text: d.text})
		s.Score += n * d.weight
	}
}

// entityIDs lists the entity ids of a DSL (for counting records).
func entityIDs(raw json.RawMessage) []string {
	var d struct {
		Entities []struct {
			ID string `json:"id"`
		} `json:"entities"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return nil
	}
	var out []string
	for _, e := range d.Entities {
		if e.ID != "" {
			out = append(out, e.ID)
		}
	}
	return out
}

// dslSummary names the screens and components of a DSL:
// 「画面名」: list・button「追加」... User content: report file only.
func dslSummary(raw json.RawMessage) string {
	var d struct {
		App struct {
			Name string `json:"name"`
		} `json:"app"`
		Entities []struct {
			Name   string `json:"name"`
			Fields []any  `json:"fields"`
		} `json:"entities"`
		Screens []struct {
			Title      string           `json:"title"`
			Components []map[string]any `json:"components"`
		} `json:"screens"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return ""
	}
	var parts []string
	for _, sc := range d.Screens {
		var names []string
		var walk func([]map[string]any)
		walk = func(cs []map[string]any) {
			for _, c := range cs {
				if len(names) >= 12 {
					return
				}
				name, _ := c["type"].(string)
				for _, k := range []string{"label", "title", "text"} {
					if v, _ := c[k].(string); v != "" {
						name += "「" + clip(v, 20) + "」"
						break
					}
				}
				names = append(names, name)
				if ch, ok := c["children"].([]any); ok {
					var sub []map[string]any
					for _, x := range ch {
						if m, ok := x.(map[string]any); ok {
							sub = append(sub, m)
						}
					}
					walk(sub)
				}
			}
		}
		walk(sc.Components)
		parts = append(parts, "「"+clip(sc.Title, 30)+"」: "+strings.Join(names, "・"))
	}
	var ents []string
	for _, e := range d.Entities {
		ents = append(ents, fmt.Sprintf("%s(%d 項目)", clip(e.Name, 20), len(e.Fields)))
	}
	out := fmt.Sprintf("画面 %d: %s", len(d.Screens), strings.Join(parts, " / "))
	if len(ents) > 0 {
		out += " | データ: " + strings.Join(ents, "・")
	}
	return out
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
