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
	"reflect"
	"strings"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Mode is the requested generation mode.
type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeCreate Mode = "create"
	ModeEdit   Mode = "edit"
	ModeUndo   Mode = "undo" // restore the previous version, no Claude call
)

// Request is POST /v1/generate.
type Request struct {
	AppID       string `json:"appId"`
	BaseVersion int    `json:"baseVersion,omitempty"`
	Transcript  string `json:"transcript"`
	Text        string `json:"text,omitempty"` // alias used by openapi-ai.yaml
	Mode        Mode   `json:"mode,omitempty"`
	Locale      Locale `json:"locale,omitempty"` // ja|en; empty = detect from the transcript
	// LogicPass overrides Service.LogicPass for this request: true = UI pass
	// then a second call that adds calculations (logic.go). Create turns only.
	LogicPass *bool `json:"logicPass,omitempty"`
	// AllowedComponents (#245): component types (container + leaf) the client
	// can render; omitted/empty = all. DisabledFeatures: fieldHealth |
	// cameraCapture (unknown values ignored). See restrict.go.
	AllowedComponents []string `json:"allowedComponents,omitempty"`
	DisabledFeatures  []string `json:"disabledFeatures,omitempty"`
}

// Latency is per-stage wall time in ms.
type Latency struct {
	Claude int64 `json:"claude"`
	Total  int64 `json:"total"`
}

// Meta is diagnostic detail about how the result was produced.
type Meta struct {
	// PublicPersonalData (#253): personal-data-like fields that became readable
	// by anyone in this generation (iOS can ask before keeping them public).
	PublicPersonalData []PublicPersonalData `json:"publicPersonalData,omitempty"`
	// Diagnostics below.
	Model            string     `json:"model"`
	Attempts         int        `json:"attempts"`      // Claude calls made
	Regenerations    int        `json:"regenerations"` // attempts - 1 (validation retries)
	AreaFallback     bool       `json:"areaFallback"`  // areas stripped to pass validation
	DowngradedToFull bool       `json:"downgradedToFull,omitempty"`
	Usage            Usage      `json:"usage"`
	OutputChars      int        `json:"outputChars,omitempty"` // model text length (whitespace share diagnostics)
	LastProblems     []string   `json:"lastProblems,omitempty"`
	LogicPass        *LogicMeta `json:"logicPass,omitempty"` // set when the logic pass ran
	// LintRetried: the first valid output had content warnings (dsl.Lint) and
	// was regenerated once with them; Lint lists the warnings left in the
	// returned document (create) or added by the patch (edit).
	LintRetried bool     `json:"lintRetried,omitempty"`
	Lint        []string `json:"lint,omitempty"`
	// Omissions (#257): requested parts the app does not do and the model did
	// not mention; the summary got a fixed sentence for each.
	Omissions []string `json:"omissions,omitempty"`
	// Restricted (#245): what the build restriction removed from the output
	// after generation (disallowed parts, Field.health, camera sources).
	Restricted []string `json:"restricted,omitempty"`
}

