package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Marks (#255) expect keys, checked by checkMarks:
//
//	mark (bool)       some list item has item.mark (a like / save toggle)
//	markedByMe (bool) some component filters by "$marks.<id>" with {"me": true}
//	markEntity (bool) an entity looks like a like / save child (ref to a parent,
//	                  named like / favorite / いいね / 保存 …); false = none
var (
	markEntityIDRe   = regexp.MustCompile(`(^|_)(likes?|favou?rites?|favs?|bookmarks?|saved?|saves|stars?|reads?)(_|$)`)
	markEntityWordRe = regexp.MustCompile(`いいね|お気に入り|保存|ブックマーク|既読|スター`)
)

// markUses lists list items with a mark and "$marks" filters ("screen:type:mark").
func markUses(a *dsl.App) (toggles, mine []string) {
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				at := s.ID + ":" + string(x.Type)
				if x.Item != nil && x.Item.Mark != "" {
					toggles = append(toggles, at+":"+x.Item.Mark)
				}
				if x.Filter != nil && strings.HasPrefix(x.Filter.Field, dsl.RecordMarkPrefix) {
					mine = append(mine, at+":"+strings.TrimPrefix(x.Filter.Field, dsl.RecordMarkPrefix))
				}
			}
		}
	}
	return toggles, mine
}

// markEntities lists entities that re-create marks by hand: a ref to another
// entity and a like / save name.
func markEntities(a *dsl.App) []string {
	var out []string
	for i := range a.Entities {
		e := &a.Entities[i]
		hasRef := false
		for _, f := range e.Fields {
			if f.Type == dsl.FieldRef {
				hasRef = true
			}
		}
		if hasRef && (markEntityIDRe.MatchString(e.ID) || markEntityWordRe.MatchString(e.Name)) {
			out = append(out, e.ID)
		}
	}
	return out
}

func checkMarks(exp map[string]any, a *dsl.App) []string {
	if a == nil {
		return nil
	}
	toggles, mine := markUses(a)
	var miss []string
	for k, v := range exp {
		want, ok := v.(bool)
		if !ok {
			continue
		}
		switch k {
		case "mark":
			if (len(toggles) > 0) != want {
				miss = append(miss, fmt.Sprintf("mark=%v", toggles))
			}
		case "markedByMe":
			if (len(mine) > 0) != want {
				miss = append(miss, fmt.Sprintf("markedByMe=%v", mine))
			}
		case "markEntity":
			if got := markEntities(a); (len(got) > 0) != want {
				miss = append(miss, fmt.Sprintf("markEntity=%v", got))
			}
		}
	}
	return miss
}
