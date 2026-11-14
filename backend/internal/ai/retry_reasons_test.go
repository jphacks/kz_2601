package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A create rejected by validation, then accepted, logs retry_reasons=validation
// (labels only: no transcript, DSL, appId or raw uid).
func TestGenerateLog_RetryReasonsCreate(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{withBadEntity(t, kb), string(kb)}})
	ctx := auth.ContextWithUID(context.Background(), rawUID)
	m, raw := genLog(t, svc, func() {
		if _, err := svc.Generate(ctx, Request{AppID: "app-RAW-1", Transcript: secretPrompt}); err != nil {
			t.Fatal(err)
		}
	})
	if m["outcome"] != GenRegenerated || m["retry_reasons"] != RetryValidation {
		t.Fatalf("line: %v", m)
	}
	for _, leak := range []string{"SECRET-PROMPT", "app-RAW-1", rawUID, "ghost", "entities"} {
		if strings.Contains(raw, leak) {
			t.Errorf("log leaks %q: %s", leak, raw)
		}
	}
}

// An edit whose patches fail twice falls back to a full rewrite: every
// rejected attempt is labeled, in order. A patch that does not apply is
// invalid output, not a validation failure.
func TestGenerateLog_RetryReasonsEditDowngrade(t *testing.T) {
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
	if m["outcome"] != GenRegenerated || m["downgraded_to_full"] != true {
		t.Fatalf("line: %v", m)
	}
	want := RetryInvalidOutput + "," + RetryInvalidOutput + "," + RetryInvalidOutput
	if m["retry_reasons"] != want {
		t.Fatalf("retry_reasons = %v, want %s", m["retry_reasons"], want)
	}
	if strings.Contains(raw, "ghost") || strings.Contains(raw, "一覧") {
		t.Fatalf("log leaks content: %s", raw)
	}
}

// A first-try success has no retry_reasons.
func TestGenerateLog_NoRetryReasons(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kb)}})
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って"}); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := m["retry_reasons"]; ok {
		t.Fatalf("line: %v", m)
	}
}

// retryReason: patch_* codes only = invalid_output; any other code (or a mix)
// = validation; max_tokens wins.
func TestRetryReasonPatchCodes(t *testing.T) {
	_, _, patchErr := dsl.ApplyPatch([]byte(`{}`), nil) // patch_empty
	var ve *dsl.ValidationError
	if !errors.As(patchErr, &ve) || ve.Codes()[0] != dsl.CodePatchEmpty {
		t.Fatalf("patch error %v", patchErr)
	}
	if got := retryReason(patchErr, nil); got != RetryInvalidOutput {
		t.Fatalf("patch_empty: %s", got)
	}
	_, valErr := dsl.Parse([]byte(`{"version":1}`))
	if !errors.As(valErr, &ve) || patchOnly(ve.Codes()) {
		t.Fatalf("validation error %v", valErr)
	}
	if got := retryReason(valErr, nil); got != RetryValidation {
		t.Fatalf("validation: %s", got)
	}
	if got := retryReason(patchErr, &Response{StopReason: "max_tokens"}); got != RetryTruncated {
		t.Fatalf("truncated: %s", got)
	}
	if patchOnly(nil) || !patchOnly([]string{dsl.CodePatchApply, dsl.CodePatchInvalid}) || patchOnly([]string{dsl.CodePatchApply, "schema_required"}) {
		t.Fatal("patchOnly")
	}
}
