package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// The recipe selector: one small-model call that picks the on-demand recipes
// (on_demand_rules.go) a request needs, from the transcript and the app's
// names against a one-line index of the recipes. Its picks are added to the
// keyword hits. It starts as soon as generateStream knows the app and is
// waited for at most RecipeTimeout; on timeout or error the keywords alone
// decide, so it can only add recipes, never block a generation.

// Selector defaults (Service.RecipeModel / RecipeTimeout; env AI_RECIPE_MODEL).
const (
	DefaultRecipeModel   = "claude-haiku-5-5"
	DefaultRecipeTimeout = 1500 * time.Millisecond
	recipeMaxTokens      = 256
	recipeMaxTranscript  = 1000 // runes sent of the transcript
	recipeMaxNames       = 40   // entity names / component types sent
)

// OpRecipes labels the selector call in ai.usage.
const OpRecipes = "recipes"

// Selector outcomes (Meta.RecipeSelector).
const (
	recipeSelOK      = "ok"
	recipeSelTimeout = "timeout"
	recipeSelError   = "error"
	recipeSelInvalid = "invalid" // the reply was not the schema
)

func (s *Service) recipeModel() string {
	if s.RecipeModel != "" {
		return s.RecipeModel
	}
	return DefaultRecipeModel
}

func (s *Service) recipeTimeout() time.Duration {
	if s.RecipeTimeout > 0 {
		return s.RecipeTimeout
	}
	return DefaultRecipeTimeout
}

// recipeThinking turns thinking off on Haiku 5.5 (a pick from a list: lowest
// latency; accepted at effort low). Other models keep their default: Opus 5.5
// and Sonnet 5.5 reject "disabled".
func recipeThinking(model string) string {
	if strings.HasPrefix(model, "claude-haiku") {
		return "disabled"
	}
	return ""
}

// recipeDesc is the index line of a recipe: its bullet up to the first
// ": " after a quote (the requests it is for), e.g.
// 「枠を選んで申し込む」「1 つの枠に 1 人だけ」(面談・相談・レッスン).
func recipeDesc(text string) string {
	d := strings.TrimPrefix(text, "- ")
	if i := strings.Index(d, ": "); i > 0 {
		d = d[:i]
	}
	return d
}

var recipeIndexOnce = sync.OnceValues(func() (string, json.RawMessage) {
	var b strings.Builder
	b.WriteString(`あなたは振り分け係です。アプリを作る・直す依頼に、下の「作り方」の一覧のどれが役に立つかを選びます。
- 依頼(と今のアプリ)に近いものの id をすべて選ぶ。言い方が違っても、やりたいことが同じなら選ぶ。
- 迷ったら選ぶ(余分に選んでも害は小さい。抜けると作り方が分からない)。
- どれも当てはまらなければ空の配列。
- 答えは {"ids":[...]} だけ。

## 作り方の一覧(id: どんな依頼の時か)
`)
	var ids []any
	for _, blk := range onDemandBlocks {
		for _, r := range blk.recipes {
			b.WriteString(r.id + ": " + recipeDesc(r.text) + "\n")
			ids = append(ids, r.id)
		}
	}
	schema, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ids": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": ids}},
		},
		"required":             []string{"ids"},
		"additionalProperties": false,
	})
	if err != nil {
		panic("ai: recipe schema: " + err.Error())
	}
	return b.String(), schema
})

// recipeUserMessage is the transcript plus, for an existing app, its name,
// entity names and component types (no records, no prompt texts).
func recipeUserMessage(transcript string, app []byte) string {
	if r := []rune(transcript); len(r) > recipeMaxTranscript {
		transcript = string(r[:recipeMaxTranscript])
	}
	msg := "依頼: " + transcript
	name, entities, comps := appOutline(app)
	if name != "" || len(entities) > 0 || len(comps) > 0 {
		msg += "\n今のアプリ: " + name +
			"\nデータ: " + strings.Join(entities, ", ") +
			"\n部品: " + strings.Join(comps, ", ")
	}
	return msg
}

