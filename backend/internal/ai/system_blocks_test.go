package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Edit sends [systemPrompt][editRules] as two cached blocks: the first is
// byte-identical to create's for the same restriction (shared cache prefix),
// and the blocks read as the old single systemPrompt+"\n"+editRules string.
func TestEditSystemBlocks_ShareCreatePrefix(t *testing.T) {
	var all []string
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	a, d := reviewBuild()
	cases := map[string]Restriction{
		"none":      {},
		"review":    NewRestriction(a, d),
		"oldClient": NewClientRestriction(all, nil, nil),
		"noShared":  NewRestriction(nil, []string{FeatureSharedData}),
		"noCamera":  NewRestriction(nil, []string{FeatureCameraCapture, FeatureFieldHealth}),
	}
	for name, r := range cases {
		ctx := withRestriction(context.Background(), r)
		f := &fakeCaller{replies: []string{"{}", "{}"}}
		s := &Service{Claude: f}
		if _, err := s.call(ctx, systemPrompt, "u", GenerationSchema(), &attemptResult{}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.callWith(ctx, systemPrompt, editSystemSuffix(), "u", PatchGenerationSchema(), &attemptResult{}, nil); err != nil {
			t.Fatal(err)
		}
		create, edit := f.calls[0], f.calls[1]
		if create.SystemSuffix != "" || edit.System != create.System || edit.SystemSuffix == "" {
			t.Fatalf("%s: edit's first block differs from create's", name)
		}
		// The restriction note closes the first block instead of the whole
		// prompt; everything else matches the old single string exactly.
		note := ""
		if r.Active() {
			note = "\n\n" + restrictionNote(r)
		}
		old := restrictPrompt(ctx, systemPrompt+"\n"+editRules)
		if got := strings.TrimSuffix(edit.System, note) + edit.SystemSuffix + note; got != old {
			t.Fatalf("%s: blocks do not concatenate to the old prompt (len %d vs %d)", name, len(got), len(old))
		}
	}
}

// callWith runs serverPushPrompt on the first block only: the edit suffix
// must hold none of the push-wording anchors (#760).
func TestEditSystemSuffix_NoServerPushAnchors(t *testing.T) {
	if s := editSystemSuffix(); rewriteServerPush(s) != s {
		t.Fatal("editRules holds a server-push anchor; callWith must rewrite the suffix too")
	}
}

// Both blocks go on the wire as cached text blocks, in order (2 of the 4
// cache breakpoints).
func TestClientSendsSystemSuffixBlock(t *testing.T) {
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okBody("claude-test"))
	}))
	defer srv.Close()
	c, _ := testClient(srv, "claude-test")
	for _, tc := range []struct {
		suffix string
		want   int
	}{{"", 1}, {"\nedit", 2}} {
		if _, err := c.Complete(context.Background(), Completion{System: "base", SystemSuffix: tc.suffix, Messages: []Message{{Role: "user", Content: "u"}}}); err != nil {
			t.Fatal(err)
		}
		var wr wireRequest
		if err := json.Unmarshal(sent, &wr); err != nil {
			t.Fatal(err)
		}
		if len(wr.System) != tc.want || wr.System[0].Text != "base" {
			t.Fatalf("system blocks: %+v", wr.System)
		}
		for _, b := range wr.System {
			if b.CacheControl["type"] != "ephemeral" {
				t.Fatalf("block not cached: %+v", b)
			}
		}
		if tc.want == 2 && wr.System[1].Text != "\nedit" {
			t.Fatalf("suffix block: %+v", wr.System[1])
		}
	}
}
