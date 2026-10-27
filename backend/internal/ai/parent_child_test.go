package ai

import (
	"context"
	"os"
	"strings"
	"testing"
)

func postComments(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/dsl/examples/post_comments.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The model sees prefillRef in the catalog, the generation schema and the
// parent -> child rules.
func TestPrefillRefTaught(t *testing.T) {
	if !strings.Contains(catalog, "prefillRef") {
		t.Error("catalog lacks navigate.prefillRef")
	}
	if !strings.Contains(string(GenerationSchema()), `"prefillRef"`) {
		t.Error("generation schema lacks prefillRef")
	}
	for _, want := range []string{"## 親から子を作る(#256", `"prefillRef":"<その ref field id>"`, "同じ entity のフォーム"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

// A posts list whose onItemTap opens the comment form (the d_quiz_game bug)
// is rejected and the regeneration prompt names the fix.
func TestCreate_CrossEntityTapRegenerates(t *testing.T) {
	good := postComments(t)
	bad := strings.Replace(good,
		`"onItemTap": { "type": "navigate", "screen": "post_form", "presentation": "push" }`,
		`"onItemTap": { "type": "navigate", "screen": "comment_form", "presentation": "push" }`, 1)
	if bad == good {
		t.Fatal("fixture did not change")
	}
	f := &fakeCaller{replies: []string{bad, good}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "投稿にコメントできるアプリ"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", res.Meta.Attempts)
	}
	second := f.calls[1].Messages[0].Content
	for _, want := range []string{`opens screen "comment_form", the form of entity "post_comments"`, `screen ids: post_form`, `set prefillRef to one of "post"`} {
		if !strings.Contains(second, want) {
			t.Errorf("regeneration prompt lacks %q:\n%s", want, second)
		}
	}
	if !strings.Contains(string(res.DSL), `"prefillRef":"post"`) {
		t.Errorf("prefillRef dropped from the stored DSL: %s", res.DSL)
	}
}