// Event is a progress message emitted by GenerateStream before the Result,
// as soon as the first top-level block of the model output is complete:
// create turns send `app` (name / description / tabs + a provisional spoken
// line), edit turns send `summary`. iOS can show and read them out while the
// rest of the DSL is still being generated.
type Event struct {
	Type        string   `json:"type"` // app|summary
	Attempt     int      `json:"attempt"`
	Locale      Locale   `json:"locale"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Tabs        []string `json:"tabs,omitempty"`
	Summary     string   `json:"summary,omitempty"`
}

// Sink receives Events in order, on the calling goroutine.
type Sink func(Event)

// Result is the 200 body of POST /v1/generate.
type Result struct {
	AppID           string          `json:"appId"`
	Version         int             `json:"version"`
	PreviousVersion int             `json:"previousVersion"`
	Mode            string          `json:"mode"` // full|patch|restore
	RestoredFrom    int             `json:"restoredFrom,omitempty"`
	DSL             json.RawMessage `json:"dsl"`
	Patch           []dsl.PatchOp   `json:"patch,omitempty"`
	Summary         string          `json:"summary"`
	Locale          Locale          `json:"locale"` // language of the UI strings and summary
	Transcript      string          `json:"transcript"`
	Source          string          `json:"source"` // claude|restore
	// Unchanged (#211): the generation produced no change (only `test` ops, or
	// the same DSL). Nothing is saved: version == previousVersion and summary
	// says why (「〜は変更できませんでした。理由: …。代わりに…」). Omitted when false.
	Unchanged bool    `json:"unchanged,omitempty"`
	LatencyMs Latency `json:"latencyMs"`
	Meta      Meta    `json:"meta"`
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
	// LogicPass enables the second "calculations only" call after a create
	// (env AI_LOGIC_PASS=1; Request.LogicPass overrides per request).
	LogicPass bool
	// LintRetry regenerates once when the first valid output has content
	// warnings (dsl.Lint). Off by default: warnings are only reported in
	// Meta.Lint, and the user fixes the app by voice instead (env AI_LINT_RETRY=1).
	LintRetry bool
	// Chat is the model setup of the clarify turn (chat.go); zero = defaults.
	ChatConfig ChatConfig
	// Verifier checks the caller's ID token on /v1/chat (#286): the
	// conversation is kept per uid, and DELETE /v1/chat/{appId} needs one.
	// nil = no caller is ever verified (DELETE answers 401).
	Verifier auth.TokenVerifier
	AuthMode string // auth.ModeDev also accepts X-Debug-Uid with the emulator

	conv     *convStore // clarify conversations (chat.go); lazily created
	convOnce sync.Once
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
	// forceFull: the DSL was changed after the patch (build restriction), so
	// it is saved as a full document instead of the model's ops.
	forceFull bool
}

// Generate runs the pipeline: fetch current -> decide mode -> Claude ->
// validate (retry) -> fallback -> save -> result.
func (s *Service) Generate(ctx context.Context, req Request) (*Result, error) {
	return s.GenerateStream(ctx, req, nil)
}

// GenerateStream is Generate with Claude streaming: sink (if non-nil) gets
// an Event as soon as the app block / summary has been generated; the Result
// is still validated and saved before it is returned.
func (s *Service) GenerateStream(ctx context.Context, req Request, sink Sink) (*Result, error) {
	started := time.Now()
	restrict := NewRestriction(req.AllowedComponents, req.DisabledFeatures)
	ctx = withRestriction(ctx, restrict)
	transcript := strings.TrimSpace(req.Transcript)
	if transcript == "" {
		transcript = strings.TrimSpace(req.Text)
	}
	if req.AppID == "" || transcript == "" {
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "appId and transcript are required"}
	}
	loc, err := resolveLocale(req.Locale, transcript)
	if err != nil {
		return nil, err
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
		switch {
		case wantsUndo(transcript):
			mode = ModeUndo
		case isInitial(current) || wantsNewApp(transcript):
			mode = ModeCreate
		default:
			mode = ModeEdit
		}
	}
	if mode == ModeUndo {
		return s.undo(ctx, req.AppID, transcript, env, started, loc)
	}

	var ar *attemptResult
	logic := s.logicEnabled(req)
	switch mode {
	case ModeCreate:
		ref := ""
		if !isInitial(current) {
			ref = string(env.DSL)
		}
		ar, err = s.generateFull(ctx, transcript, ref, nil, loc, sink, logic)
		if err == nil && logic {
			s.applyLogicPass(ctx, transcript, ar, loc)
		}
		if err == nil {
			ar.summary = withShareNote(ar.summary, transcript, nil, ar.raw, loc) // #259 #215
			notePublicPII(ar, nil, loc)                                          // #253
		}
	case ModeEdit:
		ar, err = s.generateEdit(ctx, transcript, env, loc, sink)
		if err == nil {
			ar.summary = withShareNote(ar.summary, transcript, env.DSL, ar.raw, loc) // #259 #215
			notePublicPII(ar, env.DSL, loc)                                          // #253
		}
	default:
		return nil, &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "mode must be auto|create|edit|undo"}
	}
	if err != nil {
		return nil, err
	}
	if err := applyRestriction(ar, restrict, mode == ModeCreate, loc); err != nil {
		return nil, err
	}

	s.noteOmissions(transcript, ar, loc)

	if mode == ModeEdit && len(ar.ops) > 0 && sameDSL(env.DSL, ar.raw) {
		// #211: a no-op is not saved as a new version; the summary tells the user why.
		summary := ar.summary
		if summary == "" {
			summary = loc.text().unchanged
		}
		return &Result{
			AppID: req.AppID, Version: env.Version, PreviousVersion: env.Version, Mode: "patch",
			DSL: env.DSL, Summary: summary, Locale: loc, Transcript: transcript, Source: "claude",
			Meta: ar.meta, Unchanged: true,
			LatencyMs: Latency{Claude: ar.latency.Milliseconds(), Total: time.Since(started).Milliseconds()},
		}, nil
	}
	src := appconfigclient.Source{Kind: "ai", Transcript: transcript, Summary: ar.summary, Model: s.Model}
	var saved *appconfigclient.Envelope
	if len(ar.ops) > 0 && !ar.meta.AreaFallback && !ar.forceFull {
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
		Locale:          loc,
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

// noteOmissions appends a fixed sentence to the summary for each part of the
// request the final app leaves out without saying so (#257).
func (s *Service) noteOmissions(transcript string, ar *attemptResult, loc Locale) {
	app, err := dsl.Parse(ar.raw)
	if err != nil {
		return
	}
	om := omissions(transcript, app, ar.summary+"\n"+app.App.Description, true)
	if len(om) == 0 {
		return
	}
	ar.meta.Omissions = omissionIDs(om)
	note := omissionNote(om, transcript, loc)
	switch {
	case ar.summary == "":
		ar.summary = note
	case loc == LocaleEN:
		ar.summary = strings.TrimSpace(ar.summary) + " " + note
	default:
		ar.summary = strings.TrimSpace(ar.summary) + note
	}
}

// generateFull asks for a whole AppDSL; on validation problems it re-prompts
// up to maxRegen times, then strips areas, then fails with the problems.
func (s *Service) generateFull(ctx context.Context, transcript, refDSL string, meta *Meta, loc Locale, sink Sink, uiOnly bool) (*attemptResult, error) {
	ar := &attemptResult{}
	if meta != nil {
		ar.meta = *meta
	}
	ar.meta.Model = s.Model
	var problems []string
	var lastCandidate []byte
	var linted *attemptResult // valid output kept while regenerating for its lint warnings
	for i := 0; i <= s.maxRegen(); i++ {
		resp, err := s.call(ctx, systemPrompt, createUserMessage(transcript, refDSL, problems, loc, uiOnly), GenerationSchema(), ar,
			appEventWatcher(sink, loc, ar.meta.Attempts+1))
		if err != nil {
			return nil, err
		}
		raw := dropFormPhotoPreviews(fillDefaults(extractJSON(resp.Text)))
		raw = fixButtonOverlap(raw) // before validation: saves a regeneration (#169)
		app, perr := dsl.Parse(raw)
		if perr == nil && linted == nil && i < s.maxRegen() {
			// #245: a part this build cannot render is a validation problem;
			// on the last attempt it is accepted and removed by applyRestriction.
			if v := restrictionViolations(raw, nil, restrictionFrom(ctx)); len(v) > 0 {
				lastCandidate = raw
				problems = v
				ar.meta.LastProblems = problems
				continue
			}
		}
		if perr == nil {
			// Post-processing order (integration of #173 and #174; each step re-parses):
			//   1. addMissingLists / fixEnglishUnits: existing content fixes.
			//   2. fixLayoutRules (#174): moves mic-seat buttons right or into the
			//      toolbar and fixes date titles on the model's own layout.
			//   3. addMissingAddButtons (#173): runs after 2 so it sees the final
			//      openers (a button moved to the toolbar still counts) and never
			//      adds a toolbar button that 2 would then dedupe or move.
			//   4. summarizeFor below (summary hints, #173) last, so the spoken
			//      button positions describe the final layout.
			if fixed := addMissingLists(raw, app); len(fixed) != len(raw) {
				if fixedApp, err := dsl.Parse(fixed); err == nil {
					raw, app = fixed, fixedApp
				}
			}
			if fixed := fixEnglishUnits(raw, loc); len(fixed) != len(raw) {
				if fixedApp, err := dsl.Parse(fixed); err == nil {
					raw, app = fixed, fixedApp
				}
			}
			if fixed := fixLayoutRules(raw, app); string(fixed) != string(raw) {
				if fixedApp, err := dsl.Parse(fixed); err == nil {
					raw, app = fixed, fixedApp
				}
			}
			if fixed := addMissingAddButtons(raw, app, loc); len(fixed) != len(raw) {
				if fixedApp, err := dsl.Parse(fixed); err == nil {
					raw, app = fixed, fixedApp
				}
			}
			if fixed := fixStatUnitLabels(raw); len(fixed) != len(raw) {
				if fixedApp, err := dsl.Parse(fixed); err == nil {
					raw, app = fixed, fixedApp
				}
			}
			ar.raw = canon(raw)
			ar.summary = summarizeFor(app, loc)
			ar.meta.Lint = app.Lint()
			// #257: parts of the request dropped without a word count as content problems too
			om := omissions(transcript, app, app.App.Description, !uiOnly)
			lintRetry := s.LintRetry && len(ar.meta.Lint)+len(om) > 0
			// #254: "家族だけ" without a shared entity gets one retry even with LintRetry off
			shareProblem := shareCreateProblem(transcript, ar.raw)
			if (lintRetry || shareProblem != "") && linted == nil && i < s.maxRegen() {
				// one retry for content problems; keep this output in case it fails
				kept := *ar
				linted = &kept
				problems = nil
				if lintRetry {
					ar.meta.LintRetried = true
					problems = lintProblems(append(append([]string(nil), ar.meta.Lint...), omissionProblems(om)...))
				}
				if shareProblem != "" {
					problems = append(problems, shareProblem)
				}
				ar.meta.LastProblems = problems
				continue
			}
			return ar, nil
		}
		if linted != nil {
			break // the lint retry broke the document: keep the earlier valid one
		}
		lastCandidate = raw
		problems = problemsOf(perr, resp)
		ar.meta.LastProblems = problems
	}
	if linted != nil {
		ar.raw, ar.summary, ar.meta.Lint = linted.raw, linted.summary, linted.meta.Lint
		return ar, nil
	}
	if stripped, ok := stripAreas(lastCandidate); ok {
		if app, perr := dsl.Parse(stripped); perr == nil {
			ar.raw = stripped
			ar.summary = summarizeFor(app, loc)
			ar.meta.AreaFallback = true
			return ar, nil
		}
	}
	return nil, &Error{Status: http.StatusBadRequest, Code: "generation_failed",
		Message: "could not produce a valid app from the request", Problems: problems, Attempts: ar.meta.Attempts}
}

// generateEdit asks for a PatchDocument, validates with dsl.ApplyPatch, retries
// with problems, then tries area-stripping, then downgrades to a full rewrite.
func (s *Service) generateEdit(ctx context.Context, transcript string, env *appconfigclient.Envelope, loc Locale, sink Sink) (*attemptResult, error) {
	ar := &attemptResult{}
	ar.meta.Model = s.Model
	var problems []string
	var lastOps []dsl.PatchOp
	var lastSummary string
	var linted *attemptResult
	baseLint := lintOf(env.DSL)
	for i := 0; i <= s.maxRegen(); i++ {
		resp, err := s.call(ctx, systemPrompt+"\n"+editRules,
			editUserMessage(transcript, env.Version, string(env.DSL), problems, loc), PatchGenerationSchema(), ar,
			summaryEventWatcher(sink, loc, ar.meta.Attempts+1))
		if err != nil {
			return nil, err
		}
		var doc dsl.PatchDocument
		uerr := json.Unmarshal(extractJSON(resp.Text), &doc)
		if uerr != nil && linted != nil {
			break
		}
		if uerr != nil {
			problems = []string{"output is not a valid PatchDocument: " + uerr.Error()}
			if resp.StopReason == "max_tokens" {
				problems = append(problems, "output was truncated (max_tokens); make the patch smaller")
			}
			ar.meta.LastProblems = problems
			continue
		}
		doc.Patch = fillPatchDefaults(doc.Patch, env.DSL)
		next, _, perr := dsl.ApplyPatch(env.DSL, doc.Patch)
		if perr == nil && linted == nil && i < s.maxRegen() {
			// #245: the patch may not add parts this build cannot render
			// (ones already in the app are removed by applyRestriction).
			if v := restrictionViolations(next, env.DSL, restrictionFrom(ctx)); len(v) > 0 {
				lastOps, lastSummary = doc.Patch, doc.Summary
				problems = v
				ar.meta.LastProblems = problems
				continue
			}
		}
		if perr == nil {
			ar.raw = next
			ar.ops = doc.Patch
			ar.summary = normalizeDigits(strings.TrimSpace(doc.Summary))
			ar.meta.Lint = newLint(baseLint, lintOf(next))
			lintRetry := s.LintRetry && len(ar.meta.Lint) > 0
			// #259: a share request answered with "できない" gets one retry even with LintRetry off
			shareProblem := shareEditProblem(transcript, env.DSL, next)
			if (lintRetry || shareProblem != "") && linted == nil && i < s.maxRegen() {
				kept := *ar
				linted = &kept
				problems = nil
				if lintRetry {
					ar.meta.LintRetried = true
					problems = lintProblems(ar.meta.Lint)
				}
				if shareProblem != "" {
					problems = append(problems, shareProblem)
				}
				ar.meta.LastProblems = problems
				continue
			}
			return ar, nil
		}
		if linted != nil {
			break
		}
		lastOps, lastSummary = doc.Patch, doc.Summary
		problems = problemsOf(perr, resp)
		ar.meta.LastProblems = problems
	}
	if linted != nil {
		ar.raw, ar.ops, ar.summary, ar.meta.Lint = linted.raw, linted.ops, linted.summary, linted.meta.Lint
		return ar, nil
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
	fr, err := full.generateFull(ctx, transcript, string(env.DSL), &meta, loc, sink, false)
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

// applyRestriction (#245) removes what the build cannot render from the final
// DSL (after retries, logic pass and fallbacks). A changed patch result is
// saved as a full document; create turns get a fresh summary.
func applyRestriction(ar *attemptResult, r Restriction, create bool, loc Locale) error {
	out, notes, changed := enforceRestriction(ar.raw, r)
	if !changed {
		return nil
	}
	app, err := dsl.Parse(out)
	if err != nil {
		return &Error{Status: http.StatusBadRequest, Code: "generation_failed",
			Message:  "could not produce a valid app without the parts this app version cannot use",
			Problems: append(notes, err.Error()), Attempts: ar.meta.Attempts}
	}
	ar.raw = canon(out)
	ar.forceFull = true
	ar.meta.Restricted = notes
	if create || ar.summary == "" {
		ar.summary = summarizeFor(app, loc)
	}
	return nil
}

// call performs one Claude request and folds usage/attempts/latency into ar.
// onText (nil = non-streaming) receives the text deltas.
func (s *Service) call(ctx context.Context, system, user string, schema json.RawMessage, ar *attemptResult, onText func(string)) (*Response, error) {
	timeout := s.CallTimeout
	if timeout == 0 {
		timeout = 40 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.Now()
	system, schema = restrictPrompt(ctx, system), restrictSchema(ctx, schema)
	resp, err := s.Claude.Complete(cctx, Completion{System: system, Messages: []Message{{Role: "user", Content: user}}, Schema: schema, OnText: onText})
	ar.latency += time.Since(t)
	ar.meta.Attempts++
	ar.meta.Regenerations = ar.meta.Attempts - 1
	if err != nil {
		return nil, mapClaudeErr(err)
	}
	ar.meta.Usage.Add(resp.Usage)
	ar.meta.OutputChars += len(resp.Text)
	if resp.StopReason == "refusal" {
		msg := "the model declined this request"
		if resp.StopDetails != nil && resp.StopDetails.Category != "" {
			msg += " (" + resp.StopDetails.Category + ")"
		}
		return nil, &Error{Status: http.StatusBadRequest, Code: "refused", Message: msg, Attempts: ar.meta.Attempts}
	}
	return resp, nil
}

// appEventWatcher returns the OnText hook for a create turn: when the
// top-level `app` object has been streamed, it emits one Event with the
// name / tabs and a provisional spoken line. nil sink = no streaming.
func appEventWatcher(sink Sink, loc Locale, attempt int) func(string) {
	if sink == nil {
		return nil
	}
	w := newKeyWatcher("app")
	return func(delta string) {
		raw, ok := w.Feed(delta)
		if !ok {
			return
		}
		var meta dsl.AppMeta
		if err := json.Unmarshal(raw, &meta); err != nil || strings.TrimSpace(meta.Name) == "" {
			return
		}
		ev := Event{Type: "app", Attempt: attempt, Locale: loc, Name: meta.Name, Description: meta.Description}
		for _, t := range meta.Tabs {
			ev.Tabs = append(ev.Tabs, t.Title)
		}
		ev.Summary = draftSummary(&meta, loc)
		sink(ev)
	}
}

// summaryEventWatcher is the edit-turn counterpart: the PatchDocument is
// written summary-first, so the spoken line is available before the ops.
func summaryEventWatcher(sink Sink, loc Locale, attempt int) func(string) {
	if sink == nil {
		return nil
	}
	w := newKeyWatcher("summary")
	return func(delta string) {
		raw, ok := w.Feed(delta)
		if !ok {
			return
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
			return
		}
		sink(Event{Type: "summary", Attempt: attempt, Locale: loc, Summary: strings.TrimSpace(s)})
	}
}

// draftSummary is the provisional line read out while the screens are still
// being generated: 「家計簿アプリを作っています。ホームと集計の2タブです。」 /
// "Making a Budget Book app with 2 tabs: Home and Stats."
func draftSummary(m *dsl.AppMeta, loc Locale) string {
	titles := make([]string, 0, len(m.Tabs))
	for _, t := range m.Tabs {
		titles = append(titles, t.Title)
	}
	if loc == LocaleEN {
		name := strings.TrimSpace(m.Name)
		if !strings.HasSuffix(strings.ToLower(name), "app") {
			name += " app"
		}
		s := "Making " + withArticle(name)
		if len(titles) >= 2 {
			s += fmt.Sprintf(" with %d tabs: %s", len(titles), joinEN(titles, "and"))
		}
		return s + "."
	}
	name := m.Name
	if !strings.HasSuffix(name, "アプリ") {
		name += "アプリ"
	}
	s := name + "を作っています。"
	if len(titles) >= 2 {
		s += fmt.Sprintf("%sの%dタブです。", joinJa(titles), len(titles))
	}
	return s
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

// extractJSON tolerates a code fence or a short preamble around the JSON
// (needed when the request runs without output_config.format). Only the first
// JSON value is taken, so trailing prose ("...}Wait, I also changed X") does
// not fail with "invalid character after top-level value".
func extractJSON(text string) []byte {
	t := strings.TrimSpace(text)
	start := strings.Index(t, "{")
	if start < 0 {
		return []byte(t)
	}
	var first json.RawMessage
	if err := json.NewDecoder(strings.NewReader(t[start:])).Decode(&first); err == nil {
		return first
	}
	if end := strings.LastIndex(t, "}"); end > start {
		return []byte(t[start : end+1])
	}
	return []byte(t)
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

// newAppPhrases anywhere in the utterance switch an existing app to a full
// rebuild (ja and en; matched case-insensitively).
var newAppPhrases = []string{
	"新しいアプリ", "作り直して", "作りなおして", "別のアプリ", "ゼロから", "一から",
	"new app", "different app", "another app", "start over", "start again", "from scratch", "rebuild",
}

func wantsNewApp(t string) bool {
	s := strings.ToLower(t)
	for _, p := range newAppPhrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// undoPrefixes are matched at the start of a short utterance only, so edits
// such as 「一覧を元の位置に戻して」 / "put the list back at the top" or long
// sentences are never mistaken for undo.
var undoPrefixes = []string{
	"元に戻し", "もとに戻し", "元へ戻し", "戻して",
	"取り消し", "取消", "アンドゥ", "undo",
	"さっきのやめ", "さっきのをやめ", "さっきの変更をやめ", "さっきの変更を戻", "さっきの変更を元に戻",
	"さっきのを戻", "さっきのを元に戻", "さっきのを取り消", "さっきの取り消",
	"やっぱりやめ", "やっぱやめ",
	"一つ前に戻", "ひとつ前に戻", "1つ前に戻", "一つ前の状態に戻", "ひとつ前の状態に戻", "前の状態に戻",
	// en ("undo" above also covers "undo that" / "undo the last change")
	"revert", "roll back", "rollback", "go back", "take that back", "take it back",
	"scrap that", "cancel that", "never mind", "nevermind", "forget that", "forget it",
}

// undoMaxRunes / undoMaxRunesEN bound the utterance length: 16 runes fits
// 「さっきの変更を元に戻して」, 32 fits "go back to the previous version".
const (
	undoMaxRunes   = 16
	undoMaxRunesEN = 32
)

// wantsUndo is the keyword classifier for 「元に戻して」 / "undo". No Claude
// call: the utterance must be short and start with one of undoPrefixes.
func wantsUndo(t string) bool {
	s := strings.ToLower(strings.TrimSpace(t))
	s = strings.TrimRight(s, "。．.!！?？、,…　 ")
	limit := undoMaxRunes
	if !hasJapanese(s) {
		limit = undoMaxRunesEN
	}
	if s == "" || len([]rune(s)) > limit {
		return false
	}
	for _, p := range undoPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// undo restores the state before the current version. When the current
// version is itself a restore, it steps back from the version it restored
// (so repeated 「戻して」 keeps going back instead of redoing). Version 1 is
// the voice-only seed, so there is nothing to undo below version 2.
func (s *Service) undo(ctx context.Context, appID, transcript string, env *appconfigclient.Envelope, started time.Time, loc Locale) (*Result, error) {
	versions, err := s.Store.Versions(ctx, appID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	target := env.Version - 1
	for _, v := range versions {
		if v.Version == env.Version && v.Source != nil && v.Source.Kind == "restore" && v.Source.RestoredFrom > 0 {
			target = v.Source.RestoredFrom - 1
		}
	}
	res := &Result{
		AppID:           appID,
		Version:         env.Version,
		PreviousVersion: env.Version,
		Mode:            "restore",
		DSL:             env.DSL,
		Locale:          loc,
		Transcript:      transcript,
		Source:          "restore",
		Meta:            Meta{Model: s.Model},
	}
	if target < 1 {
		res.Summary = loc.text().nothingToUndo
		res.LatencyMs = Latency{Total: time.Since(started).Milliseconds()}
		return res, nil
	}
	saved, err := s.Store.Restore(ctx, appID, target)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	res.Version = saved.Version
	res.RestoredFrom = target
	res.DSL = saved.DSL
	res.Summary = fmt.Sprintf(loc.text().restored, target)
	res.LatencyMs = Latency{Total: time.Since(started).Milliseconds()}
	return res, nil
}

// summarizeFor picks the summary renderer for the locale (ja is the default).
func summarizeFor(a *dsl.App, loc Locale) string {
	if loc == LocaleEN {
		return summarizeEN(a)
	}
	return normalizeDigits(summarize(a))
}

// summarize renders the Japanese summary for a full generation
// (edit turns get summary straight from the PatchDocument). It is read aloud
// by iOS, so it is phrased as speech: "家計簿アプリを作りました。ホームと集計の
// 2タブで、右下のボタンから支出を追加できます。毎朝8時に通知が届きます。"
func summarize(a *dsl.App) string {
	var b strings.Builder
	name := a.App.Name
	if !strings.HasSuffix(name, "アプリ") {
		name += "アプリ"
	}
	fmt.Fprintf(&b, "%sを作りました。", name)
	if n := len(a.App.Tabs); n >= 2 {
		titles := make([]string, 0, n)
		for _, t := range a.App.Tabs {
			titles = append(titles, t.Title)
		}
		fmt.Fprintf(&b, "%sの%dタブで、", joinJa(titles), n)
	}
	if len(a.Entities) > 0 {
		names := make([]string, 0, len(a.Entities))
		for _, e := range a.Entities {
			names = append(names, e.Name)
		}
		if hints := addHints(a, addButtonJa, addToolbarJa); len(hints) > 0 {
			parts := make([]string, 0, len(hints))
			for _, h := range hints {
				parts = append(parts, fmt.Sprintf("%sから%sを", h.where, joinJa(h.entities)))
			}
			fmt.Fprintf(&b, "%s追加できます。", strings.Join(parts, "、"))
		} else {
			fmt.Fprintf(&b, "%sを記録できます。", joinJa(names))
		}
	} else {
		b.WriteString("すぐに使えます。")
	}
	for _, n := range a.App.Notifications {
		if when := notificationWhen(n); when != "" {
			fmt.Fprintf(&b, "%sに通知が届きます。", when)
		}
	}
	b.WriteString(editNoteJa(a))
	b.WriteString(substitutionNote(a.App.Description))
	return b.String()
}

// substitutionNote extracts the "○○は△△で代替しました。" sentences the prompt
// asks Claude to leave in app.description when a requested part is not in the
// catalog (create turns have no model-written summary; edit turns put it in
// PatchDocument.summary directly).
// #257: "○○は今はできません。" sentences are read out the same way.
func substitutionNote(description string) string {
	return strings.Join(honestNote(description, "。"), "")
}

// joinJa joins 2 items with と, more with ・.
func joinJa(items []string) string {
	if len(items) == 2 {
		return items[0] + "と" + items[1]
	}
	return strings.Join(items, "・")
}

// addButtonJa / addToolbarJa phrase where an add control is ("右下のボタン",
// "右上の「メンバー追加」").
func addButtonJa(c *dsl.Component) string { return areaPosition(c.Area) + "ボタン" }

func addToolbarJa(tb *dsl.ToolbarButton) string { return "右上の「" + tb.Label + "」" }

// areaPosition turns a grid rectangle into "右下の" / "左上の" / "下の" / "".
func areaPosition(area string) string {
	ar, err := dsl.ParseArea(area)
	if err != nil {
		return ""
	}
	v, h := "", ""
	switch {
	case ar.Row2 <= 4:
		v = "上"
	case ar.Row1 >= 9:
		v = "下"
	}
	switch {
	case ar.Col2 <= 2:
		h = "左"
	case ar.Col1 >= 3:
		h = "右"
	}
	if v == "" && h == "" {
		return ""
	}
	return h + v + "の"
}

// notificationWhen renders "毎朝8時" / "毎晩9時" / "毎日12時" / "毎週" / "".
func notificationWhen(n dsl.Notification) string {
	var hh, mm int
	if _, err := fmt.Sscanf(n.Time, "%d:%d", &hh, &mm); err != nil {
		if n.Repeat == "weekly" {
			return "毎週"
		}
		return ""
	}
	clock := fmt.Sprintf("%d時", hh)
	switch {
	case hh >= 18:
		clock = fmt.Sprintf("%d時", hh-12)
	case hh == 0:
		clock = "0時"
	}
	if mm != 0 {
		clock += fmt.Sprintf("%d分", mm)
	}
	switch n.Repeat {
	case "daily":
		switch {
		case hh >= 18:
			return "毎晩" + clock
		case hh < 11:
			return "毎朝" + clock
		default:
			return "毎日" + clock
		}
	case "weekly":
		return "毎週" + clock
	default:
		return clock
	}
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
		switch ae.Status {
		case 429:
			return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Claude rate limit"}
		case 400, 401, 403:
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

// sameDSL reports whether two DSL documents are equal as JSON values (key
// order and whitespace ignored).
func sameDSL(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
