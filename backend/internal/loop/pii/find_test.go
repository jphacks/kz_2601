package pii

import (
	"slices"
	"strings"
	"testing"
)

// Find is the exit check on LLM output (#366 検査器 2). Made-up values only.
func TestFind(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Kind
	}{
		{"clean", "写真を追加すると落ちる", []Kind{}},
		{"placeholders only", "[メール] と [電話] と [ID] が出た", []Kind{}},
		{"one kind reported once", "https://example.org と https://example.net", []Kind{URL}},
		{"kinds in rule order", "田中さん https://example.org", []Kind{URL, Name}},
		// the full-width form is normalised before matching
		{"full-width", "０９０ー１２３４ー５６７８", []Kind{Phone}},
		// zero-width characters do not hide a value (the digits also read
		// as a long number: see the overlap test below)
		{"zero-width split", "090\u200b-1234\u200d-5678", []Kind{Phone, Number}},
		// 28 letters without a digit is a word, not a uid
		{"uid shape kept by rule", strings.Repeat("a", 28), []Kind{}},
		{"uid shape", strings.Repeat("a1", 14), []Kind{ID}},
	}
	for _, c := range cases {
		if got := Find(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v want %v", c.name, got, c.want)
		}
	}
}

// Find runs every rule on the raw text (nothing is replaced in between), so
// one value may be reported under more than one kind. That errs towards
// holding the output; it must never report fewer.
func TestFindOverlapsErrTowardsReporting(t *testing.T) {
	got := Find("連絡 a@example.com / 090-1234-5678")
	for _, k := range []Kind{Email, Phone} {
		if !slices.Contains(got, k) {
			t.Fatalf("%v misses %s", got, k)
		}
	}
	seen := map[Kind]bool{}
	for _, k := range got {
		if seen[k] {
			t.Fatalf("%s reported twice: %v", k, got)
		}
		seen[k] = true
	}
}

func TestResultTotal(t *testing.T) {
	r := Mask("a@example.com b@example.com 090-1234-5678")
	if r.Total() != 3 || r.Counts[Email] != 2 || r.Counts[Phone] != 1 {
		t.Fatalf("%+v", r)
	}
	if (Result{}).Total() != 0 {
		t.Fatal("empty total")
	}
	if Mask("").Ratio != 0 {
		t.Fatal("empty ratio")
	}
}