// appOutline reads the app name, entity names (or ids) and the component
// types used anywhere in the screens, sorted and capped.
func appOutline(app []byte) (name string, entities, comps []string) {
	if len(app) == 0 {
		return "", nil, nil
	}
	var doc struct {
		App struct {
			Name string `json:"name"`
		} `json:"app"`
		Entities []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entities"`
		Screens []json.RawMessage `json:"screens"`
	}
	if json.Unmarshal(app, &doc) != nil {
		return "", nil, nil
	}
	for _, e := range doc.Entities {
		n := e.Name
		if n == "" {
			n = e.ID
		}
		if n != "" && len(entities) < recipeMaxNames {
			entities = append(entities, n)
		}
	}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if cs, ok := x["components"].([]any); ok {
				for _, c := range cs {
					if m, ok := c.(map[string]any); ok {
						if t, ok := m["type"].(string); ok && t != "" {
							seen[t] = true
						}
					}
				}
			}
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, sc := range doc.Screens {
		var v any
		if json.Unmarshal(sc, &v) == nil {
			walk(v)
		}
	}
	for t := range seen {
		comps = append(comps, t)
	}
	sort.Strings(comps)
	if len(comps) > recipeMaxNames {
		comps = comps[:recipeMaxNames]
	}
	return doc.App.Name, entities, comps
}

// recipePick is a selector call in flight.
type recipePick struct {
	done     chan struct{}
	deadline time.Time
	start    time.Time
	ids      []string // set before done closes
	status   string
	dur      time.Duration
}

type recipePickKey struct{}

func withRecipePick(ctx context.Context, p *recipePick) context.Context {
	return context.WithValue(ctx, recipePickKey{}, p)
}

func recipePickFrom(ctx context.Context) *recipePick {
	p, _ := ctx.Value(recipePickKey{}).(*recipePick)
	return p
}

// startRecipePick sends the selector call in the background. Its usage goes
// to ai.usage and the request's cost tally (operation recipes).
func (s *Service) startRecipePick(ctx context.Context, transcript string, app []byte) *recipePick {
	timeout := s.recipeTimeout()
	p := &recipePick{done: make(chan struct{}), start: time.Now()}
	p.deadline = p.start.Add(timeout)
	system, schema := recipeIndexOnce()
	user := recipeUserMessage(transcript, app)
	go func() {
		defer close(p.done)
		cctx, cancel := context.WithTimeout(WithOperation(ctx, OpRecipes), timeout)
		defer cancel()
		resp, err := s.Claude.Complete(cctx, Completion{
			System:     system,
			Messages:   []Message{{Role: "user", Content: user}},
			Schema:     schema,
			Structured: true,
			MaxTokens:  recipeMaxTokens,
			Model:      s.recipeModel(),
			Effort:     "low",
			Thinking:   recipeThinking(s.recipeModel()),
		})
		p.dur = time.Since(p.start)
		switch {
		case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded)):
			p.status = recipeSelTimeout
		case err != nil:
			p.status = recipeSelError
		default:
			p.ids, p.status = parseRecipeIDs(resp.Text)
		}
	}()
	return p
}

// wait returns the picks, or none with "timeout" once the deadline passes
// (a slow call keeps running and is dropped).
func (p *recipePick) wait() (ids []string, status string, dur time.Duration) {
	t := time.NewTimer(time.Until(p.deadline) + 50*time.Millisecond)
	defer t.Stop()
	select {
	case <-p.done:
		return p.ids, p.status, p.dur
	case <-t.C:
		return nil, recipeSelTimeout, time.Since(p.start)
	}
}

// parseRecipeIDs reads {"ids":[...]}, keeping known ids once each.
func parseRecipeIDs(text string) ([]string, string) {
	var out struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(extractJSON(text), &out); err != nil {
		return nil, recipeSelInvalid
	}
	known := map[string]bool{}
	for _, b := range onDemandBlocks {
		for _, r := range b.recipes {
			known[r.id] = true
		}
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range out.IDs {
		if known[id] && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, recipeSelOK
}
