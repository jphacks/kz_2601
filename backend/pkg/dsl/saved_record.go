package dsl

// Saved records must be openable. A form screen (screen.entity) edits a record
// only when a record tap hands it the record id: list / mapView / gridTable /
// randomPicker / matchList onItemTap or a calendar.onDateTap row. A toolbar
// "new" button opens it empty, and table / timeline rows are not tappable, so
// an entity shown only in a table could be added but never opened, edited or
// deleted (schedule_poll #755, adaptive_study #761), and a prefillRef button
// on that form was never usable.

// recordTapTypes pass the tapped record's id to their tap action.
var recordTapTypes = map[ComponentType]bool{
	CompList: true, CompMapView: true, CompGridTable: true, CompRandomPicker: true, CompMatchList: true, CompCalendar: true,
}

// savedRecordReach warns about an entity with a form whose saved records no
// reachable record tap opens, when editing is clearly meant:
//
//   - its records are listed only by components without a record tap (table,
//     timeline, ...): a list without onItemTap is a deliberate append-only log
//     (bmi, attendance_nearby), so it stays quiet, or
//   - its form has a control that only works on a saved record (deleteRecord,
//     navigate.prefillRef).
//
// Skipped: entities without a form, setting entities (single included: the
// form edits the one record), and entities no reachable screen lists
// (entityReach already asks for a list).
func (l *linter) savedRecordReach() {
	forms := formScreens(l.app)
	opened, tappable := l.app.recordTaps()
	editOnly := map[string]bool{} // its form has a saved-record-only control
	for _, s := range l.app.Screens {
		if s.Entity != "" {
			acts := make([]*Action, 0, len(s.Toolbar))
			for i := range s.Toolbar {
				acts = append(acts, &s.Toolbar[i].Action)
			}
			eachScreenComponent(s.Components, func(_ string, c *Component) { acts = append(acts, c.Action) })
			for _, a := range acts {
				if a != nil && (a.Type == ActionDeleteRecord || (a.Type == ActionNavigate && a.PrefillRef != "")) {
					editOnly[s.Entity] = true
				}
			}
		}
	}
	skip := l.settingEntities()
	for _, id := range l.app.UnlistedEntities() {
		skip[id] = true
	}
	for _, e := range l.app.Entities {
		fs := forms[e.ID]
		if len(fs) == 0 || skip[e.ID] || opened[e.ID] || (tappable[e.ID] && !editOnly[e.ID]) {
			continue
		}
		l.add(`entity %q has a form screen %q, but no component lets the user open a saved %q record (table / timeline rows cannot be tapped, and a toolbar "new" button opens an empty form), so saved records can never be edited or deleted: show them with a list (or calendar / mapView / gridTable) whose onItemTap is {"type":"navigate","screen":%q} on a reachable screen`, e.ID, fs[0], e.ID, fs[0])
	}
}

// OpenedEntities lists the entities whose saved records a reachable record tap
// opens in their form: what savedRecordReach accepts (ai-eval masterTappable).
func (a *App) OpenedEntities() map[string]bool {
	opened, _ := a.recordTaps()
	return opened
}

// recordTaps scans the reachable screens. opened: a record tap navigates to
// the entity's form without prefillRef (prefillRef starts a new record).
// tappable: shown by a component that could tap a record at all.
func (a *App) recordTaps() (opened, tappable map[string]bool) {
	formOf := map[string]string{}
	for _, s := range a.Screens {
		if s.Entity != "" {
			formOf[s.ID] = s.Entity
		}
	}
	reach := a.Reachable()
	opened, tappable = map[string]bool{}, map[string]bool{}
	for _, s := range a.Screens {
		if !reach[s.ID] {
			continue
		}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if !recordTapTypes[c.Type] || c.Entity == "" {
				return
			}
			tappable[c.Entity] = true
			tap := c.OnItemTap
			if c.Type == CompCalendar {
				tap = c.OnDateTap
			}
			if tap != nil && tap.Type == ActionNavigate && tap.PrefillRef == "" && formOf[tap.Screen] == c.Entity {
				opened[c.Entity] = true
			}
		})
	}
	return opened, tappable
}
