package triage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSimilarClusters(t *testing.T) {
	ctx := context.Background()
	e := FakeEmbedder{Dim: 256}
	reps := []ClusterRep{
		{Key: "photo-add-crash", Count: 3, Summary: "写真を追加するとアプリが終了する。"},
		{Key: "dark-mode", Count: 1, Summary: "暗い配色の切り替えの要望。"},
		{Key: "photo-save", Count: 2, Summary: "写真を追加するとアプリが終了します。"},
		{Key: "new-one", Count: 9, Summary: "写真を追加するとアプリが終了する。"}, // itself
		{Key: "empty", Count: 1, Summary: " "},
	}
	got, err := SimilarClusters(ctx, e, "new-one", "写真を追加するとアプリが終了する", reps, DefaultSimilarThreshold)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range got {
		keys = append(keys, s.Key)
		if s.Sim < DefaultSimilarThreshold || s.Sim > 1 {
			t.Fatalf("sim %+v", s)
		}
	}
	if strings.Join(keys, ",") != "photo-add-crash,photo-save" || got[0].Count != 3 {
		t.Fatalf("got %+v", got)
	}
	// a high threshold leaves nothing; nothing to compare is not an error
	if got, err := SimilarClusters(ctx, e, "x", "写真を追加する", reps, 1); err != nil || len(got) != 0 {
		t.Fatalf("threshold 1: %+v %v", got, err)
	}
	if got, err := SimilarClusters(ctx, e, "x", "写真", nil, 0.5); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no reps: %+v %v", got, err)
	}
	// at most MaxSimilar, nearest first
	many := []ClusterRep{}
	for _, k := range []string{"d", "c", "b", "a"} {
		many = append(many, ClusterRep{Key: k, Count: 1, Summary: "同じ要約"})
	}
	if got, _ := SimilarClusters(ctx, e, "x", "同じ要約", many, 0.5); len(got) != MaxSimilar || got[0].Key != "a" {
		t.Fatalf("cap: %+v", got)
	}
	// an embedder error surfaces
	if _, err := SimilarClusters(ctx, failEmbedder{}, "x", "写真", reps, 0.5); err == nil {
		t.Fatal("embedder error swallowed")
	}
}

type failEmbedder struct{}

func (failEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("down")
}

func TestEmbedFromEnv(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	for _, v := range []string{"", "off"} {
		if e, _, err := EmbedFromEnv(env(EnvEmbed, v)); e != nil || err != nil {
			t.Fatalf("%q: %v %v", v, e, err)
		}
	}
	e, th, err := EmbedFromEnv(env(EnvEmbed, "on"))
	if e == nil || th != DefaultSimilarThreshold || err != nil {
		t.Fatalf("on: %v %v %v", e, th, err)
	}
	if _, th, err := EmbedFromEnv(env(EnvEmbed, "on", EnvEmbedThreshold, "0.9")); th != 0.9 || err != nil {
		t.Fatalf("threshold: %v %v", th, err)
	}
	for _, bad := range [][]string{{EnvEmbed, "yes"}, {EnvEmbed, "on", EnvEmbedThreshold, "0"}, {EnvEmbed, "on", EnvEmbedThreshold, "1.5"}} {
		if _, _, err := EmbedFromEnv(env(bad...)); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}
