package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// Chat eval (-chat): runs internal/ai/eval/chat_cases.json through
// Service.Chat. Each case is a scripted conversation; each step sends one
// user utterance and checks `expect` (question|ready|answer|proposal),
// optional `source` (claude|shortcut|forced) and optional `native` keywords
// in the brief.
//
// Chat-first (#185): a case with `base` (a DSL file, e.g.
// internal/ai/examples/kakeibo.json) is seeded into the store first, so auto
// mode picks edit; `mode` / `capabilities` are sent as given (-capabilities
// sets them for cases that have neither). `textAny` (the reply text contains
// at least one) and `textNot` (none of them) check that answers match the
// DSL; `generate` passes the first option's instruction (proposal) or the
// transcript (ready) to Service.Generate on a copy of the base and records
// whether it went through. `optionsNot` (no option's title / detail /
// instruction contains any of them) and `genNot` (the generate summary
// contains none of them, e.g. 「省き」 = a part was dropped) catch ideas that
// mix in something the DSL cannot do.

type ChatStep struct {
	Say      string   `json:"say"`
	Expect   string   `json:"expect"`
	Source   string   `json:"source,omitempty"`
	Native   []string `json:"native,omitempty"`
	TextAny  []string `json:"textAny,omitempty"`
	TextNot  []string `json:"textNot,omitempty"`
	Generate bool     `json:"generate,omitempty"`
	// OptionsNot / GenNot: see the comment above.
	OptionsNot []string `json:"optionsNot,omitempty"`
	GenNot     []string `json:"genNot,omitempty"`
	// Multi (#220): expected multiSelect (nil = not checked).
	Multi *bool `json:"multi,omitempty"`
	// GenerateAll (#220): with generate, a multiSelect proposal sends every
	// option joined as iOS does for 「これでいく(N 件)」.
	GenerateAll bool `json:"generateAll,omitempty"`
	// SuggAny (#211): every suggestion chip contains at least one of these
	// (the current app's field names), so a chip answers the question.
	SuggAny []string `json:"suggAny,omitempty"`
}

type ChatCase struct {
	ID           string     `json:"id"`
	Group        string     `json:"group"`
	Locale       string     `json:"locale,omitempty"`
	Note         string     `json:"note,omitempty"`
	Base         string     `json:"base,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Capabilities []string   `json:"capabilities,omitempty"`
	Steps        []ChatStep `json:"steps"`
}

// ChatGenRecord is the outcome of passing a chat result to /generate.
type ChatGenRecord struct {
	Instruction string   `json:"instruction"`
	OK          bool     `json:"ok"`
	Error       string   `json:"error,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Ops         int      `json:"ops"`
	Unchanged   bool     `json:"unchanged,omitempty"` // #211: generate made no change
	TotalMs     int64    `json:"totalMs"`
	Lint        []string `json:"lint,omitempty"`
}

