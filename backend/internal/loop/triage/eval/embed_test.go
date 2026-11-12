package eval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// splitModel gives one problem two cluster names (the case #2148 targets).
type splitModel struct{}

func (splitModel) Complete(_ context.Context, _, user string, _ json.RawMessage) (string, triage.Usage, error) {
	rows := []map[string]any{}
	for _, m := range reBlock.FindAllStringSubmatch(user, -1) {
		cluster, summary, cat := "dark-mode", "暗い配色の切り替えの要望。", "feature"
		switch {
		case strings.Contains(m[2], "写真"):
			cluster, summary, cat = "photo-add-crash", "写真を追加するとアプリが終了する。", "bug"
		case strings.Contains(m[2], "画像"):
			cluster, summary, cat = "image-crash", "写真を追加するとアプリが終了します。", "bug"
		}
		rows = append(rows, map[string]any{"id": m[1], "category": cat, "area": "render", "impact": "high",
			"cluster": cluster, "summary": summary, "pii_suspected": false})
	}
	b, err := json.Marshal(map[string]any{"items": rows})
	return string(b), triage.Usage{}, err
}

func TestCompareEmbed(t *testing.T) {
	ctx := context.Background()
	cases := []Case{
		{ID: "1", Text: "写真を追加すると落ちる", Category: "bug", Priority: "P1", Bundle: "photo"},
		{ID: "2", Text: "画像を入れたら強制終了", Category: "bug", Priority: "P1", Bundle: "photo"},
		{ID: "3", Text: "ダークモードが欲しい", Category: "feature", Priority: "P3"},
	}
	rep, err := Run(ctx, splitModel{}, cases, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Embed != nil || strings.Contains(rep.String(), "embedding merge") {
		t.Fatal("measured without being asked")
	}
	if err := rep.CompareEmbed(ctx, triage.FakeEmbedder{Dim: 256}, 0); err != nil {
		t.Fatal(err)
	}
	c := rep.Embed
	if c == nil || c.Threshold != DefaultEmbedThreshold {
		t.Fatalf("compare %+v", c)
	}
	if c.Before.Bundles != 3 || c.Before.Mistakes() != 1 || c.After.Bundles != 2 || c.After.Mistakes() != 0 {
		t.Fatalf("before %+v after %+v", c.Before, c.After)
	}
	// before is the run's own bundling
	if c.Before.Together != rep.Together || c.Before.Apart != rep.Apart {
		t.Fatalf("before %+v report %s/%s", c.Before, rep.Together, rep.Apart)
	}
	out := rep.String()
	for _, want := range []string{"embedding merge (cosine >= 0.70)", "before         3         1", "after          2         0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// a threshold nothing reaches leaves the bundling as it was
	if err := rep.CompareEmbed(ctx, triage.FakeEmbedder{Dim: 256}, 1.01); err != nil || rep.Embed.After != rep.Embed.Before {
		t.Fatalf("no merge: %+v %v", rep.Embed, err)
	}
}

func TestCompareEmbedErrors(t *testing.T) {
	ctx := context.Background()
	if err := (&Report{}).CompareEmbed(ctx, triage.FakeEmbedder{}, 0); !errors.Is(err, ErrNoRun) {
		t.Fatalf("empty report: %v", err)
	}
	cases := []Case{
		{ID: "1", Text: "写真を追加すると落ちる", Category: "bug", Priority: "P1"},
		{ID: "2", Text: "ダークモードが欲しい", Category: "feature", Priority: "P3"},
	}
	rep, err := Run(ctx, splitModel{}, cases, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.CompareEmbed(ctx, badEmbedder{}, 0); err == nil {
		t.Fatal("embedder error swallowed")
	}
}

type badEmbedder struct{}

func (badEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("down")
}

// The rules baseline on the labelled set gives numbers both ways.
func TestCompareEmbedOnLabelledSet(t *testing.T) {
	rep, err := Run(context.Background(), Rules{}, loadSet(t), Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.CompareEmbed(context.Background(), triage.FakeEmbedder{Dim: 256}, 0); err != nil {
		t.Fatal(err)
	}
	c := rep.Embed
	if c.Before.Bundles == 0 || c.After.Bundles == 0 || c.After.Bundles > c.Before.Bundles {
		t.Fatalf("%+v", c)
	}
	t.Log("\n" + c.String())
}
