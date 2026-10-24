// Package ai turns a transcript into a DSL change: full AppDSL for the first
// generation, JSON Patch for edits, both validated by pkg/dsl with a bounded
// regenerate-on-problems loop and an area-stripping fallback.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Mode is the requested generation mode.
type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeCreate Mode = "create"
	ModeEdit   Mode = "edit"
)

// Request is POST /v1/generate.
type Request struct {
	AppID       string `json:"appId"`
	BaseVersion int    `json:"baseVersion,omitempty"`
	Transcript  string `json:"transcript"`
	Text        string `json:"text,omitempty"` // alias used by openapi-ai.yaml
	Mode        Mode   `json:"mode,omitempty"`
}

// Latency is per-stage wall time in ms.
type Latency struct {
	Claude int64 `json:"claude"`
	Total  int64 `json:"total"`
}

// Meta is diagnostic detail about how the result was produced.
type Meta struct {
	Model            string   `json:"model"`
	Attempts         int      `json:"attempts"`      // Claude calls made
	Regenerations    int      `json:"regenerations"` // attempts - 1 (validation retries)
	AreaFallback     bool     `json:"areaFallback"`  // areas stripped to pass validation
	DowngradedToFull bool     `json:"downgradedToFull,omitempty"`
	Usage            Usage    `json:"usage"`
	LastProblems     []string `json:"lastProblems,omitempty"`
}

// Result is the 200 body of POST /v1/generate.
type Result struct {
	AppID           string          `json:"appId"`
	Version         int             `json:"version"`
	PreviousVersion int             `json:"previousVersion"`
	Mode            string          `json:"mode"` // full|patch
	DSL             json.RawMessage `json:"dsl"`
	Patch           []dsl.PatchOp   `json:"patch,omitempty"`
	Summary         string          `json:"summary"`
	Transcript      string          `json:"transcript"`
	Source          string          `json:"source"` // claude
	LatencyMs       Latency         `json:"latencyMs"`
	Meta            Meta            `json:"meta"`
}

// Error is a failure mapped to an HTTP status and error code.
type Error struct {
	Status   int
	Code     string
	Message  string
	Problems []string
	Attempts int
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Service wires Claude and the DSL store.
type Service struct {
	Claude           Caller
	Store            appconfigclient.Store
	Model            string
	MaxRegenerations int           // validation-driven retries after the first attempt (default 2)
	CallTimeout      time.Duration // per Claude call (default 40s)
}

func (s *Service) maxRegen() int {
	if s.MaxRegenerations < 0 {
		return 0
	}
	if s.MaxRegenerations == 0 {
		return 2
	}
	return s.MaxRegenerations
}

type attemptResult struct {
	raw     []byte // validated DSL (full) or next DSL (patch)
	ops     []dsl.PatchOp
	summary string
	meta    Meta
	latency time.Duration
}

// Generate runs the pipeline: fetch current -> decide mode -> Claude ->
// validate (retry) -> fallback -> save -> result.
func (s *Service) Generate(ctx context.Context, req Request) (*Result, error) {
	started := time.Now()
	transcript := strings.TrimSpace(req.Transcript)
	if transcript == "" {
		transcript = strings.TrimSpace(req.Text)
	}
	if req.AppID == "" || transcript == "" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "appId and transcript are required"}
	}
	env, err := s.Store.Get(ctx, req.AppID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	current, err := dsl.Parse(env.DSL)
	if err != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "current DSL is invalid: " + err.Error()}
	}
	mode := req.Mode
	if mode == "" || mode == ModeAuto {
		if isInitial(current) || wantsNewApp(transcript) {
			mode = ModeCreate
		} else {
			mode = ModeEdit
		}
	}

	var ar *attemptResult
	switch mode {
	case ModeCreate:
		ref := ""
		if !isInitial(current) {
			ref = string(env.DSL)
		}
		ar, err = s.generateFull(ctx, transcript, ref, nil)
	case ModeEdit:
		ar, err = s.generateEdit(ctx, transcript, env)
	default:
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "mode must be auto|create|edit"}
	}
	if err != nil {
		return nil, err
	}

	src := appconfigclient.Source{Kind: "ai", Transcript: transcript, Summary: ar.summary, Model: s.Model}
	var saved *appconfigclient.Envelope
	if len(ar.ops) > 0 && !ar.meta.AreaFallback {
		src.Mode = "patch"
		saved, err = s.Store.Patch(ctx, req.AppID, env.Version, ar.ops, src)
	} else {
		src.Mode = "full"
		saved, err = s.Store.Put(ctx, req.AppID, env.Version, ar.raw, src)
	}
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if len(saved.DSL) == 0 {
		saved.DSL = ar.raw
	}
	res := &Result{
		AppID:           req.AppID,
		Version:         saved.Version,
		PreviousVersion: env.Version,
		Mode:            src.Mode,
		DSL:             saved.DSL,
		Summary:         ar.summary,
		Transcript:      transcript,
		Source:          "claude",
		Meta:            ar.meta,
	}
	if src.Mode == "patch" {
		res.Patch = ar.ops
	}
	res.LatencyMs = Latency{Claude: ar.latency.Milliseconds(), Total: time.Since(started).Milliseconds()}
	return res, nil
}

