package ai

import "regexp"

// serverRulesRecallAsk widens the trigger for server requests the base words
// miss. A missed server rule ships a silently broken app; a false positive
// costs one small call, and the on-device dedupe drops the repeat.
const serverRulesRecallAsk = `` +
	// another person to reach
	`|(夫|妻|彼女|彼氏|親|母|父|担当者|店長|お店の人|上司|先生|生徒|お客さん|他の人|ほかの人|グループ|参加者全員)(に|にも|へ)[^。]{0,12}(通知|知らせ|届|連絡|来る|見える)` +
	// someone else's action
	`|(予約|注文|問い合わせ|提出|スレッド)が(入|あ|立|キャンセル|来)` +
	`|(夫|妻|彼女|彼氏|親|母|父|子ども|子供|息子|娘|生徒|メンバー|友達|相手|パートナー|他の人|ほかの人)が[^。]{0,12}(追加|書|提出|終え|済ませ)[^。]{0,6}たら` +
	// scheduled writes and summaries
	`|自動で[^。]{0,10}(ランキング|集計|締め|振り分|班分|記録|まとめ|つけ)` +
	`|毎(朝|晩|夜|日|週|月|年|[月火水木金土日]曜)[^。]{0,16}(まとめ|保存|記録|集計)` +
	`|(月末|月初|週末)[^。]{0,10}(まとめ|集計|締め|記録|保存)` +
	`|ローテーション` +
	// overlapping bookings
	`|同じ時間[^。]{0,10}予約|ダブルブッキング` +
	`|(?i:\b(notify|ping|text|email|let)\b[^.]{0,24}\b(my (partner|mom|dad|wife|husband|family|parents)|everyone|everybody|all (the )?(participants|members)|the (group|team))\b` +
	`|\bwhen (my|a|an) \w+( \w+)? (adds|joins|signs up|posts)` +
	`|\b(save|record|log|store|keep)s? (a|the) (\w+'s )?(daily |weekly |monthly )?(summary|total)\b` +
	`|\b(log|record|save)\b[^.]{0,12}\bweather\b|double[- ]?book)`

func init() {
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + serverRulesRecallAsk)
}
