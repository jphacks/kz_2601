package dsl

import (
	"fmt"
	"regexp"
	"strconv"
)

// Screen grid: 6 columns (A-F) x rows of visibleHeight/12. Rows beyond
// GridVisibleRows make the screen scroll; MaxGridRows bounds AI output.
const (
	GridColumns     = 6
	GridVisibleRows = 12
	MaxGridRows     = 48
)

var areaRe = regexp.MustCompile(`^([A-F])([1-9][0-9]*):([A-F])([1-9][0-9]*)$`)

// Area is a parsed `area` rectangle. Columns are 0-based (A=0), rows 1-based,
// both ends inclusive.
type Area struct {
	Col1, Row1, Col2, Row2 int
}

// ParseArea parses "A1:C3". It rejects malformed strings, columns past F,
// row 0, inverted rectangles and rows beyond MaxGridRows.
func ParseArea(s string) (Area, error) {
	m := areaRe.FindStringSubmatch(s)
	if m == nil {
		return Area{}, fmt.Errorf("area %q must look like A1:C3 (columns A-F, rows 1-%d)", s, MaxGridRows)
	}
	a := Area{Col1: int(m[1][0] - 'A'), Col2: int(m[3][0] - 'A')}
	a.Row1, _ = strconv.Atoi(m[2])
	a.Row2, _ = strconv.Atoi(m[4])
	if a.Col2 < a.Col1 || a.Row2 < a.Row1 {
		return Area{}, fmt.Errorf("area %q: bottom-right cell must be at or after top-left", s)
	}
	if a.Row2 > MaxGridRows {
		return Area{}, fmt.Errorf("area %q: rows beyond %d are not allowed", s, MaxGridRows)
	}
	return a, nil
}

// String renders back to spreadsheet notation.
func (a Area) String() string {
	return fmt.Sprintf("%c%d:%c%d", 'A'+a.Col1, a.Row1, 'A'+a.Col2, a.Row2)
}

// Overlaps reports whether two rectangles share at least one cell.
func (a Area) Overlaps(b Area) bool {
	return a.Col1 <= b.Col2 && b.Col1 <= a.Col2 && a.Row1 <= b.Row2 && b.Row1 <= a.Row2
}

// checkGrid validates the `area` layout of one screen's direct children:
// format/range, no overlap between siblings, and no `area` inside containers
// (v1: the grid exists only one level deep; container children stack).
func checkGrid(screenID string, comps []Component, add func(string, ...any)) {
	type placed struct {
		where string
		area  Area
	}
	var done []placed
	for i := range comps {
		c := &comps[i]
		where := fmt.Sprintf("screen %q components[%d]", screenID, i)
		if c.ID != "" {
			where += fmt.Sprintf(" (id %q)", c.ID)
		}
		for j := range c.Children {
			if c.Children[j].Area != "" {
				add("%s.children[%d]: area %q is ignored inside a container; only direct children of a screen are placed on the grid (v1)",
					where, j, c.Children[j].Area)
			}
		}
		if c.Area == "" {
			continue
		}
		a, err := ParseArea(c.Area)
		if err != nil {
			add("%s: %v", where, err)
			continue
		}
		for _, p := range done {
			if a.Overlaps(p.area) {
				add("%s area %s overlaps %s area %s; areas of sibling components must not share cells",
					where, a, p.where, p.area)
			}
		}
		done = append(done, placed{where, a})
	}
}