// generateFull asks for a whole AppDSL; on validation problems it re-prompts
// up to maxRegen times, then strips areas, then fails with the problems.
func (s *Service) generateFull(ctx context.Context, transcript, refDSL string, meta *Meta) (*attemptResult, error) {
	ar := &attemptResult{}
	if meta != nil {
		ar.meta = *meta
	}
	ar.meta.Model = s.Model
	var problems []string
	var lastCandidate []byte
	for i := 0; i <= s.maxRegen(); i++ {
		resp, err := s.call(ctx, systemPrompt, createUserMessage(transcript, refDSL, problems), GenerationSchema(), ar)
		if err != nil {
			return nil, err
		}
		raw := []byte(resp.Text)
		app, perr := dsl.Parse(raw)
		if perr == nil {
			ar.raw = canon(raw)
			ar.summary = summarize(app)
			return ar, nil
		}
		lastCandidate = raw
		problems = problemsOf(perr, resp)
		ar.meta.LastProblems = problems
	}
	if stripped, ok := stripAreas(lastCandidate); ok {
		if app, perr := dsl.Parse(stripped); perr == nil {
			ar.raw = stripped
			ar.summary = summarize(app)
			ar.meta.AreaFallback = true
			return ar, nil
		}
	}
	return nil, &Error{Status: http.StatusBadRequest, Code: "generation_failed",
		Message: "could not produce a valid app from the request", Problems: problems, Attempts: ar.meta.Attempts}
}

// generateEdit asks for a PatchDocument, validates with dsl.ApplyPatch, retries
// with problems, then tries area-stripping, then downgrades to a full rewrite.
func (s *Service) generateEdit(ctx context.Context, transcript string, env *appconfigclient.Envelope) (*attemptResult, error) {
	ar := &attemptResult{}
	ar.meta.Model = s.Model
	var problems []string
	var lastOps []dsl.PatchOp
	var lastSummary string
	for i := 0; i <= s.maxRegen(); i++ {
		resp, err := s.call(ctx, systemPrompt+"\n"+editRules,
			editUserMessage(transcript, env.Version, string(env.DSL), problems), PatchGenerationSchema(), ar)
		if err != nil {
			return nil, err
		}
		var doc dsl.PatchDocument
		if uerr := json.Unmarshal([]byte(resp.Text), &doc); uerr != nil {
			problems = []string{"output is not a valid PatchDocument: " + uerr.Error()}
			if resp.StopReason == "max_tokens" {
				problems = append(problems, "output was truncated (max_tokens); make the patch smaller")
			}
			ar.meta.LastProblems = problems
			continue
		}
		next, _, perr := dsl.ApplyPatch(env.DSL, doc.Patch)
		if perr == nil {
			ar.raw = next
			ar.ops = doc.Patch
			ar.summary = strings.TrimSpace(doc.Summary)
			return ar, nil
		}
		lastOps, lastSummary = doc.Patch, doc.Summary
		problems = problemsOf(perr, resp)
		ar.meta.LastProblems = problems
	}
	// Fallback 1: apply the last patch unchecked, drop areas, re-validate.
	if len(lastOps) > 0 {
		if next, ok := applyUnchecked(env.DSL, lastOps); ok {
			if stripped, ok := stripAreas(next); ok {
				if _, perr := dsl.Parse(stripped); perr == nil {
					ar.raw = stripped
					ar.ops = lastOps
					ar.summary = strings.TrimSpace(lastSummary)
					ar.meta.AreaFallback = true
					return ar, nil
				}
			}
		}
	}
	// Fallback 2: one full rewrite with the current DSL as reference.
	meta := ar.meta
	meta.DowngradedToFull = true
	full := &Service{Claude: s.Claude, Store: s.Store, Model: s.Model, MaxRegenerations: -1, CallTimeout: s.CallTimeout}
	fr, err := full.generateFull(ctx, transcript, string(env.DSL), &meta)
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) {
			ge.Problems = append(problems, ge.Problems...)
			ge.Attempts = ar.meta.Attempts + 1
		}
		return nil, err
	}
	return fr, nil
}

