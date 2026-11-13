package eval

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

var t0 = time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)

func item(id, text string, at time.Time, ai bool) loop.Item {
	return loop.Item{ID: id, Source: loop.SourceInApp, UID: "uid-" + id, Text: text, AIAllowed: ai, Status: loop.ItemNew, IngestedAt: at}
}

// The template holds the week's items the model may see, masked again, with
// empty labels; it is not loadable until a person labels it, and then it
// runs through Run.
func TestTemplateRoundTrip(t *testing.T) {
	rejected := item("c", "共有したら真っ白", t0.Add(2*time.Hour), false)
	rejected.HoldReason = loop.HoldRejected
	piiHeld := item("d", "名前が出てしまう", t0.Add(3*time.Hour), false)
	piiHeld.HoldReason = loop.HoldPII
	items := []loop.Item{
		item("b", "写真を追加すると落ちる 連絡は taro@example.com まで", t0.Add(time.Hour), true),
		item("a", "ダークモードが欲しい", t0, true),
		rejected,
		piiHeld, // the model must not see it again
		item("e", "先週の分", t0.Add(-time.Hour), true),     // before the window
		item("f", "来週の分", t0.Add(8*24*time.Hour), true), // after the window
	}
	var b strings.Builder
	ex, err := Template(&b, items, t0, t0.Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if ex.InWindow != 3 || ex.Written != 3 || ex.Skipped != 0 {
		t.Fatalf("export %+v", ex)
	}
	out := b.String()
	if strings.Contains(out, "taro@example.com") || !strings.Contains(out, "[メール]") || strings.Contains(out, "uid-") {
		t.Fatalf("template leaks:\n%s", out)
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		var l templateLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatal(err)
		}
		if l.Category != "" || l.Priority != "" || l.Bundle != "" || !strings.Contains(line, `"bundle":""`) {
			t.Fatalf("labels not empty: %s", line)
		}
		ids = append(ids, l.ID)
	}
	if strings.Join(ids, ",") != "a,b,c" {
		t.Fatalf("ids %v (want oldest first)", ids)
	}

	if _, err := Load(strings.NewReader(out)); err == nil || !strings.Contains(err.Error(), "not labelled yet") {
		t.Fatalf("unlabelled template loaded: %v", err)
	}
	labelled := strings.NewReplacer(
		`"category":"","priority":"","bundle":""}`, `"category":"bug","priority":"P2","bundle":"x"}`,
	).Replace(out)
	cases, err := Load(strings.NewReader(labelled))
	if err != nil || len(cases) != 3 {
		t.Fatalf("labelled: %d cases, %v", len(cases), err)
	}
	if _, err := Run(context.Background(), Rules{}, cases, Hooks{}); err != nil {
		t.Fatal(err)
	}
}

// More than MaxCases items are spread over the window, not cut at the start.
func TestTemplateThinsEvenly(t *testing.T) {
	n := 3 * MaxCases
	items := make([]loop.Item, n)
	for i := range items {
		items[i] = item("id"+strconv.Itoa(100000+i), "落ちる "+strconv.Itoa(i), t0.Add(time.Duration(i)*time.Minute), true)
	}
	var b strings.Builder
	ex, err := Template(&b, items, t0, t0.Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if ex.InWindow != n || ex.Written != MaxCases {
		t.Fatalf("export %+v", ex)
	}
	if !strings.Contains(b.String(), `"id":"id100000"`) || !strings.Contains(b.String(), `"id":"id`+strconv.Itoa(100000+n-3)+`"`) {
		t.Fatal("template does not span the window")
	}
}

// Run masks each text before the model sees it, so a hand-made set cannot
// send personal data.
func TestRunMasksText(t *testing.T) {
	var seen string
	m := modelFunc(func(user string) { seen += user })
	cases := []Case{{ID: "1", Text: "090-1234-5678 に電話ください、落ちます", Category: "bug", Priority: "P2"}}
	if _, err := Run(context.Background(), m, cases, Hooks{}); err != nil {
		t.Fatal(err)
	}
	if seen == "" || strings.Contains(seen, "1234") {
		t.Fatalf("model saw: %s", seen)
	}
}

type modelFunc func(user string)

func (f modelFunc) Complete(ctx context.Context, system, user string, schema json.RawMessage) (string, triage.Usage, error) {
	f(user)
	return Rules{}.Complete(ctx, system, user, schema)
}

// The confusion tables count every case, an unclassified one in its own
// column; Below names the fields under the bar.
func TestReportConfusionAndVerdict(t *testing.T) {
	rep, err := Run(context.Background(), constModel{}, loadSet(t), Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.CategoryTable.Count("bug", "question"); got == 0 {
		t.Fatalf("bug->question %d", got)
	}
	total := 0
	for _, r := range rep.CategoryTable.Rows {
		for _, c := range rep.CategoryTable.Cols {
			total += rep.CategoryTable.Count(r, c)
		}
	}
	if total != rep.Cases {
		t.Fatalf("table counts %d of %d cases", total, rep.Cases)
	}
	below := strings.Join(rep.Below(Bar), ",")
	if !strings.Contains(below, "category") || !strings.Contains(below, "bundle-apart") {
		t.Fatalf("below %q", below)
	}
	s := rep.String()
	for _, want := range []string{"verdict:  below 80%", "category (rows: person, columns: model)", "bundle pairs"} {
		if !strings.Contains(s, want) {
			t.Fatalf("report lacks %q:\n%s", want, s)
		}
	}

	rules, err := Run(context.Background(), Rules{}, loadSet(t), Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if b := rules.Below(Bar); len(b) != 0 || !strings.Contains(rules.String(), "every field at or above 80%") {
		t.Fatalf("rules below %v", b)
	}
}
