package eval

import (
	"fmt"
	"slices"
	"strings"
)

// Bar is #376's completion bar: 人の判断と 8 割以上一致.
const Bar = 0.8

// Confusion counts (want, got) pairs: rows are the person's labels, columns
// the model's answers. The known labels always appear, in their order;
// anything else (held, none) is appended in the order first seen.
type Confusion struct {
	Rows, Cols []string
	counts     map[[2]string]int
}

func newConfusion(labels []string) Confusion {
	return Confusion{Rows: slices.Clone(labels), Cols: slices.Clone(labels), counts: map[[2]string]int{}}
}

// Add counts one pair.
func (c *Confusion) Add(want, got string) {
	if c.counts == nil {
		c.counts = map[[2]string]int{}
	}
	if !slices.Contains(c.Rows, want) {
		c.Rows = append(c.Rows, want)
	}
	if !slices.Contains(c.Cols, got) {
		c.Cols = append(c.Cols, got)
	}
	c.counts[[2]string{want, got}]++
}

// Count is the number of pairs (want, got).
func (c Confusion) Count(want, got string) int { return c.counts[[2]string{want, got}] }

func (c Confusion) write(b *strings.Builder) {
	w := 0
	for _, r := range c.Rows {
		w = max(w, len(r))
	}
	fmt.Fprintf(b, "    %-*s", w, "")
	for _, col := range c.Cols {
		fmt.Fprintf(b, "  %s", col)
	}
	b.WriteString("\n")
	for _, r := range c.Rows {
		fmt.Fprintf(b, "    %-*s", w, r)
		for _, col := range c.Cols {
			fmt.Fprintf(b, "  %*d", len(col), c.Count(r, col))
		}
		b.WriteString("\n")
	}
}

// bundleTable is the pairs as a 2x2 table: labelled the same problem or
// not, against bundled together by the model or not.
func bundleTable(together, apart Score) Confusion {
	c := newConfusion([]string{"same", "apart"})
	c.counts[[2]string{"same", "same"}] = together.Matched
	c.counts[[2]string{"same", "apart"}] = together.Total - together.Matched
	c.counts[[2]string{"apart", "apart"}] = apart.Matched
	c.counts[[2]string{"apart", "same"}] = apart.Total - apart.Matched
	return c
}

// Below lists the fields under bar. A field with nothing to compare (no
// pair labelled as one bundle, say) is not counted as below.
func (r Report) Below(bar float64) []string {
	var out []string
	for _, f := range []struct {
		name string
		s    Score
	}{{"category", r.Category}, {"priority", r.Priority}, {"bundle-together", r.Together}, {"bundle-apart", r.Apart}} {
		if f.s.Total > 0 && f.s.Rate() < bar {
			out = append(out, f.name)
		}
	}
	return out
}

func (r Report) verdict(bar float64) string {
	below := r.Below(bar)
	if len(below) == 0 {
		return fmt.Sprintf("  verdict:  every field at or above %.0f%%\n", 100*bar)
	}
	return fmt.Sprintf("  verdict:  below %.0f%%: %s\n", 100*bar, strings.Join(below, ", "))
}
