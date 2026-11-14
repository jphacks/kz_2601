package ai

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// validation_codes is "code:count,..." of known codes only.
var codeCountsRe = regexp.MustCompile(`^[a-z_]+:[0-9]+(,[a-z_]+:[0-9]+)*$`)

// A create rejected by validation (an entity that does not exist) logs the
// rule's code on the regenerated line, never the transcript, DSL, appId or uid.
func TestGenerateLog_ValidationCodesCreate(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{withBadEntity(t, kb), string(kb)}})
	ctx := auth.ContextWithUID(context.Background(), rawUID)
	m, raw := genLog(t, svc, func() {
		if _, err := svc.Generate(ctx, Request{AppID: "app-RAW-1", Transcript: secretPrompt}); err != nil {
			t.Fatal(err)
		}
	})
	codes, _ := m["validation_codes"].(string)
	if m["outcome"] != GenRegenerated || !codeCountsRe.MatchString(codes) || !strings.Contains(codes, dsl.CodeBadRef+":") {
		t.Fatalf("line: %v", m)
	}
	for _, leak := range []string{"SECRET-PROMPT", "app-RAW-1", rawUID, "ghost", "does not exist"} {
		if strings.Contains(raw, leak) {
			t.Errorf("log leaks %q: %s", leak, raw)
		}
	}
}

// Codes of every attempt rejected by validation add up (the patch path does
// not exist: patch_apply twice); the attempt that was not JSON adds a retry
// reason but no code.
func TestGenerateLog_ValidationCodesEditDowngrade(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	badPatch := `{"patch":[{"op":"replace","path":"/screens/0/components/0/entity","value":"ghost"}],"summary":"x"}`
	svc := &Service{Claude: &fakeCaller{replies: []string{"not json", badPatch, badPatch, string(kb)}}, Store: store, Model: "test-model"}
	m, raw := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "一覧を直して"}); err != nil {
			t.Fatal(err)
		}
	})
	codes, _ := m["validation_codes"].(string)
	if codes != dsl.CodePatchApply+":2" {
		t.Fatalf("validation_codes = %q (line %v)", codes, m)
	}
	if strings.Contains(raw, "ghost") || strings.Contains(raw, "一覧") {
		t.Fatalf("log leaks content: %s", raw)
	}
}

// A first-try success has no validation_codes.
func TestGenerateLog_NoValidationCodes(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kb)}})
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って"}); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := m["validation_codes"]; ok {
		t.Fatalf("line: %v", m)
	}
}

func TestCodeCounts(t *testing.T) {
	if got := codeCounts([]string{"bad_ref", "missing_field", "bad_ref"}); got != "bad_ref:2,missing_field:1" {
		t.Fatalf("codeCounts = %q", got)
	}
}
