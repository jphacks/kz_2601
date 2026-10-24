package ai

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Locale is the language of the generated UI strings and of the spoken
// summary. The DSL structure (ids, types, enums) is locale-independent.
type Locale string

const (
	LocaleJA Locale = "ja"
	LocaleEN Locale = "en"
)

// resolveLocale returns the explicit locale when valid, otherwise the locale
// detected from the transcript. Unknown values are a 400.
func resolveLocale(explicit Locale, transcript string) (Locale, error) {
	switch explicit {
	case "":
		return DetectLocale(transcript), nil
	case LocaleJA, LocaleEN:
		return explicit, nil
	default:
		return "", &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "locale must be ja or en"}
	}
}

// DetectLocale is the character-class heuristic: any kana or kanji means
// Japanese; a transcript without them (Latin only) is English. Empty is ja.
func DetectLocale(transcript string) Locale {
	if strings.TrimSpace(transcript) == "" || hasJapanese(transcript) {
		return LocaleJA
	}
	return LocaleEN
}

// hasJapanese reports whether s contains hiragana, katakana or kanji.
func hasJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

// localeNames is the line the user turn carries so the (static, cached)
// system prompt can refer to "the requested locale".
var localeNames = map[Locale]string{
	LocaleJA: "ja(日本語)",
	LocaleEN: "en (English): write every user-facing string in natural English",
}

// summaryText is the locale-dependent wording of the spoken summary.
type summaryText struct {
	nothingToUndo string
	restored      string // fmt with version
	readyText     string // clarify phase: spoken when the brief is complete
}

var summaryTexts = map[Locale]summaryText{
	LocaleJA: {nothingToUndo: "戻せる変更がありません。", restored: "ひとつ前の状態(version %d)に戻しました。", readyText: "では作りますね。"},
	LocaleEN: {nothingToUndo: "There is nothing to undo.", restored: "I went back to the previous state (version %d).", readyText: "Got it, I'll build it now."},
}

func (l Locale) text() summaryText {
	if t, ok := summaryTexts[l]; ok {
		return t
	}
	return summaryTexts[LocaleJA]
}

// summarizeEN is the English counterpart of summarize(): "I made a budget
// app. Use the button at the bottom right to add expenses. You'll get a
// reminder every morning at 8."
func summarizeEN(a *dsl.App) string {
	var b strings.Builder
	name := strings.TrimSpace(a.App.Name)
	if !strings.HasSuffix(strings.ToLower(name), "app") {
		name += " app"
	}
	fmt.Fprintf(&b, "I made %s.", withArticle(name))
	if n := len(a.App.Tabs); n >= 2 {
		titles := make([]string, 0, n)
		for _, t := range a.App.Tabs {
			titles = append(titles, t.Title)
		}
		fmt.Fprintf(&b, " It has %d tabs: %s.", n, joinEN(titles, "and"))
	}
	if len(a.Entities) > 0 {
		names := make([]string, 0, len(a.Entities))
		for _, e := range a.Entities {
			names = append(names, pluralEN(strings.ToLower(e.Name)))
		}
		if hints := addHints(a, addButtonEN, addToolbarEN); len(hints) > 0 {
			parts := make([]string, 0, len(hints))
			for _, h := range hints {
				plural := make([]string, 0, len(h.entities))
				for _, n := range h.entities {
					plural = append(plural, pluralEN(strings.ToLower(n)))
				}
				parts = append(parts, fmt.Sprintf("%s to add %s", h.where, joinEN(plural, "or")))
			}
			fmt.Fprintf(&b, " Use %s.", joinEN(parts, "and"))
		} else {
			fmt.Fprintf(&b, " It keeps track of %s.", joinEN(names, "and"))
		}
	} else {
		b.WriteString(" It's ready to use.")
	}
	for _, n := range a.App.Notifications {
		if when := notificationWhenEN(n); when != "" {
			fmt.Fprintf(&b, " You'll get a reminder %s.", when)
		}
	}
	if note := substitutionNoteEN(a.App.Description); note != "" {
		b.WriteString(" " + note)
	}
	return b.String()
}

// substitutionNoteEN mirrors substitutionNote: the prompt asks Claude to
// write "X was replaced with Y." in app.description when a requested part
// is not in the catalog.
func substitutionNoteEN(description string) string {
	var out []string
	for _, sentence := range strings.SplitAfter(description, ".") {
		if strings.Contains(strings.ToLower(sentence), "replaced") {
			out = append(out, strings.TrimSpace(sentence))
		}
	}
	return strings.Join(out, " ")
}

// withArticle prefixes "a"/"an" by the first letter.
func withArticle(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("aeiouAEIOU", rune(s[0])) {
		return "an " + s
	}
	return "a " + s
}

// uncountableEN are entity names that read wrong with an "s" ("add spendings").
var uncountableEN = map[string]bool{"spending": true, "money": true, "water": true, "food": true, "income": true, "equipment": true, "homework": true, "info": true, "data": true, "sleep": true}

// pluralEN is the naive English plural: entry -> entries, expense -> expenses.
// Uncountable last words and -ing nouns stay as they are (spending, reading).
func pluralEN(s string) string {
	last := s[strings.LastIndex(s, " ")+1:]
	switch {
	case s == "" || uncountableEN[last] || strings.HasSuffix(last, "ing"):
		return s
	case strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsRune("aeiou", rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

// joinEN joins "a", "a and b", "a, b and c".
func joinEN(items []string, conj string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " " + conj + " " + items[len(items)-1]
	}
}

// addButtonEN / addToolbarEN phrase where an add control is ("the button at
// the bottom right", `the "Add member" button at the top right`).
func addButtonEN(c *dsl.Component) string {
	if pos := areaPositionEN(c.Area); pos != "" {
		return "the button at the " + pos
	}
	return "the " + strings.Trim(c.Label, " ") + " button"
}

func addToolbarEN(tb *dsl.ToolbarButton) string {
	return `the "` + tb.Label + `" button at the top right`
}

// areaPositionEN turns a grid rectangle into "bottom right" / "top left" / "bottom" / "".
func areaPositionEN(area string) string {
	ar, err := dsl.ParseArea(area)
	if err != nil {
		return ""
	}
	v, h := "", ""
	switch {
	case ar.Row2 <= 4:
		v = "top"
	case ar.Row1 >= 9:
		v = "bottom"
	}
	switch {
	case ar.Col2 <= 2:
		h = "left"
	case ar.Col1 >= 3:
		h = "right"
	}
	return strings.TrimSpace(v + " " + h)
}

// notificationWhenEN renders "every morning at 8" / "every evening at 9:30" /
// "every day at 1 pm" / "every week at 8" / "at 8" / "".
func notificationWhenEN(n dsl.Notification) string {
	var hh, mm int
	if _, err := fmt.Sscanf(n.Time, "%d:%d", &hh, &mm); err != nil {
		if n.Repeat == "weekly" {
			return "every week"
		}
		return ""
	}
	h12 := hh % 12
	if h12 == 0 {
		h12 = 12
	}
	clock := fmt.Sprintf("at %d", h12)
	if mm != 0 {
		clock = fmt.Sprintf("at %d:%02d", h12, mm)
	}
	switch n.Repeat {
	case "daily":
		switch {
		case hh >= 18:
			return "every evening " + clock
		case hh < 11:
			return "every morning " + clock
		default:
			return "every day " + clock + ampm(hh)
		}
	case "weekly":
		return "every week " + clock
	default:
		return clock
	}
}

func ampm(hh int) string {
	if hh >= 12 {
		return " pm"
	}
	return " am"
}
