package ai

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// On-demand rules: recipe blocks that only a few requests need (the JPHACKS
// product recipes) are not sent on every call. A rules file registers its
// block with
//
//	func init() { onDemandRules(xRules, recipe("id", words, parts), ...) }
//
// instead of `systemPrompt += "\n\n" + xRules`. The block is still appended
// to systemPrompt (later init rewrites and the rule tests see it as before),
// and with Service.OnDemandRules (env AI_ON_DEMAND_RULES=1) create / edit
// send onDemandCore(systemPrompt): the prompt without every registered block,
// identical for all requests, so it stays cached. The recipes the request
// matches go after it in Completion.SystemTail (uncached).
//
// A recipe matches when one of its words is in the transcript or in the
// existing app's text (edit / reference DSL), or one of its parts is in that
// app's compact JSON. A miss loses the recipe; a false hit only costs its
// bytes, so words are broad (TestOnDemandRecall_*). Off by default: on
// phrasings not used to write the words, recall is ~70-79% (under the 80%
// target), so the switch waits for a better selector or a live eval.

// onDemandHeading leads the tail: recipes are sent one per bullet.
const onDemandHeading = "## この依頼に近い作り方(過去の作品から。当てはまる所だけ使う)"

type onDemandRecipe struct {
	id    string
	text  string     // one "- " line of the block
	words [][]string // any entry matches: all of its groups, each any alternative
	parts []string   // substrings of the existing app's compact JSON
}

type onDemandBlock struct {
	text    string // as appended: systemPrompt += "\n\n" + text
	recipes []onDemandRecipe
}

var onDemandBlocks []onDemandBlock

// recipeTrigger is one recipe's triggers, in the order of the block's bullets.
type recipeTrigger struct {
	id    string
	words []string
	parts []string
}

// recipe builds a trigger. A word is "a|b" (any) and "x+y" (both), e.g.
// "体力|hp+タスク|課題". Words are compared after normalizeTrigger.
func recipe(id string, words []string, parts ...string) recipeTrigger {
	return recipeTrigger{id: id, words: words, parts: parts}
}

// onDemandRules appends block to systemPrompt like `+= "\n\n" + block` and
// registers its "- " lines as recipes, one trigger per line in order.
func onDemandRules(block string, triggers ...recipeTrigger) {
	var bullets []string
	for _, ln := range strings.Split(block, "\n") {
		if strings.HasPrefix(ln, "- ") {
			bullets = append(bullets, ln)
		}
	}
	if len(bullets) != len(triggers) {
		panic("onDemandRules: the block's bullets and triggers differ in number")
	}
	b := onDemandBlock{text: block}
	for i, tr := range triggers {
		r := onDemandRecipe{id: tr.id, text: bullets[i], parts: tr.parts}
		for _, w := range tr.words {
			var groups []string
			for _, g := range strings.Split(w, "+") {
				groups = append(groups, normalizeTrigger(g))
			}
			r.words = append(r.words, groups)
		}
		b.recipes = append(b.recipes, r)
	}
	systemPrompt += "\n\n" + block
	onDemandBlocks = append(onDemandBlocks, b)
}

var onDemandCoreMemo sync.Map // system -> system without the registered blocks

// onDemandCore drops every registered block from system (systemPrompt). A
// block a later init rewrote is not found and stays (TestOnDemandBlocks_FoundVerbatim).
func onDemandCore(system string) string {
	if len(onDemandBlocks) == 0 {
		return system
	}
	if v, ok := onDemandCoreMemo.Load(system); ok {
		return v.(string)
	}
	core := system
	for _, b := range onDemandBlocks {
		core = strings.Replace(core, "\n\n"+b.text, "", 1)
	}
	v, _ := onDemandCoreMemo.LoadOrStore(system, core)
	return v.(string)
}

// onDemandPrompt is the first system block and the tail of a create / edit
// turn. Off (Service.OnDemandRules unset) = the whole systemPrompt, no tail.
func (s *Service) onDemandPrompt(transcript string, app []byte) (system string, tail, ids []string) {
	if !s.OnDemandRules {
		return systemPrompt, nil, nil
	}
	tail, ids = onDemandTail(transcript, app)
	return onDemandCore(systemPrompt), tail, ids
}

// onDemandTail is the tail block of a request as pieces: the heading, then
// each matched recipe, every piece led by "\n\n" (so dropFeatureRules removes
// a feature's recipe in a restricted build). None matched = nil. app is the
// existing / reference DSL (nil on a plain create).
func onDemandTail(transcript string, app []byte) (pieces, ids []string) {
	text := normalizeTrigger(transcript) + appText(app)
	compact := ""
	if len(app) > 0 {
		compact = compactJSON(string(app))
	}
	for _, blk := range onDemandBlocks {
		for _, r := range blk.recipes {
			if r.matches(text, compact) {
				pieces = append(pieces, "\n\n"+r.text)
				ids = append(ids, r.id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return append([]string{"\n\n" + onDemandHeading}, pieces...), ids
}

// restrictTail joins the pieces, each restricted like a second block (per
// piece, so restrictPrompt's memo stays bounded by the recipes). A tail left
// with only its heading is dropped.
func restrictTail(ctx context.Context, pieces []string) string {
	if len(pieces) < 2 {
		return ""
	}
	var b strings.Builder
	for _, p := range pieces[1:] {
		b.WriteString(restrictPromptSuffix(ctx, p))
	}
	if strings.TrimSpace(b.String()) == "" {
		return ""
	}
	return restrictPromptSuffix(ctx, pieces[0]) + b.String()
}

func (r onDemandRecipe) matches(text, compact string) bool {
	for _, p := range r.parts {
		if compact != "" && strings.Contains(compact, p) {
			return true
		}
	}
next:
	for _, groups := range r.words {
		for _, g := range groups {
			if !hasAlt(text, g) {
				continue next
			}
		}
		return true
	}
	return false
}

func hasAlt(text, alts string) bool {
	for _, a := range strings.Split(alts, "|") {
		if a != "" && strings.Contains(text, a) {
			return true
		}
	}
	return false
}

// normalizeTrigger folds width (NFKC), case, katakana to hiragana and drops
// spaces, so 「ＨＰ」「hp」「H P」 and 「モンスター」「もんすたー」 compare equal.
func normalizeTrigger(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			continue
		case r >= 'ァ' && r <= 'ヶ':
			r -= 'ァ' - 'ぁ'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// appText is every string in the app's JSON (names, labels, options, ids),
// each normalized and led by a separator so no word spans two of them.
func appText(app []byte) string {
	if len(app) == 0 {
		return ""
	}
	var v any
	if json.Unmarshal(app, &v) != nil {
		return ""
	}
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString("\x1f" + normalizeTrigger(x))
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return b.String()
}