type ChatStepRecord struct {
	Say         string          `json:"say"`
	OK          bool            `json:"ok"`
	Error       string          `json:"error,omitempty"`
	Kind        string          `json:"kind,omitempty"`
	Source      string          `json:"source,omitempty"`
	Turn        int             `json:"turn"`
	Text        string          `json:"text,omitempty"`
	Suggestions []string        `json:"suggestions,omitempty"`
	Options     []ai.ChatOption `json:"options,omitempty"`
	MultiSelect bool            `json:"multiSelect,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	Gen         *ChatGenRecord  `json:"generate,omitempty"`
	Brief       *ai.Brief       `json:"brief,omitempty"`
	Transcript  string          `json:"transcript,omitempty"`
	ClaudeMs    int64           `json:"claudeMs"`
	FirstTokMs  int64           `json:"firstTokenMs"`
	TotalMs     int64           `json:"totalMs"`
	Attempts    int             `json:"attempts"`
	Usage       ai.Usage        `json:"usage"`
	Misses      []string        `json:"misses,omitempty"`
}

type ChatRecord struct {
	ID     string           `json:"id"`
	Group  string           `json:"group"`
	Locale string           `json:"locale"`
	OK     bool             `json:"ok"`
	Steps  []ChatStepRecord `json:"steps"`
}

type ChatSummary struct {
	Cases       int   `json:"cases"`
	CasesOK     int   `json:"casesOk"`
	Steps       int   `json:"steps"`
	StepsOK     int   `json:"stepsOk"`
	ClaudeCalls int   `json:"claudeCalls"`
	ClaudeP50Ms int64 `json:"claudeP50Ms"` // over steps that called Claude
	ClaudeP95Ms int64 `json:"claudeP95Ms"`
	FirstTokP50 int64 `json:"firstTokenP50Ms"`
	InTokP50    int64 `json:"inputTokensP50"`
	OutTokP50   int64 `json:"outputTokensP50"`
	TotalP50Ms  int64 `json:"totalP50Ms"` // over every step incl. shortcuts
}

type ChatRun struct {
	Date    string       `json:"date"`
	Model   string       `json:"model"`
	Effort  string       `json:"effort"`
	Tag     string       `json:"tag,omitempty"`
	Cases   []ChatRecord `json:"cases"`
	Summary ChatSummary  `json:"summary"`
}

func runChat(ctx context.Context, svc *ai.Service, casesPath, outDir, tag string, filter map[string]bool, caps []string) {
	cfg := svc.ChatConfig
	if cfg.Model == "" {
		cfg.Model = ai.DefaultChatModel
	}
	if cfg.Effort == "" {
		cfg.Effort = ai.DefaultChatEffort
	}
	if cfg.Thinking == "" {
		cfg.Thinking = ai.DefaultChatThinking
	}
	model, effort := cfg.Model, cfg.Effort
	if cfg.Thinking != "" {
		effort += "-" + cfg.Thinking
	}
	raw, err := os.ReadFile(casesPath)
	if err != nil {
		log.Fatal(err)
	}
	var cases []ChatCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		log.Fatal(err)
	}
	run := ChatRun{Date: time.Now().Format("2006-01-02"), Model: model, Effort: effort, Tag: tag}
	for _, c := range cases {
		if len(filter) > 0 && !filter[c.ID] {
			continue
		}
		appID := fmt.Sprintf("chat-%s-%d", c.ID, time.Now().UnixNano())
		rec := ChatRecord{ID: c.ID, Group: c.Group, Locale: c.Locale, OK: true}
		var base []byte
		if c.Base != "" {
			base, err = os.ReadFile(c.Base)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := svc.Store.Put(ctx, appID, 0, base, appconfigclient.Source{Kind: "seed"}); err != nil {
				log.Fatalf("[%s] seed: %v", c.ID, err)
			}
		}
		capsFor := c.Capabilities
		if len(capsFor) == 0 && c.Mode == "" {
			capsFor = caps
		}
		for i, st := range c.Steps {
			log.Printf("[%s] step %d %q", c.ID, i+1, st.Say)
			sr := ChatStepRecord{Say: st.Say}
			started := time.Now()
			res, err := svc.Chat(ctx, ai.ChatRequest{AppID: appID, Locale: ai.Locale(c.Locale), Mode: c.Mode, Capabilities: capsFor,
				Messages: []ai.ChatMessage{{Role: "user", Text: st.Say}}})
			sr.TotalMs = time.Since(started).Milliseconds()
			if err != nil {
				sr.Error = err.Error()
				rec.OK = false
				rec.Steps = append(rec.Steps, sr)
				log.Printf("[%s] FAIL %v", c.ID, err)
				break
			}
			sr.Kind, sr.Source, sr.Turn, sr.Text = res.Kind, res.Source, res.Turn, res.Text
			sr.Suggestions, sr.Brief, sr.Transcript = res.Suggestions, res.Brief, res.Transcript
			sr.Options, sr.Mode, sr.MultiSelect = res.Options, res.Mode, res.MultiSelect
			sr.ClaudeMs, sr.Attempts, sr.Usage = res.LatencyMs.Claude, res.Meta.Attempts, res.Meta.Usage
			sr.FirstTokMs = res.FirstTokenMs
			if rec.Locale == "" {
				rec.Locale = string(res.Locale)
			}
			sr.Misses = checkChatStep(st, res)
			if st.Generate && base != nil {
				sr.Gen = generateFromChat(ctx, svc, c.ID, base, res, st.GenerateAll)
				if !sr.Gen.OK {
					sr.Misses = append(sr.Misses, "generate failed: "+sr.Gen.Error)
				}
				if sr.Gen.Unchanged {
					sr.Misses = append(sr.Misses, "generate unchanged: "+sr.Gen.Summary)
				}
				for _, w := range st.GenNot {
					if strings.Contains(sr.Gen.Summary, w) {
						sr.Misses = append(sr.Misses, "generate summary has "+w)
					}
				}
			}
			sr.OK = len(sr.Misses) == 0
			if !sr.OK {
				rec.OK = false
			}
			log.Printf("[%s] %s/%s mode=%s turn=%d claude=%dms total=%dms misses=%v text=%q sugg=%v options=%v multi=%v transcript=%q",
				c.ID, res.Kind, res.Source, res.Mode, res.Turn, sr.ClaudeMs, sr.TotalMs, sr.Misses, res.Text, res.Suggestions, res.Options, res.MultiSelect, res.Transcript)
			rec.Steps = append(rec.Steps, sr)
		}
		run.Cases = append(run.Cases, rec)
	}
	run.Summary = summarizeChat(run.Cases)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	name := fmt.Sprintf("%s-chat-%s-%s", run.Date, model, effort)
	if tag != "" {
		name += "-" + tag
	}
	out, _ := json.MarshalIndent(run, "", "  ")
	path := filepath.Join(outDir, name+".json")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatal(err)
	}
	printChatTable(run)
	fmt.Printf("\nwrote %s\n", path)
}

// generateFromChat runs /generate (edit) with what 「これでいく」 / ready
// would send, on a fresh copy of the base DSL.
func generateFromChat(ctx context.Context, svc *ai.Service, caseID string, base []byte, res *ai.ChatResult, all bool) *ChatGenRecord {
	g := &ChatGenRecord{}
	switch {
	case res.Kind == "proposal" && all && res.MultiSelect:
		g.Instruction = joinInstructions(res.Options)
	case res.Kind == "proposal" && len(res.Options) > 0:
		g.Instruction = res.Options[0].Instruction
	case res.Kind == "ready":
		g.Instruction = res.Transcript
	default:
		g.Error = "nothing to generate (kind " + res.Kind + ")"
		return g
	}
	appID := fmt.Sprintf("gen-%s-%d", caseID, time.Now().UnixNano())
	if _, err := svc.Store.Put(ctx, appID, 0, base, appconfigclient.Source{Kind: "seed"}); err != nil {
		g.Error = err.Error()
		return g
	}
	started := time.Now()
	out, err := svc.Generate(ctx, ai.Request{AppID: appID, Transcript: g.Instruction, Mode: ai.ModeEdit, Locale: res.Locale})
	g.TotalMs = time.Since(started).Milliseconds()
	if err != nil {
		g.Error = err.Error()
		return g
	}
	g.OK, g.Mode, g.Summary, g.Ops, g.Lint = true, out.Mode, out.Summary, len(out.Patch), out.Meta.Lint
	g.Unchanged = out.Unchanged
	log.Printf("[%s] generate %s ops=%d %dms summary=%q", caseID, out.Mode, g.Ops, g.TotalMs, out.Summary)
	return g
}

func checkChatStep(st ChatStep, res *ai.ChatResult) []string {
	var miss []string
	lower := strings.ToLower(res.Text)
	if len(st.TextAny) > 0 {
		hit := false
		for _, w := range st.TextAny {
			if strings.Contains(lower, strings.ToLower(w)) {
				hit = true
			}
		}
		if !hit {
			miss = append(miss, "text lacks any of "+strings.Join(st.TextAny, "/"))
		}
	}
	for _, w := range st.TextNot {
		if strings.Contains(lower, strings.ToLower(w)) {
			miss = append(miss, "text has "+w)
		}
	}
	for _, o := range res.Options {
		all := strings.ToLower(o.Title + " " + o.Detail + " " + o.Instruction)
		for _, w := range st.OptionsNot {
			if strings.Contains(all, strings.ToLower(w)) {
				miss = append(miss, "option "+o.ID+" has "+w)
			}
		}
	}
	if res.Kind == "proposal" {
		if n := len(res.Options); n < 1 || n > 3 {
			miss = append(miss, fmt.Sprintf("options=%d", n))
		}
		for _, o := range res.Options {
			if o.Instruction == "" || o.Title == "" {
				miss = append(miss, "empty option")
			}
		}
	}
	if st.Multi != nil && res.MultiSelect != *st.Multi {
		miss = append(miss, fmt.Sprintf("multiSelect=%v (want %v)", res.MultiSelect, *st.Multi))
	}
	if st.Expect != "" && res.Kind != st.Expect {
		miss = append(miss, "kind="+res.Kind+" (want "+st.Expect+")")
	}
	if st.Source != "" && res.Source != st.Source {
		miss = append(miss, "source="+res.Source+" (want "+st.Source+")")
	}
	if res.Kind == "question" && len(st.SuggAny) > 0 {
		for _, sg := range res.Suggestions {
			hit := false
			for _, w := range st.SuggAny {
				hit = hit || strings.Contains(sg, w)
			}
			if !hit {
				miss = append(miss, "suggestion not from the app: "+sg)
			}
		}
	}
	if res.Kind == "question" {
		if strings.TrimSpace(res.Text) == "" {
			miss = append(miss, "empty question")
		}
		if n := len(res.Suggestions); n < 1 || n > 3 {
			miss = append(miss, fmt.Sprintf("suggestions=%d", n))
		}
	}
	if res.Kind == "ready" {
		if strings.TrimSpace(res.Transcript) == "" {
			miss = append(miss, "empty transcript")
		}
		have := map[string]bool{}
		if res.Brief != nil {
			for _, n := range res.Brief.Native {
				have[n] = true
			}
		}
		for _, want := range st.Native {
			if !have[want] {
				miss = append(miss, "native missing "+want)
			}
		}
	}
	return miss
}

func summarizeChat(recs []ChatRecord) ChatSummary {
	var s ChatSummary
	var claudeMs, totalMs, ttft, inTok, outTok []int64
	for _, r := range recs {
		s.Cases++
		if r.OK {
			s.CasesOK++
		}
		for _, st := range r.Steps {
			s.Steps++
			if st.OK {
				s.StepsOK++
			}
			if st.Error != "" {
				continue
			}
			totalMs = append(totalMs, st.TotalMs)
			if st.Attempts > 0 {
				s.ClaudeCalls += st.Attempts
				claudeMs = append(claudeMs, st.ClaudeMs)
				ttft = append(ttft, st.FirstTokMs)
				u := st.Usage
				inTok = append(inTok, int64(u.InputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens))
				outTok = append(outTok, int64(u.OutputTokens))
			}
		}
	}
	s.ClaudeP50Ms = percentile(claudeMs, 0.5)
	s.ClaudeP95Ms = percentile(claudeMs, 0.95)
	s.TotalP50Ms = percentile(totalMs, 0.5)
	s.FirstTokP50 = percentile(ttft, 0.5)
	s.InTokP50 = percentile(inTok, 0.5)
	s.OutTokP50 = percentile(outTok, 0.5)
	return s
}

func printChatTable(run ChatRun) {
	fmt.Printf("\n## chat %s %s effort=%s\n\n", run.Date, run.Model, run.Effort)
	fmt.Println("| id | loc | step | say | kind | src | turn | claude ms | 1st tok ms | in/out tok | text | suggestions | native | misses |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range run.Cases {
		for i, st := range r.Steps {
			kind := st.Kind
			if st.Error != "" {
				kind = "ERROR: " + st.Error
			}
			native := "-"
			if st.Brief != nil && len(st.Brief.Native) > 0 {
				native = strings.Join(st.Brief.Native, ",")
			}
			misses := "-"
			if len(st.Misses) > 0 {
				misses = strings.Join(st.Misses, "; ")
			}
			sugg := "-"
			if len(st.Suggestions) > 0 {
				sugg = strings.Join(st.Suggestions, " / ")
			}
			for _, o := range st.Options {
				sugg += " [" + o.Title + ": " + o.Instruction + "]"
			}
			if st.Transcript != "" && st.Brief == nil {
				sugg += " transcript=" + st.Transcript
			}
			if st.Gen != nil {
				sugg += fmt.Sprintf(" → generate ok=%v unchanged=%v %s %dms %s", st.Gen.OK, st.Gen.Unchanged, st.Gen.Mode, st.Gen.TotalMs, st.Gen.Summary+st.Gen.Error)
			}
			fmt.Printf("| %s | %s | %d | %s | %s | %s | %d | %d | %d | %d/%d | %s | %s | %s | %s |\n",
				r.ID, r.Locale, i+1, cell(st.Say), kind, st.Source, st.Turn, st.ClaudeMs, st.FirstTokMs,
				st.Usage.InputTokens+st.Usage.CacheReadInputTokens+st.Usage.CacheCreationInputTokens, st.Usage.OutputTokens,
				cell(st.Text), cell(sugg), native, misses)
		}
	}
	s := run.Summary
	fmt.Printf("\ncases=%d ok=%d | steps=%d ok=%d | claude calls=%d p50=%dms p95=%dms 1st-tok p50=%dms in/out tok p50=%d/%d | step total p50=%dms\n",
		s.Cases, s.CasesOK, s.Steps, s.StepsOK, s.ClaudeCalls, s.ClaudeP50Ms, s.ClaudeP95Ms, s.FirstTokP50, s.InTokP50, s.OutTokP50, s.TotalP50Ms)
	ids := make([]string, 0)
	for _, r := range run.Cases {
		if !r.OK {
			ids = append(ids, r.ID)
		}
	}
	sort.Strings(ids)
	if len(ids) > 0 {
		fmt.Printf("failed: %s\n", strings.Join(ids, ", "))
	}
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "/"), "\n", " ") }

// joinInstructions mirrors iOS ChatOption.joinedInstruction (#220).
func joinInstructions(opts []ai.ChatOption) string {
	if len(opts) == 1 {
		return opts[0].Instruction
	}
	lines := make([]string, len(opts))
	for i, o := range opts {
		lines[i] = "・" + o.Instruction
	}
	return strings.Join(lines, "\n")
}
