package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func choreApp(scope dsl.EntityScope, edit dsl.EntityEdit, toggle bool) *dsl.App {
	app := dsl.Initial()
	app.App.Name = "家事分担"
	app.Entities = []dsl.Entity{{ID: "chore", Name: "家事", Scope: scope, Edit: edit, Fields: []dsl.Field{
		{ID: "title", Name: "家事", Type: dsl.FieldString},
		{ID: "when", Name: "日付", Type: dsl.FieldDate},
	}}}
	item := &dsl.ListItem{Title: "title"}
	if toggle {
		item.Toggle = "when"
		item.Resets = "daily"
	}
	app.Screens[0].Components = []dsl.Component{{Type: dsl.CompList, Entity: "chore", Item: item}}
	return &app
}

// #249 / #252: the spoken summary states who may change shared records.
func TestSummaryStatesSharedEditRule(t *testing.T) {
	members := choreApp(dsl.ScopePublic, dsl.EditMembers, true)
	if got := summarize(members); !strings.Contains(got, "家事は参加者の誰でも更新でき、削除は作った人だけができます。") {
		t.Errorf("members ja: %q", got)
	}
	members.Entities[0].Name = "Chore"
	if got := summarizeEN(members); !strings.Contains(got, "Anyone in the app can update chores; only the creator can delete them.") {
		t.Errorf("members en: %q", got)
	}

	authorOnly := choreApp(dsl.ScopePublic, "", true)
	if got := summarize(authorOnly); !strings.Contains(got, "家事は作った人だけが変更できます。") {
		t.Errorf("author-only with toggle ja: %q", got)
	}

	// a plain board (no check) and private records say nothing extra
	for name, app := range map[string]*dsl.App{
		"board":   choreApp(dsl.ScopePublic, "", false),
		"private": choreApp("", dsl.EditMembers, true),
	} {
		if got := summarize(app); strings.Contains(got, "作った人") {
			t.Errorf("%s: unexpected edit note %q", name, got)
		}
	}
}
