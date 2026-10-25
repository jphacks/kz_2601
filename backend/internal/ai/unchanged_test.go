package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// #211: a test-only patch changes nothing: not saved, unchanged=true, summary kept.
func TestEdit_UnchangedIsNotSaved(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	noop := `{"summary":"項目は変更できませんでした。理由: 何の項目かが決まっていないため。","patch":[{"op":"test","path":"/entities/0/id","value":"expense"}]}`
	f := &fakeCaller{replies: []string{noop}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "項目を足して"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Unchanged || res.Version != res.PreviousVersion || len(res.Patch) != 0 || !strings.Contains(res.Summary, "変更できませんでした") {
		t.Fatalf("result: %+v", res)
	}
	env, err := store.Get(context.Background(), "a1")
	if err != nil || env.Version != res.Version {
		t.Fatalf("store version changed: %v / %d vs %d", err, env.Version, res.Version)
	}
	body, _ := json.Marshal(res)
	if !strings.Contains(string(body), `"unchanged":true`) {
		t.Fatalf("json: %s", body)
	}
}

// A real change keeps unchanged out of the JSON (old clients see the same body).
func TestEdit_ChangedOmitsUnchanged(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	p := `{"summary":"カテゴリにカフェを追加しました。","patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"}]}`
	svc := &Service{Claude: &fakeCaller{replies: []string{p}}, Store: store, Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "カテゴリにカフェを追加して"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(res)
	if res.Unchanged || res.Version == res.PreviousVersion || strings.Contains(string(body), "unchanged") {
		t.Fatalf("result: v=%d prev=%d %s", res.Version, res.PreviousVersion, body)
	}
}

func TestSameDSL(t *testing.T) {
	if !sameDSL([]byte(`{"a":1,"b":[1,2]}`), []byte(`{ "b":[1,2], "a":1 }`)) {
		t.Fatal("same")
	}
	if sameDSL([]byte(`{"a":1}`), []byte(`{"a":2}`)) {
		t.Fatal("different")
	}
}
