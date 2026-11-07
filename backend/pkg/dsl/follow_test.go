package dsl

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func timelineJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "timeline.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestTimelineExampleUsesFollows(t *testing.T) {
	app, err := Parse([]byte(timelineJSON(t)))
	if err != nil {
		t.Fatal(err)
	}
	filters, actions := FollowUses(app)
	if !slices.Equal(filters, []string{"following:list"}) {
		t.Errorf("filters = %v", filters)
	}
	if !slices.Equal(actions, []string{"post_form:button", "post_form:button"}) {
		t.Errorf("actions = %v", actions)
	}
}

func TestFollowsErrors(t *testing.T) {
	const filter = `"filter": { "field": "$author", "op": "eq", "value": { "myFollows": true } }`
	cases := []struct {
		name, old, new, want string
	}{
		{"filter on a field", filter, `"filter": { "field": "body", "op": "eq", "value": { "myFollows": true } }`,
			`value {"myFollows":true} is only for field "$author"`},
		{"filter op", filter, `"filter": { "field": "$author", "op": "gt", "value": { "myFollows": true } }`,
			`{"myFollows":true} takes op eq (people I follow) or neq (people I do not follow), not "gt"`},
		{"private entity", `"scope": "public",`, ``, `filter by people I follow is only for shared/public entities`},
		{"follow without entity", `"action": { "type": "follow", "entity": "posts" }`, `"action": { "type": "follow" }`,
			`follow needs entity`},
		{"unknown entity", `"action": { "type": "follow", "entity": "posts" }`, `"action": { "type": "follow", "entity": "people" }`,
			`follow entity "people" does not exist`},
		{"in agg", `"aggregate": "count",
          "filter": { "field": "$author", "op": "eq", "value": { "me": true } } },`,
			`"value": { "agg": "count", "entity": "posts", "filter": { "field": "$author", "op": "eq", "value": { "myFollows": true } } } },`,
			`works only as the filter of a list/table/timeline/stat/chart component`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := timelineJSON(t)
			if !strings.Contains(src, tc.old) {
				t.Fatalf("example has no %q", tc.old)
			}
			_, err := Parse([]byte(strings.Replace(src, tc.old, tc.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFollowsNeqAccepted(t *testing.T) {
	src := strings.Replace(timelineJSON(t), `"op": "eq", "value": { "myFollows": true }`, `"op": "neq", "value": { "myFollows": true }`, 1)
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatal(err)
	}
}
