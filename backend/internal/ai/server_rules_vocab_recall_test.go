package ai

import "testing"

func TestWantsServerRulesRecall(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		// another person to reach
		{"夫にも連絡がいくように", true},
		{"締め切りの前日に担当者に連絡", true},
		{"お客さんの予約が入ったらスマホに来るように", true},
		{"母にも見えるようにして", true},
		// someone else's action
		{"掲示板に新しいスレッドが立ったら", true},
		{"予約がキャンセルされたら店長へ", true},
		{"問い合わせがあったら担当者へ", true},
		{"子どもが宿題を終えたら親に", true},
		{"妻が買い物リストに追加したら", true},
		// scheduled writes and summaries
		{"毎夜その日の記録をまとめて保存", true},
		{"毎金曜にその週の売上をまとめて", true},
		{"月末に支出を集計して残しておいて", true},
		{"掃除当番をローテーションで", true},
		// overlap
		{"同じ時間に2人が予約できないようにして", true},
		{"ダブルブッキングを防ぎたい", true},
		// English
		{"text my mom when I get home", true},
		{"ping the group when someone signs up", true},
		{"email all participants the agenda", true},
		{"let the team know about it", true},
		{"when a new member joins", true},
		{"Every Sunday night, save a summary of the week's habits", true},
		{"record the day's total at midnight", true},
		{"log the weather", true},
		{"prevent double-booking", true},

		// screen work stays quiet
		{"体重を記録して折れ線グラフにして", false},
		{"母の日のプレゼントリスト", false},
		{"予約の一覧を見たい", false},
		{"a to-do list where I can drag to reorder", false},
		{"Make a reading log with a star rating for each book", false},
	} {
		if got := wantsServerRules(tc.s); got != tc.want {
			t.Errorf("wantsServerRules(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
