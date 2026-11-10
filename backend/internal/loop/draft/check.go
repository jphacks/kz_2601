package draft

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// Limits of what is posted.
const (
	MaxTitleRunes = 120
	MaxBodyRunes  = 6000
	// OverlapRunes: a run this long (after normalising) shared with any
	// source text counts as quoting the feedback (#366 検査器 2: 原文の一致).
	OverlapRunes = 16
)

var (
	reURL     = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://|www\.|\b[a-z0-9\-]+\.(?:com|net|org|io|dev|app|jp|co|me|ai|xyz)\b`)
	reMention = regexp.MustCompile(`(^|[^A-Za-z0-9])@[A-Za-z0-9_\-]`)
	reHTML    = regexp.MustCompile(`<[a-zA-Z/!][^>]*>|&[a-zA-Z#0-9]+;`)
	reImage   = regexp.MustCompile(`!\[[^\]]*\]\(`)
	reLabel   = regexp.MustCompile(`(?i)\b(?:agent|needs|eval)\s*:\s*[a-z]`)
)

// Check is the exit check for a draft (#366 検査器 2, #367): it returns the
// reasons (codes) title/body may not be posted to a public repository, or
// nil. sources are the masked texts of the draft's items; the draft must not
// quote them. Human edits go through the same check.
func Check(title, body string, sources []string) []string {
	var out []string
	add := func(r string) {
		for _, x := range out {
			if x == r {
				return
			}
		}
		out = append(out, r)
	}
	if strings.TrimSpace(title) == "" {
		add("empty_title")
	}
	if utf8.RuneCountInString(title) > MaxTitleRunes || strings.Contains(title, "\n") {
		add("title_shape")
	}
	if utf8.RuneCountInString(body) > MaxBodyRunes {
		add("too_long")
	}
	for _, s := range []string{title, body} {
		switch {
		case pii.HasInvisible(s):
			add("invisible_chars")
		case reURL.MatchString(s):
			add("url")
		case reImage.MatchString(s):
			add("image")
		case reHTML.MatchString(s):
			add("html")
		case reMention.MatchString(s):
			add("mention")
		case reLabel.MatchString(s):
			add("label_like")
		}
		if len(pii.Find(s)) > 0 {
			add("pii")
		}
		if strings.Contains(s, "fb_") || strings.Contains(s, "lin_") {
			add("internal_id")
		}
	}
	if quotes(title+"\n"+body, sources) {
		add("quotes_feedback")
	}
	return out
}

// quotes reports whether text shares a run of OverlapRunes normalised
// characters with any source.
func quotes(text string, sources []string) bool {
	t := []rune(triage.NormalizeText(text))
	if len(t) < OverlapRunes {
		return false
	}
	norm := make([]string, 0, len(sources))
	for _, s := range sources {
		if n := triage.NormalizeText(s); utf8.RuneCountInString(n) >= OverlapRunes {
			norm = append(norm, n)
		}
	}
	for i := 0; i+OverlapRunes <= len(t); i++ {
		w := string(t[i : i+OverlapRunes])
		for _, s := range norm {
			if strings.Contains(s, w) {
				return true
			}
		}
	}
	return false
}
