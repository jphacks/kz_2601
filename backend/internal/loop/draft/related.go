package draft

import (
	"sort"
	"strings"
	"unicode"
)

// Related issues (#2288): open issues whose title is close to a cluster's
// summary, for a person to look at in `drafts show`. Pure: no I/O, no
// embeddings (the embedding path #2148 is separate and off by default).

// IssueTitle is an open issue as the matcher sees it: the title only.
type IssueTitle struct {
	Number int
	Title  string
}

// RelatedIssue is a match with its closeness (0..1).
type RelatedIssue struct {
	Number int
	Title  string
	Score  float64
}

const (
	// MaxRelated is how many related issues a draft lists.
	MaxRelated = 3
	// MinRelatedScore: below this a title is not shown.
	MinRelatedScore = 0.25
)

// FindRelated scores every title against summary and returns the closest
// ones (at most MaxRelated, at least MinRelatedScore), closest first; a tie
// goes to the newer (higher) number.
func FindRelated(summary string, issues []IssueTitle) []RelatedIssue {
	want := terms(summary)
	if len(want) == 0 {
		return nil
	}
	var out []RelatedIssue
	for _, iss := range issues {
		if iss.Number <= 0 {
			continue
		}
		if s := dice(want, terms(iss.Title)); s >= MinRelatedScore {
			out = append(out, RelatedIssue{Number: iss.Number, Title: iss.Title, Score: s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Number > out[j].Number
	})
	if len(out) > MaxRelated {
		out = out[:MaxRelated]
	}
	return out
}

// dice is 2|A∩B| / (|A|+|B|) over the term sets.
func dice(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for t := range a {
		if b[t] {
			n++
		}
	}
	return 2 * float64(n) / float64(len(a)+len(b))
}

// terms splits s into words (Latin letters and digits, lower case) and
// character 2-grams of Japanese runs (a run of one character is kept as is).
func terms(s string) map[string]bool {
	out := map[string]bool{}
	var word, ja []rune
	flush := func() {
		if len(word) > 0 {
			out[string(word)] = true
			word = word[:0]
		}
		switch {
		case len(ja) == 1:
			out[string(ja)] = true
		case len(ja) > 1:
			for i := 0; i+1 < len(ja); i++ {
				out[string(ja[i:i+2])] = true
			}
		}
		ja = ja[:0]
	}
	for _, r := range strings.ToLower(s) {
		if r >= 0xFF01 && r <= 0xFF5E { // full-width ASCII
			r = unicode.ToLower(r - 0xFEE0)
		}
		switch {
		case isJapanese(r):
			if len(word) > 0 {
				flush()
			}
			ja = append(ja, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if len(ja) > 0 {
				flush()
			}
			word = append(word, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func isJapanese(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) || r == 'ー'
}
