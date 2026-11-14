// Package textfilter refuses obviously objectionable words in text that other
// people see (#321, App Review 1.2): shared records, member names, listings.
// It is a minimal word list, not a classifier; reports and operator
// unpublishing (docs/infra/moderation.md) handle the rest.
package textfilter

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Category is why a text was refused. Log it, never the text.
type Category string

const (
	Slur   Category = "slur"
	Sexual Category = "sexual"
	Threat Category = "threat"
)

const categoryAllow = "allow"

//go:embed terms.txt
var termsTxt string

type term struct {
	cat    Category
	text   string   // non-ASCII: substring of the normalized text
	words  []string // ASCII: consecutive whole words
	prefix bool     // ASCII: the last word may continue
}

// Filter matches normalized text against a term list.
type Filter struct {
	native []term
	ascii  []term
	allow  []string
}

var (
	defaultOnce   sync.Once
	defaultFilter *Filter
)

// Default is the filter of the embedded terms.txt.
func Default() *Filter {
	defaultOnce.Do(func() {
		f, err := Parse(termsTxt)
		if err != nil {
			panic(err) // terms.txt is embedded and tested
		}
		defaultFilter = f
	})
	return defaultFilter
}

// Check is Default().Check.
func Check(s string) (Category, bool) { return Default().Check(s) }

// Parse reads the terms.txt format.
func Parse(src string) (*Filter, error) {
	f := &Filter{}
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cat, raw, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("terms line %d: want <category> <term>", i+1)
		}
		prefix := strings.HasSuffix(raw, "*")
		text := Normalize(strings.TrimSuffix(raw, "*"))
		if text == "" {
			return nil, fmt.Errorf("terms line %d: empty term", i+1)
		}
		switch Category(cat) {
		case Slur, Sexual, Threat:
		default:
			if cat != categoryAllow {
				return nil, fmt.Errorf("terms line %d: unknown category %q", i+1, cat)
			}
			f.allow = append(f.allow, strings.ReplaceAll(text, " ", ""))
			continue
		}
		if isASCII(text) {
			f.ascii = append(f.ascii, term{cat: Category(cat), words: strings.Fields(text), prefix: prefix})
			continue
		}
		if prefix {
			return nil, fmt.Errorf("terms line %d: * is for ASCII terms only", i+1)
		}
		f.native = append(f.native, term{cat: Category(cat), text: strings.ReplaceAll(text, " ", "")})
	}
	return f, nil
}

// Check reports whether s contains a term, and its category.
func (f *Filter) Check(s string) (Category, bool) {
	n := Normalize(s)
	if n == "" {
		return "", false
	}
	for _, a := range f.allow {
		n = strings.ReplaceAll(n, a, " ")
	}
	for _, t := range f.native {
		if strings.Contains(n, t.text) {
			return t.cat, true
		}
	}
	if len(f.ascii) == 0 {
		return "", false
	}
	words := strings.FieldsFunc(n, func(r rune) bool { return !isWordRune(r) })
	for _, t := range f.ascii {
		if hasWords(words, t) {
			return t.cat, true
		}
	}
	return "", false
}

func hasWords(words []string, t term) bool {
	k := len(t.words)
	for i := 0; i+k <= len(words); i++ {
		ok := slices.Equal(words[i:i+k-1], t.words[:k-1])
		last := words[i+k-1]
		if ok && (last == t.words[k-1] || (t.prefix && strings.HasPrefix(last, t.words[k-1]))) {
			return true
		}
	}
	return false
}

// Normalize folds the forms one word can be written in: NFKC (full-width
// letters, half-width kana), lower case, hiragana to katakana. Format
// characters are dropped; spaces, punctuation and symbols become one space,
// and runs of one-character pieces are joined.
func Normalize(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'ぁ' && r <= 'ゖ':
			b.WriteRune(r + ('ァ' - 'ぁ'))
		case unicode.Is(unicode.Cf, r):
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsControl(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	parts := strings.Fields(b.String())
	out := parts[:0]
	single := false // the last part of out is a run of one-character pieces
	for _, p := range parts {
		one := utf8.RuneCountInString(p) == 1
		if one && single {
			out[len(out)-1] += p
			continue
		}
		out = append(out, p)
		single = one
	}
	return strings.Join(out, " ")
}

func isWordRune(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') }

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
