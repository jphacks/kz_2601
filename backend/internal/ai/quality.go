package ai

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Content-quality helpers (quality audit #107 / #87): dsl.Lint warnings are
// fed back for one regeneration, plus small deterministic clean-ups.

// lintPrefix marks a problem line that came from dsl.Lint (valid document,
// wrong content) so the re-prompt says so instead of "validation failed".
const lintPrefix = "content: "

func lintProblems(warns []string) []string {
	out := make([]string, len(warns))
	for i, w := range warns {
		out[i] = lintPrefix + w
	}
	return out
}

func isLintOnly(problems []string) bool {
	for _, p := range problems {
		if !strings.HasPrefix(p, lintPrefix) {
			return false
		}
	}
	return len(problems) > 0
}

// lintOf parses raw (already valid) and returns its warnings; nil when it
// does not parse.
func lintOf(raw []byte) []string {
	app, err := dsl.Parse(raw)
	if err != nil {
		return nil
	}
	return app.Lint()
}

// indexRe drops array indexes so a warning keeps its identity when a patch
// inserts components before it.
var indexRe = regexp.MustCompile(`\[\d+\]`)

// newLint returns the warnings in after that were not already in before
// (edit turns only answer for what the patch introduced).
func newLint(before, after []string) []string {
	seen := map[string]int{}
	for _, w := range before {
		seen[indexRe.ReplaceAllString(w, "[]")]++
	}
	var out []string
	for _, w := range after {
		k := indexRe.ReplaceAllString(w, "[]")
		if seen[k] > 0 {
			seen[k]--
			continue
		}
		out = append(out, w)
	}
	return out
}

// normalizeDigits turns full-width digits into ASCII ("２タブ" -> "2タブ") in
// text read aloud / shown by iOS (#87-5).
var digitWidth = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9")

func normalizeDigits(s string) string { return digitWidth.Replace(s) }

// dropFormPhotoPreviews removes an `image` bound to a photo field that a
// photoField on the same form screen already binds: photoField shows its own
// preview, so the photo appeared twice (#87-3). Invalid JSON is returned as is.
func dropFormPhotoPreviews(raw []byte) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	changed := false
	screens, _ := doc["screens"].([]any)
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		if e, _ := sm["entity"].(string); e == "" {
			continue
		}
		comps, _ := sm["components"].([]any)
		photo := map[string]bool{}
		eachComp(comps, func(c map[string]any) {
			if c["type"] == "photoField" {
				if f, _ := c["field"].(string); f != "" {
					photo[f] = true
				}
			}
		})
		if len(photo) == 0 {
			continue
		}
		dup := func(c map[string]any) bool {
			f, _ := c["field"].(string)
			return c["type"] == "image" && photo[f]
		}
		var kept []any
		for _, c := range comps {
			cm, _ := c.(map[string]any)
			if cm != nil && dup(cm) {
				changed = true
				continue
			}
			if children, ok := cm["children"].([]any); ok {
				var kc []any
				for _, ch := range children {
					if chm, _ := ch.(map[string]any); chm != nil && dup(chm) {
						changed = true
						continue
					}
					kc = append(kc, ch)
				}
				if kc == nil {
					kc = []any{}
				}
				cm["children"] = kc
			}
			kept = append(kept, c)
		}
		if kept == nil {
			kept = []any{}
		}
		sm["components"] = kept
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

func eachComp(comps []any, fn func(map[string]any)) {
	for _, c := range comps {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		fn(cm)
		children, _ := cm["children"].([]any)
		eachComp(children, fn)
	}
}
