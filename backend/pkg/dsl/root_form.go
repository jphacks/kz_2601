package dsl

import (
	"fmt"
	"slices"
)

// Root input screen (#1719): a form (screen.entity) shown as a tab or the
// home screen whose saveRecord has no "screen". iOS closes the form after a
// save, but a tab / home root has nothing to close, so the filled form stays
// put and the saved record is never seen in a list. canLeave (fullScreen
// targets) is unaffected: a fullScreen form is a modal and does close.
//
// single entities (settings) are skipped: editing the one record in place is
// the intended use.
func (l *linter) rootFormSave() {
	roots := l.app.NavRoots()
	for i := range l.app.Screens {
		s := &l.app.Screens[i]
		if s.Entity == "" || !slices.Contains(roots, s.ID) {
			continue
		}
		if e := l.entities[s.Entity]; e == nil || e.Single {
			continue
		}
		via := saveWithoutScreen(s)
		if via == "" {
			continue
		}
		list := l.listScreenOf(s.Entity, s.ID)
		if list == "" {
			l.add(`screen %q is a form (entity %q) shown as a tab/home screen, and its saveRecord (%s) has no "screen", so saving leaves the filled form in place: add a list screen of %q as the tab and open this form from it ({"type":"navigate","screen":%q} add button), or set the saveRecord "screen" to that list screen`, s.ID, s.Entity, via, s.Entity, s.ID)
			continue
		}
		l.add(`screen %q is a form (entity %q) shown as a tab/home screen, and its saveRecord (%s) has no "screen", so saving leaves the filled form in place: open this form from list screen %q ({"type":"navigate","screen":%q} add button) instead of making it a tab, or set the saveRecord "screen" to %q`, s.ID, s.Entity, via, list, s.ID, list)
	}
}

// saveWithoutScreen returns where on s the first saveRecord without a
// then-screen is ("toolbar[0]", "components[2].action"), or "".
func saveWithoutScreen(s *Screen) string {
	bare := func(act *Action) bool { return act != nil && act.Type == ActionSaveRecord && act.Screen == "" }
	for i := range s.Toolbar {
		if bare(&s.Toolbar[i].Action) {
			return fmt.Sprintf("toolbar[%d]", i)
		}
	}
	via := ""
	eachScreenComponent(s.Components, func(where string, c *Component) {
		if via == "" && bare(c.Action) {
			via = where + ".action"
		}
	})
	return via
}

// listScreenOf returns the first screen other than skip with a list of entity.
func (l *linter) listScreenOf(entity, skip string) string {
	for i := range l.app.Screens {
		s := &l.app.Screens[i]
		if s.ID == skip {
			continue
		}
		found := false
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			found = found || (c.Type == CompList && c.Entity == entity)
		})
		if found {
			return s.ID
		}
	}
	return ""
}
