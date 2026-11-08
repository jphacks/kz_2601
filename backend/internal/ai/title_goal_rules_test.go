package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// One goal section: the title, goal and goal-rate bullets all in it, and a
// build without single gets the latest variant of both goal bullets.
func TestTitleGoalRules(t *testing.T) {
	at := strings.Index(systemPrompt, "## 一覧の見出しと目標値")
	if at < 0 {
		t.Fatal("create prompt lacks the goal section")
	}
	sec := systemPrompt[at:]
	if i := strings.Index(sec[1:], "\n## "); i >= 0 {
		sec = sec[:i+1]
	}
	for _, want := range []string{titleGoalSingleLine, goalRateSetting, "progressBar の target・gauge の max"} {
		if !strings.Contains(sec, want) {
			t.Errorf("goal section lacks %q", want)
		}
	}
	if strings.Count(systemPrompt, "progressBar の target・gauge の max は数値しか書けない") != 1 {
		t.Error("the goal-rate bullet must appear once (one goal section)")
	}
	old := NewClientRestriction(allComponentTypes(), nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching})
	if !old.NoSingle() {
		t.Fatal("want a build without singleEntity")
	}
	got := withoutNote(restrictPrompt(withRestriction(context.Background(), old), systemPrompt))
	if strings.Contains(got, titleGoalSingleLine) || !strings.Contains(got, titleGoalLatestLine) {
		t.Error("no-single prompt still tells the model to use \"single\": true for goals")
	}
	if strings.Contains(got, goalRateSetting) || !strings.Contains(got, goalRateLatest) {
		t.Error("no-single prompt still reads the goal rate with setting")
	}
}

// The goal snippets must validate on a goal / budget entity without a date
// field (goal{title,target_minutes}, budget{amount}): "by":"date" did not.
func TestTitleGoalSnippetWithoutDateField(t *testing.T) {
	snip := regexp.MustCompile(`\{"agg":"latest"[^}]*\}`)
	app := func(value string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"budget","name":"予算","fields":[{"id":"amount","name":"予算額","type":"currency"}]}],
"screens":[{"id":"home","title":"ホーム","components":[{"type":"stat","label":"予算","value":` + value + `}]}]}`)
	}
	for name, line := range map[string]string{"single": titleGoalSingleLine, "latest": titleGoalLatestLine} {
		got := snip.FindString(line)
		if got == "" {
			t.Fatalf("%s: no latest snippet in %q", name, line)
		}
		if _, err := dsl.Parse(app(got)); err != nil {
			t.Errorf("%s: snippet %s does not validate without a date field: %v", name, got, err)
		}
	}
	// the old snippet fails there, so the check above is not vacuous
	if _, err := dsl.Parse(app(`{"agg":"latest","entity":"budget","field":"amount","by":"date"}`)); err == nil || !strings.Contains(err.Error(), `latest by "date" not in entity "budget"`) {
		t.Errorf(`"by":"date" on an entity without date must fail on by: %v`, err)
	}

	for _, s := range []string{goalRateSetting, goalRateLatest} {
		var c map[string]any
		if err := json.Unmarshal([]byte(s), &c); err != nil {
			t.Fatalf("snippet is not JSON: %v\n%s", err, s)
		}
		if c["type"] != "stat" || c["format"] != "percent" || c["value"] == nil || strings.Contains(s, `"by"`) {
			t.Errorf("snippet = %s", s)
		}
	}
	raw := `{"schemaVersion":1,"app":{"name":"勉強","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"study","name":"勉強","fields":[{"id":"minutes","name":"分","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
 {"id":"goal","name":"目標","fields":[{"id":"title","name":"名前","type":"string"},{"id":"target_minutes","name":"目標(分)","type":"integer"}]}],
"screens":[{"id":"home","title":"ホーム","components":[` + goalRateLatest + `]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("goalRateLatest rejected for a goal entity without a date field: %v", err)
	}
}
