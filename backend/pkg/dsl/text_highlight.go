package dsl

import (
	"strings"
	"unicode/utf8"
)

// text: `{{fieldId}}` shows the open record's value on screen.entity screens
// (form / detail), and highlight marks words in the shown text. iOS matches
// case-insensitively after NFKC, Japanese substrings included.

const (
	// TextHighlightMaxWords caps words (fixed list, and the words read from a field on iOS).
	TextHighlightMaxWords = 20
	// TextHighlightMaxWordLen caps one fixed word (characters).
	TextHighlightMaxWordLen = 50
)

// TextHighlight is text.highlight: exactly one of Words or Field.
type TextHighlight struct {
	Words []string `json:"words,omitempty"`
	Field string   `json:"field,omitempty"` // string / text field of screen.entity; words split by , 、 or newlines
	Style string   `json:"style,omitempty"` // marker (default) | bold
}

func checkText(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if c.Text == "" {
		add("%s: text is empty", where)
	}
	for _, m := range qrPlaceholderRe.FindAllStringSubmatch(c.Text, -1) {
		switch f := fieldOrNil(formEntity, m[1]); {
		case formEntity == nil:
			add("%s: text {{%s}} requires screen.entity (a form / detail screen)", where, m[1])
		case f == nil:
			add("%s: text {{%s}} is not a field of entity %q (fields: %s)", where, m[1], formEntity.ID, fieldIDs(formEntity))
		case qrUnencodable[f.Type]:
			add("%s: text {{%s}} is type %s; use a text-like field (or image / videoPlayer)", where, m[1], f.Type)
		case f.Hidden:
			add("%s: text {{%s}} is hidden — %s", where, m[1], hiddenHint)
		}
	}
	h := c.Highlight
	if h == nil {
		return
	}
	if (len(h.Words) > 0) == (h.Field != "") {
		add("%s: text.highlight takes exactly one of words (fixed words) or field (a field holding the words)", where)
	}
	if len(h.Words) > TextHighlightMaxWords {
		add("%s: text.highlight.words has %d words (max %d)", where, len(h.Words), TextHighlightMaxWords)
	}
	for i, w := range h.Words {
		if strings.TrimSpace(w) == "" || utf8.RuneCountInString(w) > TextHighlightMaxWordLen {
			add("%s: text.highlight.words[%d] must be 1-%d characters", where, i, TextHighlightMaxWordLen)
		}
	}
	if h.Field != "" {
		switch f := fieldOrNil(formEntity, h.Field); {
		case formEntity == nil:
			add("%s: text.highlight.field requires screen.entity (use words for fixed words)", where)
		case f == nil:
			add("%s: text.highlight.field %q is not a field of entity %q (fields: %s)", where, h.Field, formEntity.ID, fieldIDs(formEntity))
		case f.Type != FieldString && f.Type != FieldText:
			add("%s: text.highlight.field %q is type %s; use a string / text field", where, h.Field, f.Type)
		case f.Hidden:
			add("%s: text.highlight.field %q is hidden — %s", where, h.Field, hiddenHint)
		}
	}
	if h.Style != "" && h.Style != "marker" && h.Style != "bold" {
		add("%s: text.highlight.style %q (marker|bold)", where, h.Style)
	}
}

func fieldOrNil(e *Entity, id string) *Field {
	if e == nil {
		return nil
	}
	return e.FieldByID(id)
}
