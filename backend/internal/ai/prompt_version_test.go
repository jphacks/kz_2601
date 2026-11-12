package ai

import (
	"context"
	"regexp"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// The saved version records the model and prompt that generated it (#372).
func TestGenerate_SourceRecordsPromptVersion(t *testing.T) {
	store := appconfigclient.NewMemory()
	f := &fakeCaller{replies: []string{string(kakeibo(t))}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	if _, err := svc.Generate(context.Background(), Request{AppID: "pv1", Transcript: "家計簿"}); err != nil {
		t.Fatal(err)
	}
	vs, err := store.Versions(context.Background(), "pv1")
	if err != nil || len(vs) == 0 || vs[0].Source == nil {
		t.Fatalf("versions: %+v %v", vs, err)
	}
	if src := vs[0].Source; src.Kind != "ai" || src.Model != "test-model" || src.PromptVersion != PromptVersion() {
		t.Fatalf("source = %+v, want ai / test-model / %s", src, PromptVersion())
	}
}

func TestPromptVersionStable(t *testing.T) {
	v := PromptVersion()
	if !regexp.MustCompile(`^p-[0-9a-f]{12}$`).MatchString(v) {
		t.Fatalf("PromptVersion() = %q, want p-<12 hex>", v)
	}
	if again := PromptVersion(); again != v {
		t.Fatalf("PromptVersion() changed: %q then %q", v, again)
	}
}
