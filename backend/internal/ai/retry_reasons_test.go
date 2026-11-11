package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
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
// rejected attempt is labeled, in order.
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
	want := RetryInvalidOutput + "," + RetryValidation + "," + RetryValidation
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
