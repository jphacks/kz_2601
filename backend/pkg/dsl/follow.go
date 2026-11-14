package dsl

import "fmt"

// Follows (#270): people follow each other inside one shared app, and a
// component shows only the posts of the people the signed-in user follows.
// The relation itself lives in appconfig (apps_public/{appId}/follows/{uid}),
// not in the DSL; the DSL only reads it:
//   - filter {"field":"$author","op":"eq","value":{"myFollows":true}}
//     (neq = posts of people I do not follow)
//   - action follow / unfollow: the author of the record on screen.
const (
	ActionFollow   ActionType = "follow"
	ActionUnfollow ActionType = "unfollow"
)

// isMyFollowsValue reports whether a decoded filter value is {"myFollows": true}.
func isMyFollowsValue(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return false
	}
	b, ok := m["myFollows"].(bool)
	return ok && b
}

// IsFollowAction reports follow / unfollow.
func IsFollowAction(t ActionType) bool { return t == ActionFollow || t == ActionUnfollow }

// checkFollowAction: follow / unfollow act on the author of the record on
// screen, so the entity must be shared/public.
func checkFollowAction(where string, act *Action, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[act.Entity]
	switch {
	case act.Entity == "":
		add("%s: %s needs entity (the shared/public entity whose poster to follow)", where, act.Type)
	case e == nil:
		add("%s: %s entity %q does not exist", where, act.Type, act.Entity)
	case !e.IsShared():
		add("%s: %s follows the poster of a shared/public record; entity %q is private (every record is the user's own)", where, act.Type, act.Entity)
	}
}

// checkFollowsFilter validates {"myFollows":true} on a component filter: only
// with field "$author" and op eq / neq. where already ends in ".filter".
func checkFollowsFilter(where string, f *Filter, e *Entity, add func(string, ...any)) {
	if f.Field != RecordAuthor {
		add(`%s: value {"myFollows":true} is only for field %q (the poster)`, where, RecordAuthor)
	} else if e != nil && !e.IsShared() {
		add(`%s: filter by people I follow is only for shared/public entities (entity %q is private, so every record is the user's own; remove the filter)`, where, e.ID)
	}
	if f.Op != "eq" && f.Op != "neq" {
		add(`%s: {"myFollows":true} takes op eq (people I follow) or neq (people I do not follow), not %q`, where, f.Op)
	}
}

// FollowUses lists where the app reads follows: "screen:type" of components
// filtering by {"myFollows":true}, and of follow / unfollow actions.
func FollowUses(a *App) (filters, actions []string) {
	for _, s := range a.Screens {
		for i, tb := range s.Toolbar {
			if IsFollowAction(tb.Action.Type) {
				actions = append(actions, fmt.Sprintf("%s:toolbar[%d]", s.ID, i))
			}
		}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			at := s.ID + ":" + string(c.Type)
			if c.Filter != nil && isMyFollowsValue(c.Filter.Value) {
				filters = append(filters, at)
			}
			for _, act := range []*Action{c.Action, c.OnItemTap} {
				if act != nil && IsFollowAction(act.Type) {
					actions = append(actions, at)
				}
			}
		})
	}
	return filters, actions
}
