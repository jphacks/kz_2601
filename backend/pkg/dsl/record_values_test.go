package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func screenOf(t *testing.T, a *App, id string) *Screen {
	t.Helper()
	for i := range a.Screens {
		if a.Screens[i].ID == id {
			return &a.Screens[i]
		}
	}
	t.Fatalf("screen %q not found", id)
	return nil
}

func boardJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The schema's x-recordValues is what the AI catalog shows; Go must accept the same names.
func TestRecordValuesMatchSchema(t *testing.T) {
	var root struct {
		Defs struct {
			Entity struct {
				Values map[string]string `json:"x-recordValues"`
			} `json:"Entity"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range root.Defs.Entity.Values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := slices.Clone(RecordValues)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("schema x-recordValues %v != dsl.RecordValues %v", keys, want)
	}
}

func TestBoardExampleUsesRecordValues(t *testing.T) {
	app, err := Parse([]byte(boardJSON(t)))
	if err != nil {
		t.Fatal(err)
	}
	list := screenOf(t, app, "home").Components[0].Children[0]
	if list.Item.Subtitle != RecordAuthor || list.Item.Trailing != RecordCreatedAt || list.Sort.Field != RecordCreatedAt {
		t.Fatalf("board list item = %+v sort = %+v", list.Item, list.Sort)
	}
	stat := screenOf(t, app, "mine").Components[0]
	if stat.Filter.Field != RecordAuthor || !isMeValue(stat.Filter.Value) {
		t.Fatalf("mine stat filter = %+v", stat.Filter)
	}
	// round-trips with the me value intact
	out, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"value":{"me":true}`) {
		t.Fatalf("me filter lost on marshal: %s", out)
	}
}

func TestRecordValuesAcceptedOnSharedEntity(t *testing.T) {
	src := boardJSON(t)
	// timeline feed by posted time, newest first
	src = strings.Replace(src, `{ "type": "stat", "id": "count"`,
		`{ "type": "timeline", "id": "feed", "area": "A1:F2", "entity": "items", "dateField": "$createdAt", "titleField": "title", "subtitleField": "$author",
		   "filter": { "field": "$author", "op": "neq", "value": { "me": true } } },
		 { "type": "stat", "id": "count"`, 1)
	src = strings.Replace(src, `"area": "A1:F2", "label": "自分の投稿"`, `"area": "A3:F3", "label": "自分の投稿"`, 1)
	src = strings.Replace(src, `"area": "A3:F12", "title": "投稿したもの"`, `"area": "A4:F12", "title": "投稿したもの"`, 1)
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatal(err)
	}
}

func TestRecordValueErrors(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{
			name: "author on a private entity",
			old:  `"scope": "public",`, new: ``,
			want: `field "$author" is only for shared/public entities`,
		},
		{
			name: "unknown record value",
			old:  `"trailing": "$createdAt"`, new: `"trailing": "$updatedAt"`,
			want: `list item field "$updatedAt" not in entity "items"`,
		},
		{
			name: "author filter needs me",
			old:  `"filter": { "field": "$author", "op": "eq", "value": { "me": true } } },`, new: `"filter": { "field": "$author", "op": "eq", "value": "u1" } },`,
			want: `use value {"me":true}`,
		},
		{
			name: "me only with author",
			old:  `"filter": { "field": "$author", "op": "eq", "value": { "me": true } } },`, new: `"filter": { "field": "title", "op": "eq", "value": { "me": true } } },`,
			want: `value {"me":true} is only for field "$author"`,
		},
		{
			name: "author filter op",
			old:  `"filter": { "field": "$author", "op": "eq", "value": { "me": true } } },`, new: `"filter": { "field": "$author", "op": "gt", "value": { "me": true } } },`,
			want: `takes op eq (my posts) or neq`,
		},
		{
			name: "author is not sortable",
			old:  `"sort": { "field": "$createdAt", "direction": "desc" }, "emptyText"`, new: `"sort": { "field": "$updatedAt", "direction": "desc" }, "emptyText"`,
			want: `sort field "$updatedAt" not in entity "items"`,
		},
		{
			name: "me inside agg",
			old: `{ "type": "stat", "id": "count", "area": "A1:F2", "label": "自分の投稿", "entity": "items", "aggregate": "count",
          "filter": { "field": "$author", "op": "eq", "value": { "me": true } } },`,
			new:  `{ "type": "stat", "id": "count", "area": "A1:F2", "label": "自分の投稿", "value": {"agg": "count", "entity": "items", "filter": { "field": "$author", "op": "eq", "value": { "me": true } }} },`,
			want: `works only as the filter of a list/table/timeline/stat/chart component`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := boardJSON(t)
			if !strings.Contains(src, tc.old) {
				t.Fatalf("board example changed: %q not found", tc.old)
			}
			_, err := Parse([]byte(strings.Replace(src, tc.old, tc.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPrivateEntityMeFilterRejected(t *testing.T) {
	src := strings.Replace(boardJSON(t), `"scope": "public",`, ``, 1)
	src = strings.Replace(src, `"subtitle": "$author", `, ``, 1)
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), `filter by "$author" is only for shared/public entities`) {
		t.Fatalf("got %v", err)
	}
}
