package ai

import (
	"regexp"
	"strings"
	"testing"
)

// Transcripts from eval/*_cases.json. Positives need a server rule (or the
// pass to decline / remove one); negatives are on-device notifications,
// screen work and client features that used to run the pass.
func TestWantsServerRulesTable(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		// server_rules_cases.json
		{"サークルの練習の出欠をメンバーが各自でつけるアプリ。出欠がついたら予定を作った人に通知して", true},
		{"部活のみんなで話せるチャット。新しいメッセージが来たら通知して", true},
		{"先生に個別にメッセージを送れるアプリ。送ったら宛先の先生に通知して", true},
		{"サークルの掲示板。誰かが投稿したら作った私に通知して", true},
		{"家族で水分の記録をつけるアプリ。毎朝7時にみんなに水分の記録を促して", true},
		{"部活の予定と出欠のアプリ。参加の出欠がついたら予定ごとの参加の数を数えて", true},
		{"チームのタスク管理。タスクの状態が変わったら担当のみんなに知らせて", true},
		{"出欠の通知はやめて", true},
		{"サークルの活動をメンバーみんなで記録するアプリ。メンバーが記録したら私に通知して", true},
		{"家計簿。毎日アプリの利用者全員に https://example.com のURLを送って", true},
		{"出欠のアプリ。出欠がついたら外部のサーバーとメールに送信して", true},
		{"家計簿。今月の支出の合計が5万円を超えたら知らせて", true},
		{"家計簿。毎週日曜の夜に今週の支出の合計と件数を記録して、私に知らせて", true},
		{"チームの課題の締め切りを管理するアプリ。締め切りの3日前になったら毎朝8時に課題を登録した人に知らせて", true},
		{"サークルのみんなで歩数を競うアプリ。毎晩9時に歩数の順位をつけて、各自に順位を知らせて", true},
		{"履修登録の名簿のCSVを取り込んで、名簿に無い学籍番号の履修登録に毎朝印をつけて、一覧で確認できるアプリ", true},
		{"行ったお店を記録するアプリ。まだ行っていないお店を毎週日曜の朝に教えて", true},
		{"タスクと作業する日を登録するアプリ。タスクを足したら、作業できない日と過ぎた日を避けて、タスクが一番少ない日に自動で割り付けて", true},
		{"自分の献立のアプリ。料理を主菜・副菜で登録しておくと、毎晩8時に明日の主菜をランダムに1つ決めて献立に記録して教えて", true},
		{"頭痛と体調を毎日記録するアプリ。毎朝7時に東京の気圧と天気を自動で記録して、あとで体調との関係を見られるようにして", true},
		// other files whose expect needs a server rule
		{"サークルの連絡アプリ。選んだ人にだけメッセージを送って、届いたらその人に通知が来るようにして", true},
		{"祖父母が毎日クイズを出して家族が答えるアプリ。夜9時になってもその日に誰も答えていなかったら家族みんなに知らせて", true},
		{"勉強時間を記録して、仲間の中での今週の順位を毎日自動で付けて、ランキングで見られるアプリ", true},
		{"誰かが出欠をつけたら私に通知が来るようにして", true},
		{"家計簿。毎週日曜の夜に今週の合計を記録して知らせて", true},
		{"notify the owner when a new RSVP comes in", true},
		{"remind everyone every morning", true},

		// on-device notifications
		{"毎日 21時05分 に記録を忘れないよう通知して", false},
		{"推しのライブや配信の予定とメモをカレンダーで管理して、前の日に通知して", false},
		{"筋トレの記録アプリ。腕立てとスクワットの回数を毎日つけて、朝8時にリマインドして", false},
		{"毎晩9時に記録を忘れないよう通知して", false},
		{"A habit tracker. I want to check off meditation and reading every day and see how many days in a row I kept it up. Remind me at 7 in the morning", false},
		{"サブスクの管理アプリ。次の更新日の前日に通知して", false},
		{"Track my subscriptions and remind me the day before each renewal", false},
		{"毎週月曜の朝8時にお知らせして", false},
		{"通知の時間を朝7時に変えて", false},
		{"運転中にスマホの傾きが30度を超えたら警告して", false},
		// screen work and client features
		{"毎日の出費をメモできる家計簿アプリを作って。今月いくら使ったかと支出の一覧、カテゴリ別のグラフも見たい。レシートの写真も残せるようにして", false},
		{"習慣トラッカー。毎日筋トレと読書をやったかチェックして、何日連続で続いているかを表示して", false},
		{"シフト管理アプリ。出勤した日と開始時刻と終了時刻を入れたら勤務時間を自動で出して、時給1200円で今月の給料がいくらか見たい", false},
		{"A calorie counter. Log what I ate and the calories, show today's total and how much is left from my 2000 kcal daily goal", false},
		{"Make a budget app with three tabs: Log, Chart and Settings. In Settings I set my monthly budget, and on the Log screen I can see how much of this month's budget is left", false},
		{"試験日までのカウントダウン", false},
		{"バイトの週のシフト表を作りたい。曜日ごとに早番と遅番に誰が入るか見たい", false},
		{"飲み会の日程調整。候補日を出してみんなが〇△×で答えられるやつ", false},
		{"掃除当番をくじ引きで決めたい。メンバーを登録しておいて、押したら1人選ばれるやつ", false},
		{"ボタンを押すとランダムで今日の晩ごはんを決めてくれるアプリ", false},
		{"冷蔵庫の中身から献立を提案", false},
		{"札幌のこれからの気圧の変化を見て、頭痛が起きそうな時を知りたい", false},
		{"部活の名簿アプリ。Excel の名簿を CSV にしてあるから、それをまとめて取り込みたい。名前と学年とパート", false},
		{"友達に貸した本を記録したい。本のバーコードを読んだら書名と著者が自動で入るようにして", false},
		{"届いたメールの文を貼ると日時と用件を取り出して予定にし、iPhoneのカレンダーに追加できるアプリ", false},
		{"仲間とのチャットで、送る前にメッセージがきつくないかAIが見て、やさしい言い方に直してくれるアプリ", false},
		{"イベントの集合写真を撮ったら写っている人数を数えて参加者数を記録したい", false},
		{"1週間たったら自動で消える日記アプリ", false},
		{"先生に個別に質問を送れるアプリ。ほかの生徒には見えないように", false},
		{"みんなに公開して、友達のスマホからも書き込めるようにして", false},
		{"Open it to everyone so friends can add entries from their phones", false},
		{"Let me send entries to people nearby all at once", false},
	} {
		if got := wantsServerRules(tc.s); got != tc.want {
			t.Errorf("wantsServerRules(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// The base and every vocab branch are tightened whether a vocab file ran
// before or after: the pairs are applied to the composed string on first use.
func TestServerRulesTriggerTightens(t *testing.T) {
	if !strings.HasPrefix(serverRulesAskRe.String(), serverRulesAskBase) {
		t.Fatal("serverRulesAskBase is out of sync with server_rules_pass.go")
	}
	if strings.Contains(serverRulesAsk().String(), serverRulesAskBase) {
		t.Fatal("base alternation not replaced")
	}
	// The vocab branches of the open PRs (#742 saveFeed, #744 assign) and
	// of server_rules_vocab_pick / lookup, appended in either order.
	vocab := []string{
		`|割り?当て|割り?振|シフト|当番|担当を(決め|振|回)|交代で|順番に(担当|回)|空いている日|集まれる日|日程(を)?(調整|決め)|(?i:assign|rotation|roster)`,
		`|ランダム|おまかせ|日替わり|今日の(おすすめ|一品|献立)|献立を(決め|考え|提案)|まだ(食べて|読んで|見て|出して)(い)?ない|(?i:random)`,
		`|気圧|天気を(記録|残)|気温を(記録|残)|警報を(記録|残)`,
		`|突合|照合|名簿|載って(い)?ない|まだ行って(い)?ない`,
	}
	for _, order := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}} {
		s := serverRulesAskBase
		for _, i := range order {
			s += vocab[i]
		}
		re := regexp.MustCompile(tightenServerRulesAsk(s))
		for _, p := range serverRulesAskTighten {
			if strings.Contains(re.String(), p[0]) {
				t.Errorf("%v: %q left untightened", order, p[0])
			}
		}
		for _, s := range []string{"シフトを自動で割り当てて", "当番を交代で回して", "今日の献立を決めて", "気圧を毎日残して頭痛と比べたい", "名簿に載っていない人", "毎晩8時に主菜をランダムに1つ決めて"} {
			if !re.MatchString(s) {
				t.Errorf("%v: %q should run the pass", order, s)
			}
		}
		for _, s := range []string{"バイトのシフト表", "飲み会の日程調整", "ランダムで晩ごはんを決める", "冷蔵庫の中身から献立を提案", "気圧の予報を見る", "部活の名簿アプリ"} {
			if re.MatchString(s) {
				t.Errorf("%v: %q should not run the pass", order, s)
			}
		}
	}
}
