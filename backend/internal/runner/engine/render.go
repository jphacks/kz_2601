package engine

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// Run-time enforcement of notification text (#339: the run-time check is the
// one that counts). The template passed Validate, but inserted values are
// whatever users typed, so each one is masked and cut here.

// Mask replaces a URL / email / phone number inside an inserted value.
const Mask = "***"

// Missing is inserted for a value that is unavailable.
const Missing = "—"

var (
	placeholderRe = regexp.MustCompile(`\{([^{}]*)\}`)
	insertEmailRe = regexp.MustCompile(`[^\s@]*@[^\s@]*`)
	insertURLRe   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://\S*|www\.\S*|[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,}(/\S*)?)`)
	insertDigitRe = regexp.MustCompile(`[0-9][0-9\-\s().+]*[0-9]`)
	// insertableTypes may be inserted (the same set Validate allows).
	insertableTypes = []dsl.FieldType{dsl.FieldString, dsl.FieldText, dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency,
		dsl.FieldBoolean, dsl.FieldDate, dsl.FieldDatetime, dsl.FieldEnum}
)

const minPhoneDigits = 10

// render fills {field} / {$author} from the trigger record, sanitizes each
// inserted value and cuts the result to maxRunes.
func (p *planner) render(tmpl string, maxRunes int) string {
	s := placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		text, formatted := p.insert(m[1 : len(m)-1])
		if formatted {
			return text
		}
		return SanitizeInsert(text)
	})
	return cut(s, maxRunes)
}

// insert is the display text of one placeholder ("" when unavailable).
// formatted = the text was formatted here from a date / bool (no user text
// in it), so it is not masked (a date has 10+ digits).
func (p *planner) insert(id string) (text string, formatted bool) {
	if p.row == nil {
		return "", false
	}
	if id == dsl.RecordAuthor {
		if !p.row.Entity.IsShared() {
			return "", false
		}
		return p.row.Record.AuthorName, false
	}
	f := p.row.Entity.FieldByID(id)
	if f == nil || f.Computed != nil || f.Hidden || !slices.Contains(insertableTypes, f.Type) {
		return "", false
	}
	v := p.e.FieldValue(id, p.row)
	switch {
	case v.Kind == expreval.Date && f.Type == dsl.FieldDatetime:
		return v.Time.In(p.loc).Format("2006-01-02 15:04"), true
	case v.Kind == expreval.Date || v.Kind == expreval.Bool:
		return v.Text(p.loc), true
	}
	return v.Text(p.loc), false
}

// SanitizeInsert makes a user value safe to insert into a notification:
// NFKC, control / invisible characters removed (line breaks become spaces),
// URL / email / phone number masked, cut to dsl.MaxNotifyInsertRunes. An
// empty value becomes Missing.
func SanitizeInsert(s string) string {
	s = norm.NFKC.String(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	s = insertEmailRe.ReplaceAllString(s, Mask)
	s = insertURLRe.ReplaceAllString(s, Mask)
	s = insertDigitRe.ReplaceAllStringFunc(s, func(run string) string {
		n := 0
		for _, r := range run {
			if r >= '0' && r <= '9' {
				n++
			}
		}
		if n >= minPhoneDigits {
			return Mask
		}
		return run
	})
	s = strings.TrimSpace(s)
	if s == "" {
		return Missing
	}
	return cut(s, dsl.MaxNotifyInsertRunes)
}

// cut keeps the first n runes.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
