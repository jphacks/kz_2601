package ai

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// sharedEditNotes lists the shared/public entities whose update rule the
// spoken summary must state (#249 / #252): members = edit:members (anyone may
// update, only the author deletes); authorOnly = author-only entities that
// still carry a check (boolean field or list.item.toggle), which other people
// would otherwise try to tick and be refused.
func sharedEditNotes(a *dsl.App) (members, authorOnly []*dsl.Entity) {
	toggled := map[string]bool{}
	for i := range a.Screens {
		eachComponent(a.Screens[i].Components, func(c *dsl.Component) {
			if c.Type == dsl.CompList && c.Item != nil && c.Item.Toggle != "" {
				toggled[c.Entity] = true
			}
		})
	}
	for i := range a.Entities {
		e := &a.Entities[i]
		if !e.IsShared() {
			continue
		}
		if e.MembersCanEdit() {
			members = append(members, e)
			continue
		}
		if toggled[e.ID] || hasBooleanField(e) {
			authorOnly = append(authorOnly, e)
		}
	}
	return members, authorOnly
}

func hasBooleanField(e *dsl.Entity) bool {
	for _, f := range e.Fields {
		if f.Type == dsl.FieldBoolean && f.Computed == nil {
			return true
		}
	}
	return false
}

func entityNames(es []*dsl.Entity, lower bool) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		n := e.Name
		if lower {
			n = strings.ToLower(n)
		}
		out = append(out, n)
	}
	return out
}

// editNoteJa: "家事は参加者の誰でも更新でき、削除は作った人だけができます。"
func editNoteJa(a *dsl.App) string {
	members, authorOnly := sharedEditNotes(a)
	var b strings.Builder
	if len(members) > 0 {
		fmt.Fprintf(&b, "%sは参加者の誰でも更新でき、削除は作った人だけができます。", joinJa(entityNames(members, false)))
	}
	if len(authorOnly) > 0 {
		fmt.Fprintf(&b, "%sは作った人だけが変更できます。", joinJa(entityNames(authorOnly, false)))
	}
	return b.String()
}

// editNoteEN: " Anyone in the app can update chores; only the creator can delete them."
func editNoteEN(a *dsl.App) string {
	members, authorOnly := sharedEditNotes(a)
	var b strings.Builder
	if len(members) > 0 {
		fmt.Fprintf(&b, " Anyone in the app can update %s; only the creator can delete them.", joinEN(pluralAll(entityNames(members, true)), "and"))
	}
	if len(authorOnly) > 0 {
		fmt.Fprintf(&b, " Only the creator can change %s.", joinEN(pluralAll(entityNames(authorOnly, true)), "and"))
	}
	return b.String()
}

func pluralAll(names []string) []string {
	for i, n := range names {
		names[i] = pluralEN(n)
	}
	return names
}
