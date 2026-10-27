package main

import (
	"regexp"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// nameIDRe / nameWordRe catch the "who" inputs #250 wants gone from shared entities
// (the poster comes from $author): member_name / person / staff_name / 名前 …
var (
	nameIDRe   = regexp.MustCompile(`(^|_)(name|person|member|staff|user|poster|author|nickname|participant|attendee)(_|$)`)
	nameWordRe = regexp.MustCompile(`名前|氏名|投稿者|ニックネーム|メンバー|参加者|スタッフ|回答者|担当者名`)
)

// nameInputs lists shared-entity string fields that look like a typed-in
// person name ("entity.field"). The entity's titleField is skipped when its
// name does not look like a person (shop_name of a ranking is the subject).
func nameInputs(a *dsl.App) []string {
	var out []string
	for i := range a.Entities {
		e := &a.Entities[i]
		if !e.IsShared() {
			continue
		}
		for _, f := range e.Fields {
			if f.Computed != nil || (f.Type != dsl.FieldString && f.Type != dsl.FieldEnum && f.Type != dsl.FieldRef) {
				continue
			}
			if nameWordRe.MatchString(f.Name) || (nameIDRe.MatchString(f.ID) && f.ID != e.TitleField) {
				out = append(out, e.ID+"."+f.ID)
			}
		}
	}
	return out
}

// authorUses lists where $author is shown and where the "only mine" filter
// is used ("screen:type").
func authorUses(a *dsl.App) (shown, me []string) {
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				at := s.ID + ":" + string(x.Type)
				if showsAuthor(&x) {
					shown = append(shown, at)
				}
				if x.Filter != nil && x.Filter.Field == dsl.RecordAuthor {
					me = append(me, at)
				}
			}
		}
	}
	return shown, me
}

func showsAuthor(c *dsl.Component) bool {
	if c.Item != nil && (c.Item.Title == dsl.RecordAuthor || c.Item.Subtitle == dsl.RecordAuthor || c.Item.Trailing == dsl.RecordAuthor) {
		return true
	}
	if c.Type == dsl.CompChatThread { // bubbles carry the sender's name (#260)
		return true
	}
	if c.Type == dsl.CompTimeline && (c.TitleField == dsl.RecordAuthor || c.SubtitleField == dsl.RecordAuthor) {
		return true
	}
	for _, col := range c.Columns {
		if col.Field == dsl.RecordAuthor {
			return true
		}
	}
	return false
}
