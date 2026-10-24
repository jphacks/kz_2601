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
)

// Chat eval (-chat): runs internal/ai/eval/chat_cases.json through
// Service.Chat. Each case is a scripted conversation; each step sends one
// user utterance and checks `expect` (question|ready), optional `source`
// (claude|shortcut|forced) and optional `native` keywords in the brief.

type ChatStep struct {
	Say    string   `json:"say"`
	Expect string   `json:"expect"`
	Source string   `json:"source,omitempty"`
	Native []string `json:"native,omitempty"`
}

type ChatCase struct {
	ID     string     `json:"id"`
	Group  string     `json:"group"`
	Locale string     `json:"locale,omitempty"`
	Note   string     `json:"note,omitempty"`
	Steps  []ChatStep `json:"steps"`
}

type ChatStepRecord struct {
	Say         string    `json:"say"`
	OK          bool      `json:"ok"`
	Error       string    `json:"error,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Source      string    `json:"source,omitempty"`
	Turn        int       `json:"turn"`
	Text        string    `json:"text,omitempty"`
	Suggestions []string  `json:"suggestions,omitempty"`
	Brief       *ai.Brief `json:"brief,omitempty"`
	Transcript  string    `json:"transcript,omitempty"`
	ClaudeMs    int64     `json:"claudeMs"`
	FirstTokMs  int64     `json:"firstTokenMs"`
	TotalMs     int64     `json:"totalMs"`
	Attempts    int       `json:"attempts"`
	Usage       ai.Usage  `json:"usage"`
	Misses      []string  `json:"misses,omitempty"`
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

func runChat(ctx context.Context, svc *ai.Service, casesPath, outDir, tag string, filter map[string]bool) {
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
		for i, st := range c.Steps {
			log.Printf("[%s] step %d %q", c.ID, i+1, st.Say)
			sr := ChatStepRecord{Say: st.Say}
			started := time.Now()
			res, err := svc.Chat(ctx, ai.ChatRequest{AppID: appID, Locale: ai.Locale(c.Locale),
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
			sr.ClaudeMs, sr.Attempts, sr.Usage = res.LatencyMs.Claude, res.Meta.Attempts, res.Meta.Usage
			sr.FirstTokMs = res.FirstTokenMs
			if rec.Locale == "" {
				rec.Locale = string(res.Locale)
			}
			sr.Misses = checkChatStep(st, res)
			sr.OK = len(sr.Misses) == 0
			if !sr.OK {
				rec.OK = false
			}
			log.Printf("[%s] %s/%s turn=%d claude=%dms misses=%v text=%q sugg=%v transcript=%q",
				c.ID, res.Kind, res.Source, res.Turn, sr.ClaudeMs, sr.Misses, res.Text, res.Suggestions, res.Transcript)
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

func checkChatStep(st ChatStep, res *ai.ChatResult) []string {
	var miss []string
	if st.Expect != "" && res.Kind != st.Expect {
		miss = append(miss, "kind="+res.Kind+" (want "+st.Expect+")")
	}
	if st.Source != "" && res.Source != st.Source {
		miss = append(miss, "source="+res.Source+" (want "+st.Source+")")
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
