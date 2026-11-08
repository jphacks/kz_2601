package ai

import (
	"encoding/json"
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Book lookup switch (owner decision 2026-11-08): openBD's terms limit use to
// introducing books, so appconfig answers GET /v1/lookup/book with 503 unless
// BOOK_LOOKUP_ENABLED (books.EnabledFromEnv). While it is off the AI must not
// generate lookupBook buttons that can only fail: Service.NoBookLookup adds
// noBook to every request's Restriction (bookRules, the schema keys and the
// note go; a new use is retried; the rest is removed by enforceRestriction).
// Zero value (dev, ai-eval, tests) = the AI may use lookupBook as before.

// bookGate adds the server's book switch to a request's restriction.
func (s *Service) bookGate(r Restriction) Restriction {
	if !s.NoBookLookup || r.noBook {
		return r
	}
	r.noBook = true
	r.setKey()
	return r
}

// NoBookLookup reports whether lookupBook is unavailable.
func (r Restriction) NoBookLookup() bool { return r.noBook }

const noBookNote = "- ボタンの lookupBook(ISBN から本の情報を入れる)は使わない(今は使えない)。書名・著者は手で入力する形にし、頼まれたら「ISBN から本の情報を入れる機能は今は使えないため、手入力の形にしました。」と申告する。\n"

// restrictBookSchema drops the lookupBook action and Action.bookLookup.
func restrictBookSchema(defs map[string]any) {
	act, _ := defs["Action"].(map[string]any)
	props, _ := act["properties"].(map[string]any)
	delete(props, "bookLookup")
	tp, _ := props["type"].(map[string]any)
	if tp == nil {
		return
	}
	if en, ok := tp["enum"].([]any); ok {
		kept := []any{}
		for _, v := range en {
			if fmt.Sprint(v) != string(dsl.ActionLookupBook) {
				kept = append(kept, v)
			}
		}
		tp["enum"] = kept
	}
	if m, ok := tp[keyEnumDescs].(map[string]any); ok {
		delete(m, string(dsl.ActionLookupBook))
	}
}

func isLookupBook(c map[string]any) bool {
	a, _ := c["action"].(map[string]any)
	t, _ := a["type"].(string)
	return t == string(dsl.ActionLookupBook)
}

// bookButtons lists where raw has a lookupBook button / toolbar item
// (screen id + place, one string per use).
func bookButtons(raw []byte) []string {
	var root map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &root) != nil {
		return nil
	}
	var out []string
	eachBookScreen(root, func(id string, sc map[string]any) {
		var walk func(string, []any)
		walk = func(where string, cs []any) {
			for i, v := range cs {
				c, _ := v.(map[string]any)
				if c == nil {
					continue
				}
				at := fmt.Sprintf("%s[%d]", where, i)
				if isLookupBook(c) {
					out = append(out, id+" "+at)
				}
				if ch, ok := c["children"].([]any); ok {
					walk(at+".children", ch)
				}
			}
		}
		cs, _ := sc["components"].([]any)
		walk("components", cs)
		tb, _ := sc["toolbar"].([]any)
		for i, v := range tb {
			if b, _ := v.(map[string]any); b != nil && isLookupBook(b) {
				out = append(out, fmt.Sprintf("%s toolbar[%d]", id, i))
			}
		}
	})
	return out
}

// bookViolations: a turn may not add lookupBook buttons (counted, so moved
// ones are not new); ones already in base are removed by enforceRestriction.
func bookViolations(raw, base []byte, r Restriction) []string {
	if !r.noBook {
		return nil
	}
	now := bookButtons(raw)
	if len(now) <= len(bookButtons(base)) {
		return nil
	}
	var out []string
	for _, p := range now {
		out = append(out, fmt.Sprintf("screen %s: action lookupBook is not available now: remove the button and let the user type the book details", p))
	}
	return out
}

// unbook removes every lookupBook button and toolbar item; it returns how many.
func unbook(root map[string]any) int {
	n := 0
	var fix func([]any) []any
	fix = func(cs []any) []any {
		kept := make([]any, 0, len(cs))
		for _, v := range cs {
			c, _ := v.(map[string]any)
			if c != nil && isLookupBook(c) {
				n++
				continue
			}
			if c != nil {
				if ch, ok := c["children"].([]any); ok {
					c["children"] = fix(ch)
				}
			}
			kept = append(kept, v)
		}
		return kept
	}
	eachBookScreen(root, func(_ string, sc map[string]any) {
		if cs, ok := sc["components"].([]any); ok {
			sc["components"] = fix(cs)
		}
		if tb, ok := sc["toolbar"].([]any); ok {
			kept := make([]any, 0, len(tb))
			for _, v := range tb {
				if b, _ := v.(map[string]any); b != nil && isLookupBook(b) {
					n++
					continue
				}
				kept = append(kept, v)
			}
			sc["toolbar"] = kept
		}
	})
	return n
}

func eachBookScreen(root map[string]any, fn func(id string, sc map[string]any)) {
	screens, _ := root["screens"].([]any)
	for _, s := range screens {
		if sc, _ := s.(map[string]any); sc != nil {
			id, _ := sc["id"].(string)
			fn(fmt.Sprintf("%q", id), sc)
		}
	}
}
