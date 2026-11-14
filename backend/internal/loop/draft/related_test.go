package draft

import (
	"fmt"
	"testing"
)

func numbers(rs []RelatedIssue) string {
	out := []int{}
	for _, r := range rs {
		out = append(out, r.Number)
	}
	return fmt.Sprint(out)
}

func TestFindRelated(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary string
		issues  []IssueTitle
		want    string
	}{
		{"same words", "保存ボタンを押すと落ちる", []IssueTitle{{10, "保存ボタンを押すと落ちる"}, {11, "天気の表示が遅い"}}, "[10]"},
		{"nothing alike", "保存ボタンを押すと落ちる", []IssueTitle{{1, "天気の表示が遅い"}, {2, "Dark mode colors"}}, "[]"},
		{"english words, case", "Crash when saving a note", []IssueTitle{{3, "crash on SAVING note"}, {4, "Add dark mode"}}, "[3]"},
		{"japanese and english", "iOS で保存すると crash する", []IssueTitle{{5, "iOS: 保存で crash"}, {6, "Android の通知"}}, "[5]"},
		{"full-width latin", "ＰＤＦの書き出し", []IssueTitle{{7, "PDF の書き出しが失敗"}}, "[7]"},
		{"empty title", "保存できない", []IssueTitle{{8, ""}, {9, "  "}}, "[]"},
		{"empty summary", "", []IssueTitle{{1, "保存できない"}}, "[]"},
		{"bad number", "保存できない", []IssueTitle{{0, "保存できない"}, {-1, "保存できない"}}, "[]"},
		{"tie goes to newer, max 3", "保存できない", []IssueTitle{{1, "保存できない"}, {4, "保存できない"}, {2, "保存できない"}, {3, "保存できない"}}, "[4 3 2]"},
		{"closest first", "保存ボタンで落ちる", []IssueTitle{{20, "保存ボタンで落ちる"}, {21, "保存ボタンの色"}, {22, "保存"}}, "[20 21]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FindRelated(tc.summary, tc.issues)
			if numbers(got) != tc.want {
				t.Errorf("got %v want %s", got, tc.want)
			}
			for _, r := range got {
				if r.Score < MinRelatedScore || r.Score > 1 {
					t.Errorf("score %v", r.Score)
				}
			}
		})
	}
}

func TestTerms(t *testing.T) {
	got := terms("iOSで保存、Crash!")
	for _, w := range []string{"ios", "で保", "保存", "crash"} {
		if !got[w] {
			t.Errorf("missing %q in %v", w, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("%v", got)
	}
}
