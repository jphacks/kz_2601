package ai

import (
	"encoding/json"
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fixButtonOverlap runs before validation. The model often lays a list
// section down to the last row and the add button on top of its bottom rows
// (#169: warikan "recent" A9:F12 vs "add_expense" D11:F12, 3 of 50 creates),
// which fails the sibling-overlap check and costs a whole regeneration (~8 s).
// A container / list / table that starts above a button and ends inside the
// button's rows is shrunk to end just above it (kept only if 2+ rows remain).
// Returns raw unchanged when nothing applies.
func fixButtonOverlap(raw []byte) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	screens, _ := doc["screens"].([]any)
	changed := false
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		comps, _ := sm["components"].([]any)
		for _, bc := range comps {
			bm, _ := bc.(map[string]any)
			b, ok := areaOf(bm)
			if !ok || bm["type"] != "button" {
				continue
			}
			for _, ac := range comps {
				am, _ := ac.(map[string]any)
				a, ok := areaOf(am)
				if !ok || !shrinkable[fmt.Sprint(am["type"])] || !a.Overlaps(b) ||
					a.Row1 >= b.Row1 || a.Row2 > b.Row2 || b.Row1-a.Row1 < 2 {
					continue
				}
				a.Row2 = b.Row1 - 1
				am["area"] = a.String()
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

var shrinkable = map[string]bool{"section": true, "vstack": true, "card": true, "list": true, "table": true}

func areaOf(c map[string]any) (dsl.Area, bool) {
	s, _ := c["area"].(string)
	a, err := dsl.ParseArea(s)
	return a, err == nil
}

// fixLayoutRules deterministically applies two visual rules (#140) the model
// still breaks now and then (#169 audit 3), without an extra API call:
//
//   - mic seat: a button anchored at the bottom-left (A-B of the last 2 rows)
//     of a non-form screen moves to the right edge of the same rows when those
//     cells are free; otherwise it becomes a toolbar button (dropped when the
//     toolbar already has the same action). Same seat as dsl lint micSeat.
//   - list date title: a list whose item.title is a date / datetime field
//     swaps it with a string / text / enum subtitle or trailing field.
//
// Returns raw unchanged when nothing applies or the result does not validate.
func fixLayoutRules(raw []byte, app *dsl.App) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	screens, _ := doc["screens"].([]any)
	changed := false
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		if sm == nil {
			continue
		}
		if e, _ := sm["entity"].(string); e == "" && clearMicSeat(sm) {
			changed = true
		}
		comps, _ := sm["components"].([]any)
		if fixDateTitles(comps, app) {
			changed = true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	if _, err := dsl.Parse(out); err != nil {
		return raw
	}
	return out
}

// clearMicSeat moves buttons off the mic seat of one non-form screen.
func clearMicSeat(sm map[string]any) bool {
	const seatCols, seatRows, anchorRows = 2, 2, 3
	comps, _ := sm["components"].([]any)
	areas := make([]*dsl.Area, len(comps))
	last := dsl.GridVisibleRows
	for i, c := range comps {
		cm, _ := c.(map[string]any)
		s, _ := cm["area"].(string)
		if a, err := dsl.ParseArea(s); err == nil {
			areas[i] = &a
			last = max(last, a.Row2)
		}
	}
	changed := false
	var keep []any
	for i, c := range comps {
		cm, _ := c.(map[string]any)
		a := areas[i]
		onSeat := a != nil && a.Col1 < seatCols && a.Row2 > last-seatRows && a.Row1 >= last-anchorRows
		if !onSeat || cm["type"] != "button" {
			keep = append(keep, c)
			continue
		}
		changed = true
		width := min(a.Col2-a.Col1+1, 3)
		moved := dsl.Area{Col1: 6 - width, Row1: a.Row1, Col2: 5, Row2: a.Row2}
		if moved.Col1 >= seatCols && free(areas, i, moved) {
			cm["area"] = moved.String()
			areas[i] = &moved
			keep = append(keep, c)
			continue
		}
		areas[i] = nil
		toToolbar(sm, cm)
	}
	if changed {
		sm["components"] = keep
	}
	return changed
}

func free(areas []*dsl.Area, self int, r dsl.Area) bool {
	for j, b := range areas {
		if j != self && b != nil && b.Overlaps(r) {
			return false
		}
	}
	return true
}

// toToolbar appends the button to the screen toolbar unless one with the
// same action is already there.
func toToolbar(sm, btn map[string]any) {
	tb, _ := sm["toolbar"].([]any)
	act, _ := json.Marshal(btn["action"])
	for _, t := range tb {
		tm, _ := t.(map[string]any)
		if a, _ := json.Marshal(tm["action"]); string(a) == string(act) {
			return
		}
	}
	item := map[string]any{"label": btn["label"], "action": btn["action"]}
	if icon, ok := btn["icon"]; ok {
		item["icon"] = icon
	}
	sm["toolbar"] = append(tb, item)
}

// fixDateTitles walks screen components (and container children).
func fixDateTitles(comps []any, app *dsl.App) bool {
	changed := false
	for _, c := range comps {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		if ch, _ := cm["children"].([]any); len(ch) > 0 && fixDateTitles(ch, app) {
			changed = true
		}
		if cm["type"] != "list" {
			continue
		}
		item, _ := cm["item"].(map[string]any)
		ent, _ := cm["entity"].(string)
		e := app.EntityByID(ent)
		if item == nil || e == nil {
			continue
		}
		title, _ := item["title"].(string)
		if f := e.FieldByID(title); f == nil || (f.Type != dsl.FieldDate && f.Type != dsl.FieldDatetime) {
			continue
		}
		for _, slot := range []string{"subtitle", "trailing"} {
			id, _ := item[slot].(string)
			if f := e.FieldByID(id); f != nil && (f.Type == dsl.FieldString || f.Type == dsl.FieldText || f.Type == dsl.FieldEnum) {
				item["title"], item[slot] = id, title
				changed = true
				break
			}
		}
	}
	return changed
}
