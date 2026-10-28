package dsl

// One add control per screen (#416). A toolbar navigate that opens the same
// form (screen.entity) as a button on the same screen is a second "add" for
// one action: the kakeibo few-shot had both the top-right ＋ and the
// bottom-right 「支出を追加」. The button stays (bigger, in thumb reach); the
// toolbar copy goes. iOS (DuplicateAddControl) hides it the same way, so
// stored documents do not show both either.

// DuplicateAddToolbar returns the indexes of s.Toolbar that navigate to a
// form screen that a button on s also opens with the same prefillRef.
func (a *App) DuplicateAddToolbar(s *Screen) []int {
	type opener struct{ screen, prefillRef string }
	buttons := map[opener]bool{}
	eachScreenComponent(s.Components, func(_ string, c *Component) {
		if c.Type == CompButton && c.Action != nil && c.Action.Type == ActionNavigate {
			buttons[opener{c.Action.Screen, c.Action.PrefillRef}] = true
		}
	})
	if len(buttons) == 0 {
		return nil
	}
	forms := map[string]bool{}
	for _, sc := range a.Screens {
		if sc.Entity != "" {
			forms[sc.ID] = true
		}
	}
	var out []int
	for i, tb := range s.Toolbar {
		act := tb.Action
		if act.Type == ActionNavigate && forms[act.Screen] && buttons[opener{act.Screen, act.PrefillRef}] {
			out = append(out, i)
		}
	}
	return out
}

func (l *linter) dupAddControls() {
	for i := range l.app.Screens {
		s := &l.app.Screens[i]
		for _, j := range l.app.DuplicateAddToolbar(s) {
			l.add("screen %q toolbar[%d]: %q opens form %q like a button on the same screen (two add controls doing the same thing); keep the button and remove this toolbar item",
				s.ID, j, s.Toolbar[j].Label, s.Toolbar[j].Action.Screen)
		}
	}
}
