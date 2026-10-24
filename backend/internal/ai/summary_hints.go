package ai

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// addHint is one add control on the initial screen and the entities whose
// form it opens (#164: 割り勘 read "右下のボタンからメンバーと支出を追加できます"
// although members are added from the toolbar).
type addHint struct {
	where    string
	entities []string // entity names, in entity order
}

// addHints groups the entities by the control on the initial screen that
// opens their form: buttons first, then toolbar buttons. Entities with no
// such control are left out. button / toolbar render the "where" phrase.
func addHints(a *dsl.App, button func(c *dsl.Component) string, toolbar func(tb *dsl.ToolbarButton) string) []addHint {
	formEntity := map[string]string{}
	for _, s := range a.Screens {
		if s.Entity != "" {
			formEntity[s.ID] = s.Entity
		}
	}
	whereOf := map[string]string{} // entity id -> first control that opens its form
	note := func(act *dsl.Action, where string) {
		if act == nil || act.Type != dsl.ActionNavigate {
			return
		}
		if e := formEntity[act.Screen]; e != "" && whereOf[e] == "" {
			whereOf[e] = where
		}
	}
	for i := range a.Screens {
		s := &a.Screens[i]
		if s.ID != a.InitialScreen {
			continue
		}
		for j := range s.Components {
			if c := &s.Components[j]; c.Type == dsl.CompButton {
				note(c.Action, button(c))
			}
		}
		for j := range s.Toolbar {
			note(&s.Toolbar[j].Action, toolbar(&s.Toolbar[j]))
		}
	}
	var out []addHint
	idx := map[string]int{}
	for _, e := range a.Entities {
		w := whereOf[e.ID]
		if w == "" {
			continue
		}
		if i, ok := idx[w]; ok {
			out[i].entities = append(out[i].entities, e.Name)
			continue
		}
		idx[w] = len(out)
		out = append(out, addHint{where: w, entities: []string{e.Name}})
	}
	return out
}
