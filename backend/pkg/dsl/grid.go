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

// LocalGridContainers are the containers whose children may carry `area`
// (#81). Their rectangle becomes a local 6-column grid: child areas count
// from A1 at the container's top-left, and rows are bounded by the
// container's own row count. Nesting stops there (one level only).
var LocalGridContainers = map[ComponentType]bool{CompCard: true, CompSection: true, CompVStack: true}

type placedArea struct {
	where string
	area  Area
}

// checkGrid validates the `area` layout of one screen's direct children
// (format/range, no overlap between siblings) and, per container, the local
// grid of its children (checkLocalGrid).
func checkGrid(screenID string, comps []Component, add func(string, ...any)) {
	var done []placedArea
	for i := range comps {
		c := &comps[i]
		where := fmt.Sprintf("screen %q components[%d]", screenID, i)
		if c.ID != "" {
			where += fmt.Sprintf(" (id %q)", c.ID)
		}
		var own *Area
		if c.Area != "" {
			a, err := ParseArea(c.Area)
			if err != nil {
				add("%s: %v", where, err)
			} else {
				own = &a
				done = addPlaced(done, placedArea{where, a}, "sibling components", add)
			}
		}
		checkLocalGrid(where, c, own, add)
	}
}

// addPlaced reports overlaps of p with the already placed rectangles and
// appends it.
func addPlaced(done []placedArea, p placedArea, scope string, add func(string, ...any)) []placedArea {
	for _, q := range done {
		if p.area.Overlaps(q.area) {
			add("%s area %s overlaps %s area %s; areas of %s must not share cells",
				p.where, p.area, q.where, q.area, scope)
		}
	}
	return append(done, p)
}

// checkLocalGrid validates `area` on a container's children (#81): only
// card / section / vstack, all children or none, rows within the
// container's own row count (own == nil: the container is stacked, so only
// the screen-wide row limit applies), no overlap, and no area one level
// deeper.
func checkLocalGrid(where string, c *Component, own *Area, add func(string, ...any)) {
	with := 0
	for j := range c.Children {
		if c.Children[j].Area != "" {
			with++
		}
		for k := range c.Children[j].Children {
			if c.Children[j].Children[k].Area != "" {
				add("%s.children[%d].children[%d]: area is allowed only one level inside a container; remove it",
					where, j, k)
			}
		}
	}
	if with == 0 {
		return
	}
	if !LocalGridContainers[c.Type] {
		add("%s: children of %s cannot have area (only card / section / vstack children can be laid out on a local grid); remove area from the children",
			where, c.Type)
		return
	}
	if with < len(c.Children) {
		add("%s: only %d of %d children have area; inside a container give every child an area (A1 = the container's top-left) or remove all of them to stack",
			where, with, len(c.Children))
		return
	}
	rows := 0
	if own != nil {
		rows = own.Row2 - own.Row1 + 1
	}
	var done []placedArea
	for j := range c.Children {
		ch := &c.Children[j]
		cw := fmt.Sprintf("%s.children[%d]", where, j)
		if ch.ID != "" {
			cw += fmt.Sprintf(" (id %q)", ch.ID)
		}
		a, err := ParseArea(ch.Area)
		if err != nil {
			add("%s: %v", cw, err)
			continue
		}
		if rows > 0 && a.Row2 > rows {
			add("%s: area %s goes past row %d; inside a container rows count from 1 to the container's own row count (its area %s has %d rows), so shrink the child or enlarge the container",
				cw, a, rows, own, rows)
		}
		done = addPlaced(done, placedArea{cw, a}, "children of one container", add)
	}
}
