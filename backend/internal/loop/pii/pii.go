// Package pii masks personal data in feedback text before it is stored for the
// self-improvement loop (#367). It is deterministic and never calls an LLM: the
// unmasked text must not leave the operator's project (#367 decision 6).
//
// Masking errs on the side of removing too much: a lost word costs a little
// triage accuracy, a leaked phone number cannot be taken back.
package pii

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Kind is one category of masked data. The value is the placeholder written
// in its place.
type Kind string

// Placeholders (#367 「何を」).
const (
	Email   Kind = "[メール]"
	Phone   Kind = "[電話]"
	URL     Kind = "[URL]"
	Secret  Kind = "[秘密]"
	Number  Kind = "[番号]"
	Address Kind = "[住所]"
	Place   Kind = "[位置]"
	ID      Kind = "[ID]"
	Name    Kind = "[名前]"
	Money   Kind = "[金額]"
)

// Kinds lists every placeholder (tests and the leak check use it).
var Kinds = []Kind{Email, Phone, URL, Secret, Number, Address, Place, ID, Name, Money}

type rule struct {
	kind Kind
	re   *regexp.Regexp
	// keep, when set, decides per match whether it is really personal data
	// (e.g. Luhn for card numbers). nil = always mask.
	keep func(string) bool
}

const prefectures = `北海道|東京都|京都府|大阪府|青森県|岩手県|宮城県|秋田県|山形県|福島県|茨城県|栃木県|群馬県|埼玉県|千葉県|神奈川県|新潟県|富山県|石川県|福井県|山梨県|長野県|岐阜県|静岡県|愛知県|三重県|滋賀県|兵庫県|奈良県|和歌山県|鳥取県|島根県|岡山県|広島県|山口県|徳島県|香川県|愛媛県|高知県|福岡県|佐賀県|長崎県|熊本県|大分県|宮崎県|鹿児島県|沖縄県`

// dash is a hyphen as people type it in Japanese text (NFKC keeps ー and ‐).
const dash = `[\-\s\x{2010}-\x{2015}\x{2212}\x{30FC}]`

// rules run in order on NFKC-normalised text; earlier rules win (a URL that
// contains digits is one [URL], not a URL with [番号] inside).
var rules = []rule{
	{URL, regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>"'）)]+|\bwww\.[^\s<>"'）)]+`), nil},
	{Email, regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`), nil},
	{Secret, regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=\-]+`), nil},
	{Secret, regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`), nil},
	{Secret, regexp.MustCompile(`\b(?:sk|pk|rk|ghp|gho|ghs|ghu|github_pat|xox[abposr]|AKIA|AIza)[_\-]?[A-Za-z0-9_\-]{12,}`), nil},
	{Secret, regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`), nil},
	{Secret, regexp.MustCompile(`[A-Za-z0-9+/_\-]{40,}={0,2}`), nil},
	{ID, regexp.MustCompile(`\b(?:fb|app|rec|lin|ldr)_[A-Za-z0-9]{8,}\b`), nil},
	// Firebase Auth uids are 28 alphanumerics with both letters and digits.
	{ID, regexp.MustCompile(`\b[A-Za-z0-9]{28}\b`), func(s string) bool {
		return strings.ContainsAny(s, "0123456789") && strings.IndexFunc(s, unicode.IsLetter) >= 0
	}},
	{Place, regexp.MustCompile(`-?\d{1,3}\.\d{3,}\s*,\s*-?\d{1,3}\.\d{3,}`), nil},
	{Place, regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`), nil},
	{Address, regexp.MustCompile(`(?:` + prefectures + `)[^\s、。,]{0,24}?\d+(?:[\-丁目番地号の]+\d+)*(?:号)?`), nil},
	{Phone, regexp.MustCompile(`\+\d{1,3}[\s\-]?\(?\d{1,4}\)?(?:[\s\-]?\d{2,4}){2,3}`), nil},
	{Phone, regexp.MustCompile(`\b0\d{1,4}` + dash + `\d{1,4}` + dash + `\d{3,4}\b|\b0[5789]0\d{8}\b|\b0\d{9}\b`), nil},
	{Number, regexp.MustCompile(`〒\s?\d{3}-?\d{4}|\b\d{3}-\d{4}\b`), nil},
	{Money, regexp.MustCompile(`[¥$€£]\s?\d[\d,]*(?:\.\d+)?|\d[\d,]*(?:\.\d+)?\s?(?:円|万円|ドル|ユーロ)`), nil},
	// Card numbers (Luhn), My Number (12 digits), account numbers and any
	// other long run of digits.
	{Number, regexp.MustCompile(`\b\d(?:[\s\-]?\d){6,22}\b`), nil},
	{Name, regexp.MustCompile(`@[A-Za-z0-9_.\-]{2,}`), nil},
	{Name, regexp.MustCompile(`(?i)\b(?:mr|mrs|ms|miss|dr)\.?\s+[A-Z][a-z]+(?:\s+[A-Z][a-z]+)?`), nil},
	// 「田中さん」「タナカ様」: the run of kanji/katakana before an honorific.
	{Name, regexp.MustCompile(`[\p{Han}\p{Katakana}ー]{1,6}(?:さん|様|さま|くん|君|ちゃん|氏|先生)`), nil},
}

// Result is the outcome of Mask.
type Result struct {
	// Text is the masked text (NFKC-normalised, control characters and
	// invisible characters removed).
	Text string
	// Counts is the number of replacements per kind.
	Counts map[Kind]int
	// Ratio is the share of the input's characters that were masked (0..1).
	// The loop holds back text masked beyond its threshold (#367: 3 割).
	Ratio float64
}

// Total is the number of replacements.
func (r Result) Total() int {
	n := 0
	for _, c := range r.Counts {
		n += c
	}
	return n
}

// Mask replaces personal data in s with placeholders.
func Mask(s string) Result {
	s = Clean(s)
	total := utf8.RuneCountInString(s)
	res := Result{Counts: map[Kind]int{}}
	masked := 0
	for _, r := range rules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			if isPlaceholder(m) || (r.keep != nil && !r.keep(m)) {
				return m
			}
			res.Counts[r.kind]++
			masked += utf8.RuneCountInString(m)
			return string(r.kind)
		})
	}
	res.Text = s
	if total > 0 {
		res.Ratio = min(1, float64(masked)/float64(total))
	}
	return res
}

// Find reports the kinds of personal data still present in s (the exit check
// on LLM output, #366 検査器 2). Placeholders themselves are not reported.
func Find(s string) []Kind {
	s = Clean(s)
	seen := map[Kind]bool{}
	out := []Kind{}
	for _, r := range rules {
		for _, m := range r.re.FindAllString(s, -1) {
			if isPlaceholder(m) || (r.keep != nil && !r.keep(m)) || seen[r.kind] {
				continue
			}
			seen[r.kind] = true
			out = append(out, r.kind)
		}
	}
	return out
}

// Clean NFKC-normalises s (full-width digits and letters become ASCII, so the
// rules see one form) and drops control characters other than newline and
// tab, and invisible formatting characters (zero-width, bidi overrides).
func Clean(s string) string {
	s = norm.NFKC.String(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
			return -1
		}
		return r
	}, s)
}

// HasInvisible reports whether s holds control or invisible formatting
// characters (other than newline and tab) before cleaning.
func HasInvisible(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return true
		}
	}
	return false
}

func isPlaceholder(m string) bool {
	for _, k := range Kinds {
		if m == string(k) {
			return true
		}
	}
	return false
}
