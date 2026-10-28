package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Marks (#255): a per-user toggle on shared records (like / save / read).
// Who marked what lives in appconfig (records/{id}/marks/{markId}/by/{uid}),
// not in the record's data, so one person counts once and nobody can type a
// mark for someone else. The DSL only names them and shows them.

// Mark is one entry of Entity.marks ($defs/Mark).
type Mark struct {
	ID    string `json:"id"`
	Icon  string `json:"icon"`
	Label string `json:"label"`
}

// MarkIcons is $defs/Mark.icon's enum (iOS maps each to an SF Symbol).
var MarkIcons = []string{"heart", "star", "bookmark", "thumbsup", "check", "eye", "flag"}

// MaxMarksPerEntity mirrors $defs/Entity.marks maxItems.
const MaxMarksPerEntity = 3

// RecordMarkPrefix starts the record value "$marks.<id>": the mark's count
// for display / sort, and "marked by me" as a filter field with {"me": true}.
const RecordMarkPrefix = "$marks."

// MarkByID returns the entity's mark or nil.
func (e *Entity) MarkByID(id string) *Mark {
	for i := range e.Marks {
		if e.Marks[i].ID == id {
			return &e.Marks[i]
		}
	}
	return nil
}

// MarkIDs lists the entity's mark ids (appconfig attaches their state to records).
func (e *Entity) MarkIDs() []string {
	out := make([]string, 0, len(e.Marks))
	for _, m := range e.Marks {
		out = append(out, m.ID)
	}
	return out
}

// markOf splits "$marks.<id>" (ok=false for anything else).
func markOf(id string) (string, bool) {
	return strings.CutPrefix(id, RecordMarkPrefix)
}

// markValueErr reports why "$marks.<id>" cannot be used on e ("" = it can).
func markValueErr(e *Entity, key, id string) string {
	mid, _ := markOf(id)
	switch {
	case !e.IsShared():
		return fmt.Sprintf(`%s %q: marks are only for shared/public entities (entity %q is private)`, key, id, e.ID)
	case e.MarkByID(mid) == nil:
		return fmt.Sprintf(`%s %q: entity %q has no mark %q (marks: %s); add {"id":%q,"icon":"heart","label":"..."} to the entity's marks`, key, id, e.ID, mid, markIDList(e), mid)
	}
	return ""
}

func markIDList(e *Entity) string {
	if len(e.Marks) == 0 {
		return "none"
	}
	return strings.Join(e.MarkIDs(), ", ")
}

// checkEntityMarks validates Entity.marks.
func checkEntityMarks(e *Entity, add func(string, ...any)) {
	if len(e.Marks) == 0 {
		return
	}
	if !e.IsShared() {
		add(`entity %q marks are only for shared/public entities (a private entity is the user's own; use a boolean field with list item.toggle instead)`, e.ID)
	}
	if len(e.Marks) > MaxMarksPerEntity {
		add("entity %q has too many marks (%d > %d)", e.ID, len(e.Marks), MaxMarksPerEntity)
	}
	seen := map[string]bool{}
	for i, m := range e.Marks {
		where := fmt.Sprintf("entity %q marks[%d]", e.ID, i)
		if !idRe.MatchString(m.ID) {
			add("%s.id %q is not a valid id", where, m.ID)
		}
		if seen[m.ID] {
			add("%s: duplicate mark id %q", where, m.ID)
		}
		seen[m.ID] = true
		if !slices.Contains(MarkIcons, m.Icon) {
			add("%s.icon %q must be one of %s", where, m.Icon, strings.Join(MarkIcons, "|"))
		}
		if strings.TrimSpace(m.Label) == "" {
			add("%s.label is empty", where)
		}
	}
}

// checkListMark validates list.item.mark against the list entity.
func checkListMark(where string, item *ListItem, e *Entity, add func(string, ...any)) {
	if item == nil || item.Mark == "" {
		return
	}
	if msg := markValueErr(e, "list item.mark", RecordMarkPrefix+item.Mark); msg != "" {
		add("%s: %s", where, msg)
	}
}
