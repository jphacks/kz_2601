package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Screen graph (#115): which screens can be reached from the roots (the
// initial screen and every tab) and how. Validate rejects edges to missing
// screens; Lint warns about screens nobody can reach and fullScreen screens
// with no way back.

// NavEdge is one way to move from a screen to another.
type NavEdge struct {
	From         string
	To           string
	Presentation string // push|sheet|fullScreen ("" = default)
	Via          string // where on From: "toolbar[0]", "components[2].children[1].onItemTap", ...
}

// NavEdges lists every navigate (and saveRecord then-screen) edge in screen order.
func (a *App) NavEdges() []NavEdge {
	var out []NavEdge
	for _, s := range a.Screens {
		edge := func(via string, act *Action) {
			if act == nil {
				return
			}
			switch {
			case act.Type == ActionNavigate:
			case act.Type == ActionSaveRecord && act.Screen != "":
			default:
				return
			}
			out = append(out, NavEdge{From: s.ID, To: act.Screen, Presentation: act.Presentation, Via: via})
		}
		for i := range s.Toolbar {
			edge(fmt.Sprintf("toolbar[%d]", i), &s.Toolbar[i].Action)
		}
		eachScreenComponent(s.Components, func(where string, c *Component) {
			edge(where+".action", c.Action)
			edge(where+".onItemTap", c.OnItemTap)
			edge(where+".onDateTap", c.OnDateTap)
		})
	}
	return out
}

// HasNav reports whether screen from has a button / toolbar / item tap that
// opens screen to.
func (a *App) HasNav(from, to string) bool {
	for _, e := range a.NavEdges() {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

// NavRoots are the screens the app can show without navigating: the tab
// roots, or initialScreen when there are no tabs. With tabs iOS opens
// initialScreen only if it is a tab root (else the first tab).
func (a *App) NavRoots() []string {
	if len(a.App.Tabs) == 0 {
		return []string{a.InitialScreen}
	}
	var roots []string
	for _, t := range a.App.Tabs {
		if !slices.Contains(roots, t.Screen) {
			roots = append(roots, t.Screen)
		}
	}
	return roots
}

// Reachable returns the screens reachable from NavRoots.
func (a *App) Reachable() map[string]bool {
	next := map[string][]string{}
	for _, e := range a.NavEdges() {
		next[e.From] = append(next[e.From], e.To)
	}
	exists := map[string]bool{}
	for _, s := range a.Screens {
		exists[s.ID] = true
	}
	seen := map[string]bool{}
	queue := a.NavRoots()
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] || !exists[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, next[id]...)
	}
	return seen
}

// Unreachable lists the screens (in document order) that Reachable misses.
func (a *App) Unreachable() []string {
	r := a.Reachable()
	var out []string
	for _, s := range a.Screens {
		if !r[s.ID] {
			out = append(out, s.ID)
		}
	}
	return out
}

// screenIDs is the "use one of" hint of the graph errors.
func screenIDs(a *App) string {
	ids := make([]string, 0, len(a.Screens))
	for _, s := range a.Screens {
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, ", ")
}

// canLeave reports whether a screen shown fullScreen has a control that
// closes it: back, or saveRecord without a then-screen (iOS closes the form).
func canLeave(s *Screen) bool {
	leaves := func(act *Action) bool {
		return act != nil && (act.Type == ActionBack || (act.Type == ActionSaveRecord && act.Screen == ""))
	}
	for i := range s.Toolbar {
		if leaves(&s.Toolbar[i].Action) {
			return true
		}
	}
	found := false
	eachScreenComponent(s.Components, func(_ string, c *Component) {
		found = found || leaves(c.Action)
	})
	return found
}

func eachScreenComponent(comps []Component, fn func(where string, c *Component)) {
	for i := range comps {
		where := fmt.Sprintf("components[%d]", i)
		fn(where, &comps[i])
		for j := range comps[i].Children {
			fn(fmt.Sprintf("%s.children[%d]", where, j), &comps[i].Children[j])
		}
	}
}

// navGraph adds the screen-graph warnings to the linter.
//
//   - initialScreen that is not a tab root when there are tabs
//   - a screen no root can reach (forms whose entity has no reachable form
//     are already reported by entityReach)
//   - a fullScreen target without back / saveRecord (iOS shows no close
//     button on a full-screen cover, so the user is stuck)
func (l *linter) navGraph() {
	reach := l.app.Reachable()
	formReached := map[string]bool{}
	for _, s := range l.app.Screens {
		if s.Entity != "" && reach[s.ID] {
			formReached[s.Entity] = true
		}
	}
	if len(l.app.App.Tabs) > 0 && !slices.Contains(l.app.NavRoots(), l.app.InitialScreen) {
		l.add(`initialScreen %q is not one of app.tabs, so iOS opens the first tab instead: set initialScreen to a tab screen (usually app.tabs[0].screen)`, l.app.InitialScreen)
	}
	for _, s := range l.app.Screens {
		if reach[s.ID] || (s.Entity != "" && !formReached[s.Entity]) || s.ID == l.app.InitialScreen {
			continue
		}
		l.add(`screen %q is not reachable from the initial screen or any tab: add a button/toolbar {"type":"navigate","screen":%q} on a reachable screen (forms: an add button or list.onItemTap), make it a tab in app.tabs, or remove the screen`, s.ID, s.ID)
	}
	byID := map[string]*Screen{}
	for i := range l.app.Screens {
		byID[l.app.Screens[i].ID] = &l.app.Screens[i]
	}
	warned := map[string]bool{}
	for _, e := range l.app.NavEdges() {
		s := byID[e.To]
		if e.Presentation != "fullScreen" || s == nil || warned[e.To] || canLeave(s) {
			continue
		}
		warned[e.To] = true
		l.add(`screen %q is opened fullScreen (from screen %q %s) but has no way back: add a toolbar button {"label":"閉じる","action":{"type":"back"}} (label in the display language) to it, or open it with presentation sheet or push`, e.To, e.From, e.Via)
	}
}
