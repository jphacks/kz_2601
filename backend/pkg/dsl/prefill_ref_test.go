package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prefillDoc: posts + comments (comment.post -> posts). TAP = posts list
// onItemTap, BTN = button action on post_form, PICK = picker for comment.post.
const prefillDoc = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating","tabs":[{"screen":"home","title":"投稿"},{"screen":"all","title":"コメント"}]},"initialScreen":"home",
"entities":[
 {"id":"posts","name":"投稿","titleField":"title","fields":[{"id":"title","name":"本文","type":"string"}]},
 {"id":"comments","name":"コメント","titleField":"text","fields":[{"id":"post","name":"投稿","type":"ref","refEntity":"posts","required":true},{"id":"text","name":"コメント","type":"string"}]}],
"screens":[
 {"id":"home","title":"投稿","toolbar":[{"label":"投稿","action":{"type":"navigate","screen":"post_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"posts","item":{"title":"title"},"onItemTap":TAP}]},
 {"id":"post_form","title":"投稿","entity":"posts","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"posts"}}],
  "components":[{"type":"textField","field":"title"},{"type":"button","label":"コメントを書く","action":BTN}]},
 {"id":"all","title":"コメント","components":[{"type":"list","entity":"comments","item":{"title":"text"},"onItemTap":{"type":"navigate","screen":"comment_form"}}]},
 {"id":"comment_form","title":"コメント","entity":"comments","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"comments"}}],
  "components":[PICK{"type":"textField","field":"text"}]}]}`

const (
	tapEdit      = `{"type":"navigate","screen":"post_form","presentation":"push"}`
	btnPlain     = `{"type":"navigate","screen":"comment_form","presentation":"sheet"}`
	btnPrefill   = `{"type":"navigate","screen":"comment_form","presentation":"sheet","prefillRef":"post"}`
	pickerOnPost = `{"type":"picker","field":"post"},`
)

func prefillRaw(tap, btn, pick string) string {
	return strings.NewReplacer("TAP", tap, "BTN", btn, "PICK", pick).Replace(prefillDoc)
}

func TestPrefillRefValidation(t *testing.T) {
	cases := []struct {
		name, tap, btn, pick string
		want                 string // "" = valid
	}{
		{"edit tap + plain button", tapEdit, btnPlain, pickerOnPost, ""},
		{"button on parent form", tapEdit, btnPrefill, pickerOnPost, ""},
		{"tap creates child", `{"type":"navigate","screen":"comment_form","prefillRef":"post"}`, btnPlain, pickerOnPost, ""},
		// the quiz bug: a posts row opening the comments form in edit mode
		{"tap to other entity form", `{"type":"navigate","screen":"comment_form"}`, btnPlain, pickerOnPost,
			`tapping a "posts" record opens screen "comment_form", the form of entity "comments"`},
		{"cross tap names the fix", `{"type":"navigate","screen":"comment_form"}`, btnPlain, pickerOnPost,
			`Point it at the form of "posts" (screen ids: post_form), or to add a new "comments" for the tapped "posts" set prefillRef to one of "post"`},
		{"tap to screen without entity", `{"type":"navigate","screen":"all"}`, btnPlain, pickerOnPost, ""},
		{"picker removed", tapEdit, btnPrefill, "", `screen "comment_form" must keep a picker for "post"`},
		{"unknown field", tapEdit, `{"type":"navigate","screen":"comment_form","prefillRef":"parent"}`, pickerOnPost,
			`prefillRef "parent" is not a field of entity "comments"; use one of "post"`},
		{"not a ref", tapEdit, `{"type":"navigate","screen":"comment_form","prefillRef":"text"}`, pickerOnPost,
			`prefillRef "text" must be a stored ref field`},
		{"target not a form", tapEdit, `{"type":"navigate","screen":"all","prefillRef":"post"}`, pickerOnPost,
			`prefillRef needs a form screen (screen.entity)`},
		{"wrong parent", tapEdit, `{"type":"navigate","screen":"post_form","prefillRef":"title"}`, pickerOnPost,
			`prefillRef "title" must be a stored ref field`},
		{"not navigate", tapEdit, `{"type":"back","prefillRef":"post"}`, pickerOnPost, `prefillRef is only allowed on navigate`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(prefillRaw(c.tap, c.btn, c.pick)))
			if c.want == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// A button with prefillRef needs a current record: on a screen without
// entity there is none.
func TestPrefillRefNeedsCurrentRecord(t *testing.T) {
	raw := strings.Replace(prefillRaw(tapEdit, btnPlain, pickerOnPost),
		`"components":[{"type":"list","entity":"comments"`,
		`"components":[{"type":"button","label":"書く","action":{"type":"navigate","screen":"comment_form","prefillRef":"post"}},{"type":"list","entity":"comments"`, 1)
	_, err := Parse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), `prefillRef needs a current record to put in "post": move this button to the form screen of "posts"`) {
		t.Fatalf("got %v", err)
	}
}

// The parent of the current record must match the ref's refEntity
// (comment_form's own record is a comment, not a post).
func TestPrefillRefParentMismatch(t *testing.T) {
	raw := strings.Replace(prefillRaw(tapEdit, btnPlain, pickerOnPost),
		`PICK`, ``, 1)
	raw = strings.Replace(raw, `{"type":"textField","field":"text"}]}]}`,
		`{"type":"textField","field":"text"},{"type":"button","label":"返信","action":{"type":"navigate","screen":"comment_form","prefillRef":"post"}}]}]}`, 1)
	_, err := Parse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), `prefillRef "post" points to "posts", but the current record here is a "comments"`) {
		t.Fatalf("got %v", err)
	}
}

// calendar / gridTable taps keep their own FormPrefill (date / cell) and
// still must open a form of their own entity.
func TestCalendarTapOtherEntity(t *testing.T) {
	raw := strings.Replace(prefillRaw(tapEdit, btnPlain, pickerOnPost),
		`"fields":[{"id":"title","name":"本文","type":"string"}]`,
		`"fields":[{"id":"title","name":"本文","type":"string"},{"id":"on","name":"日付","type":"date"}]`, 1)
	cal := func(act string) string {
		return strings.Replace(raw, `"onItemTap":`+tapEdit+`}]}`,
			`"onItemTap":`+tapEdit+`},{"type":"calendar","entity":"posts","dateField":"on","onDateTap":`+act+`}]}`, 1)
	}
	if _, err := Parse([]byte(cal(`{"type":"navigate","screen":"post_form"}`))); err != nil {
		t.Fatalf("own form: %v", err)
	}
	_, err := Parse([]byte(cal(`{"type":"navigate","screen":"comment_form"}`)))
	if err == nil || !strings.Contains(err.Error(), `.onDateTap: tapping a "posts" record opens screen "comment_form"`) {
		t.Fatalf("other form: %v", err)
	}
	_, err = Parse([]byte(cal(`{"type":"navigate","screen":"comment_form","prefillRef":"post"}`)))
	if err == nil || !strings.Contains(err.Error(), `prefillRef is not supported here`) {
		t.Fatalf("prefill: %v", err)
	}
}

// The canonical example opens the comment form from the post with prefillRef
// and stays lint-clean.
func TestPostCommentsExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "post_comments.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"prefillRef": "post"`) {
		t.Fatal("example lost its prefillRef")
	}
	if w := app.Lint(); len(w) > 0 {
		t.Fatalf("lint: %v", w)
	}
}
