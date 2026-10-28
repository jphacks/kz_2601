package dsl

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func recipesJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "recipes.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRecipesExampleUsesMarks(t *testing.T) {
	app, err := Parse([]byte(recipesJSON(t)))
	if err != nil {
		t.Fatal(err)
	}
	e := app.EntityByID("recipes")
	if e == nil || !slices.Equal(e.MarkIDs(), []string{"save"}) || e.MarkByID("save").Icon != "bookmark" {
		t.Fatalf("recipes marks = %+v", e)
	}
	home := screenOf(t, app, "home").Components[0].Children[0]
	if home.Item.Mark != "save" {
		t.Fatalf("home item = %+v", home.Item)
	}
	saved := screenOf(t, app, "saved").Components[1].Children[0]
	if saved.Filter.Field != "$marks.save" || !isMeValue(saved.Filter.Value) || saved.Item.Mark != "save" {
		t.Fatalf("saved list = %+v / %+v", saved.Filter, saved.Item)
	}
	popular := screenOf(t, app, "popular").Components[0].Children[0]
	if popular.Sort.Field != "$marks.save" {
		t.Fatalf("popular sort = %+v", popular.Sort)
	}
}

func TestMarksAccepted(t *testing.T) {
	src := recipesJSON(t)
	// a like mark next to save, shown as a count and filtered as "not liked yet"
	src = strings.Replace(src, `"marks": [{ "id": "save", "icon": "bookmark", "label": "保存" }]`,
		`"marks": [{ "id": "save", "icon": "bookmark", "label": "保存" }, { "id": "like", "icon": "heart", "label": "いいね" }]`, 1)
	src = strings.Replace(src, `"item": { "title": "title", "subtitle": "$author", "image": "photo", "mark": "save" }`,
		`"item": { "title": "title", "subtitle": "$author", "trailing": "$marks.like", "image": "photo", "mark": "like" }, "filter": { "field": "$marks.like", "op": "neq", "value": { "me": true } }`, 1)
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatal(err)
	}
}

func TestMarkErrors(t *testing.T) {
	const marks = `"marks": [{ "id": "save", "icon": "bookmark", "label": "保存" }]`
	const item = `"item": { "title": "title", "subtitle": "$author", "image": "photo", "mark": "save" }`
	const filter = `"filter": { "field": "$marks.save", "op": "eq", "value": { "me": true } } },
        {`
	cases := []struct {
		name, old, new, want string
	}{
		{
			name: "marks on a private entity",
			old:  `"scope": "public",`, new: ``,
			want: `entity "recipes" marks are only for shared/public entities`,
		},
		{
			name: "unknown icon",
			old:  marks, new: `"marks": [{ "id": "save", "icon": "pin", "label": "保存" }]`,
			want: `/entities/0/marks/0/icon: value must be one of`,
		},
		{
			name: "duplicate mark",
			old:  marks, new: `"marks": [{ "id": "save", "icon": "bookmark", "label": "保存" }, { "id": "save", "icon": "star", "label": "保存" }]`,
			want: `duplicate mark id "save"`,
		},
		{
			name: "empty label",
			old:  marks, new: `"marks": [{ "id": "save", "icon": "bookmark", "label": " " }]`,
			want: `marks[0].label is empty`,
		},
		{
			name: "item mark not declared",
			old:  item, new: strings.Replace(item, `"mark": "save"`, `"mark": "like"`, 1),
			want: `entity "recipes" has no mark "like" (marks: save)`,
		},
		{
			name: "count of an undeclared mark",
			old:  `{ "field": "$marks.save", "title": "保存" }`, new: `{ "field": "$marks.like", "title": "いいね" }`,
			want: `has no mark "like"`,
		},
		{
			name: "mark filter needs me",
			old:  filter, new: strings.Replace(filter, `{ "me": true }`, `true`, 1),
			want: `field "$marks.save" checks whether I marked the record; use value {"me":true}`,
		},
		{
			name: "mark filter op",
			old:  filter, new: strings.Replace(filter, `"op": "eq"`, `"op": "gte"`, 1),
			want: `takes op eq (marked by me) or neq (not marked by me)`,
		},
		{
			name: "mark inside agg",
			old: `{ "type": "stat", "id": "count", "area": "A1:F2", "label": "保存したレシピ", "entity": "recipes", "aggregate": "count",
          "filter": { "field": "$marks.save", "op": "eq", "value": { "me": true } } },`,
			new:  `{ "type": "stat", "id": "count", "area": "A1:F2", "label": "保存したレシピ", "value": {"agg": "count", "entity": "recipes", "filter": { "field": "$marks.save", "op": "eq", "value": { "me": true } }} },`,
			want: `marks are not records`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := recipesJSON(t)
			if !strings.Contains(src, tc.old) {
				t.Fatalf("recipes example changed: %q not found", tc.old)
			}
			_, err := Parse([]byte(strings.Replace(src, tc.old, tc.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
