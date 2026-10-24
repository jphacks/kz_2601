package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #164: each entity is read with the control that really opens its form
// (割り勘: members from the toolbar, expenses from the bottom-right button).
func TestSummaryNamesEachAddControl(t *testing.T) {
	app, err := dsl.Parse([]byte(fewShotWarikan))
	if err != nil {
		t.Fatal(err)
	}
	if got := summarize(app); !strings.Contains(got, "右上の「メンバー追加」からメンバーを、右下のボタンから支出を追加できます。") {
		t.Errorf("ja: %q", got)
	}
	for i := range app.Entities {
		app.Entities[i].Name = map[string]string{"members": "Member", "expenses": "Expense"}[app.Entities[i].ID]
	}
	if got := summarizeEN(app); !strings.Contains(got, `Use the "メンバー追加" button at the top right to add members and the button at the bottom right to add expenses.`) {
		t.Errorf("en: %q", got)
	}
}
