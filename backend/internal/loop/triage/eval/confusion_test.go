package eval

import (
	"slices"
	"testing"
)

func TestConfusionAdd(t *testing.T) {
	c := newConfusion([]string{"bug", "idea"})
	c.Add("bug", "bug")
	c.Add("bug", "bug") // the same pair twice
	c.Add("bug", "idea")
	if got := c.Count("bug", "bug"); got != 2 {
		t.Fatalf("bug/bug %d", got)
	}
	if got := c.Count("bug", "idea"); got != 1 {
		t.Fatalf("bug/idea %d", got)
	}
	if !slices.Equal(c.Rows, []string{"bug", "idea"}) || !slices.Equal(c.Cols, []string{"bug", "idea"}) {
		t.Fatalf("known labels moved: %v %v", c.Rows, c.Cols)
	}

	// a label seen for the first time is appended once, in first-seen order
	c.Add("question", "(none)")
	c.Add("question", "(held: x)")
	c.Add("question", "(none)")
	if !slices.Equal(c.Rows, []string{"bug", "idea", "question"}) {
		t.Fatalf("rows %v", c.Rows)
	}
	if !slices.Equal(c.Cols, []string{"bug", "idea", "(none)", "(held: x)"}) {
		t.Fatalf("cols %v", c.Cols)
	}
	if got := c.Count("question", "(none)"); got != 2 {
		t.Fatalf("question/none %d", got)
	}

	// an empty classification is its own label, not dropped
	c.Add("", "")
	if got := c.Count("", ""); got != 1 || !slices.Contains(c.Rows, "") || !slices.Contains(c.Cols, "") {
		t.Fatalf("empty pair: %d %v %v", got, c.Rows, c.Cols)
	}

	// a zero value works (nil map)
	var z Confusion
	z.Add("a", "b")
	if z.Count("a", "b") != 1 {
		t.Fatal("zero value Add")
	}
	if c.Count("idea", "bug") != 0 {
		t.Fatal("unseen pair is not 0")
	}
}
