package eval

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

func loadSet(t *testing.T) []Case {
	t.Helper()
	f, err := os.Open("testdata/labeled.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cases, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

// The fixed fake (Rules) on the labelled set must keep #376's bar (8 割).
// A drop means the triage pipeline (grouping, parsing, priority, drafts)
// changed how it treats the same verdicts.
func TestEvalRulesOnLabelledSet(t *testing.T) {
	cases := loadSet(t)
	if len(cases) < 20 || len(cases) > 30 {
		t.Fatalf("labelled set has %d cases, want 20-30", len(cases))
	}
	rep, err := Run(context.Background(), Rules{}, cases)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + rep.String())
	const bar = 0.8
	if rep.Category.Rate() < bar || rep.Priority.Rate() < bar {
		t.Fatalf("below %.0f%%: category %s, priority %s", 100*bar, rep.Category, rep.Priority)
	}
	if rep.Together.Rate() < bar || rep.Apart.Rate() < bar {
		t.Fatalf("bundling below %.0f%%: together %s, apart %s", 100*bar, rep.Together, rep.Apart)
	}
	// a bundle keeps its items' impact: two medium-impact feature reports
	// (g01, g02) are P2, not P3 (the draft once worked the impact back from
	// the representative's P3)
	for _, m := range rep.Misses {
		if m.Field == "priority" && (m.ID == "g01" || m.ID == "g02") {
			t.Fatalf("priority miss %+v", m)
		}
	}
}

func TestLoadRejectsBadLines(t *testing.T) {
	for name, in := range map[string]string{
		"category":  `{"id":"a","text":"x","category":"approved","priority":"P1"}`,
		"priority":  `{"id":"a","text":"x","category":"bug","priority":"P0"}`,
		"field":     `{"id":"a","text":"x","category":"bug","priority":"P1","label":"agent:ready"}`,
		"duplicate": `{"id":"a","text":"x","category":"bug","priority":"P1"}` + "\n" + `{"id":"a","text":"y","category":"bug","priority":"P1"}`,
		"no text":   `{"id":"a","text":" ","category":"bug","priority":"P1"}`,
	} {
		if _, err := Load(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A model that classifies everything wrong scores low: the scores are not
// trivially 100%.
func TestEvalScoresAWrongModelLow(t *testing.T) {
	cases := loadSet(t)
	rep, err := Run(context.Background(), constModel{}, cases)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Category.Rate() >= 0.5 || rep.Together.Rate() == 1 && rep.Apart.Rate() == 1 {
		t.Fatalf("wrong model scored %s / together %s / apart %s", rep.Category, rep.Together, rep.Apart)
	}
}

// constModel calls everything one question cluster.
type constModel struct{}

func (constModel) Complete(_ context.Context, _, user string, _ json.RawMessage) (string, triage.Usage, error) {
	rows := []map[string]any{}
	for _, m := range reBlock.FindAllStringSubmatch(user, -1) {
		rows = append(rows, map[string]any{"id": m[1], "category": "question", "area": "other", "impact": "low",
			"cluster": "all", "summary": "質問。", "pii_suspected": false})
	}
	b, _ := json.Marshal(map[string]any{"items": rows})
	return string(b), triage.Usage{InputTokens: 1}, nil
}

// #376 「似たフィードバックを束ねる」: exact duplicates are sent once and
// share the verdict; differently worded reports of one problem are bundled
// by the model's cluster slug into one draft that counts all of them.
func TestSimilarFeedbackIsBundledIntoOneDraft(t *testing.T) {
	cases := []Case{
		{ID: "1", Text: "写真を追加するとアプリが落ちる", Category: "bug", Priority: "P1", Bundle: "photo"},
		{ID: "2", Text: "写真を追加するとアプリが落ちる！", Category: "bug", Priority: "P1", Bundle: "photo"},
		{ID: "3", Text: "画像を入れたら強制終了しました", Category: "bug", Priority: "P1", Bundle: "photo"},
		{ID: "4", Text: "ダークモードが欲しい", Category: "feature", Priority: "P3"},
	}
	counter := &countingModel{m: Rules{}}
	rep, err := Run(context.Background(), counter, cases)
	if err != nil {
		t.Fatal(err)
	}
	if counter.texts != 3 {
		t.Fatalf("model saw %d texts, want 3 (the exact duplicate once)", counter.texts)
	}
	if rep.Together != (Score{3, 3}) || rep.Apart != (Score{3, 3}) {
		t.Fatalf("together %s apart %s misses %+v", rep.Together, rep.Apart, rep.Misses)
	}

	// one draft of 3 reports for the photo bundle, one for dark mode
	drafts := rep.Drafts
	var photo *loop.Draft
	for i := range drafts {
		if drafts[i].ClusterKey == "photo-add-crash" {
			photo = &drafts[i]
		}
	}
	if len(drafts) != 2 || photo == nil || photo.Count != 3 || len(photo.ItemIDs) != 3 || photo.Priority != "P1" {
		t.Fatalf("drafts %+v", drafts)
	}
}

type countingModel struct {
	m     triage.Model
	texts int
}

func (c *countingModel) Complete(ctx context.Context, system, user string, schema json.RawMessage) (string, triage.Usage, error) {
	c.texts += len(reBlock.FindAllStringSubmatch(user, -1))
	return c.m.Complete(ctx, system, user, schema)
}