// call performs one Claude request and folds usage/attempts/latency into ar.
func (s *Service) call(ctx context.Context, system, user string, schema json.RawMessage, ar *attemptResult) (*Response, error) {
	timeout := s.CallTimeout
	if timeout == 0 {
		timeout = 40 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.Now()
	resp, err := s.Claude.Complete(cctx, Completion{System: system, Messages: []Message{{Role: "user", Content: user}}, Schema: schema})
	ar.latency += time.Since(t)
	ar.meta.Attempts++
	ar.meta.Regenerations = ar.meta.Attempts - 1
	if err != nil {
		return nil, mapClaudeErr(err)
	}
	ar.meta.Usage.Add(resp.Usage)
	if resp.StopReason == "refusal" {
		msg := "the model declined this request"
		if resp.StopDetails != nil && resp.StopDetails.Category != "" {
			msg += " (" + resp.StopDetails.Category + ")"
		}
		return nil, &Error{Status: http.StatusBadRequest, Code: "refused", Message: msg, Attempts: ar.meta.Attempts}
	}
	return resp, nil
}

func problemsOf(err error, resp *Response) []string {
	var ve *dsl.ValidationError
	if errors.As(err, &ve) {
		p := append([]string(nil), ve.Problems...)
		if resp.StopReason == "max_tokens" {
			p = append(p, "output was truncated (max_tokens); produce a smaller app")
		}
		return p
	}
	return []string{err.Error()}
}

func applyUnchecked(current []byte, ops []dsl.PatchOp) ([]byte, bool) {
	raw, err := json.Marshal(ops)
	if err != nil {
		return nil, false
	}
	p, err := jsonpatch.DecodePatch(raw)
	if err != nil {
		return nil, false
	}
	next, err := p.Apply(current)
	if err != nil {
		return nil, false
	}
	return next, true
}

func canon(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

// isInitial reports whether the app is still the voice-only seed.
func isInitial(a *dsl.App) bool {
	if len(a.Entities) > 0 || len(a.Screens) != 1 {
		return false
	}
	for _, c := range a.Screens[0].Components {
		if c.Type != dsl.CompVoiceInput {
			return false
		}
	}
	return true
}

var newAppPhrases = []string{"新しいアプリ", "作り直して", "作りなおして", "別のアプリ", "ゼロから", "一から"}

func wantsNewApp(t string) bool {
	for _, p := range newAppPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// summarize renders the Japanese summary for a full generation
// (edit turns get summary straight from the PatchDocument).
func summarize(a *dsl.App) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%sを作りました。", a.App.Name)
	if n := len(a.App.Tabs); n >= 2 {
		titles := make([]string, 0, n)
		for _, t := range a.App.Tabs {
			titles = append(titles, t.Title)
		}
		fmt.Fprintf(&b, "%sの%dタブ構成で、", strings.Join(titles, "/"), n)
	} else {
		titles := make([]string, 0, len(a.Screens))
		for _, s := range a.Screens {
			if s.Title != "" {
				titles = append(titles, s.Title)
			}
		}
		if len(titles) > 0 {
			fmt.Fprintf(&b, "%sの%d画面で、", strings.Join(titles, "/"), len(a.Screens))
		}
	}
	if len(a.Entities) > 0 {
		names := make([]string, 0, len(a.Entities))
		for _, e := range a.Entities {
			names = append(names, e.Name)
		}
		fmt.Fprintf(&b, "%sを記録できます。", strings.Join(names, "・"))
	} else {
		b.WriteString("すぐに使えます。")
	}
	for _, n := range a.App.Notifications {
		if n.Time != "" {
			when := map[string]string{"daily": "毎日", "weekly": "毎週", "once": ""}[n.Repeat]
			fmt.Fprintf(&b, "%s%sに「%s」の通知を送ります。", when, n.Time, n.Title)
		}
	}
	return b.String()
}

func mapStoreErr(err error) error {
	var ae *appconfigclient.Error
	if errors.As(err, &ae) {
		code := ae.Code
		if code == "" {
			code = "appconfig_error"
		}
		return &Error{Status: ae.Status, Code: code, Message: ae.Message}
	}
	return &Error{Status: http.StatusBadGateway, Code: "appconfig_error", Message: err.Error()}
}

func mapClaudeErr(err error) error {
	var ge *Error
	if errors.As(err, &ge) {
		return ge
	}
	var ae *APIError
	if errors.As(err, &ae) {
		switch {
		case ae.Status == 429:
			return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Claude rate limit"}
		case ae.Status == 400 || ae.Status == 401 || ae.Status == 403:
			return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "Claude request rejected: " + ae.Message}
		default:
			return &Error{Status: http.StatusServiceUnavailable, Code: "upstream_unavailable", Message: "Claude unavailable: " + ae.Type}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Status: http.StatusGatewayTimeout, Code: "upstream_timeout", Message: "Claude call timed out"}
	}
	return &Error{Status: http.StatusServiceUnavailable, Code: "upstream_unavailable", Message: err.Error()}
}
