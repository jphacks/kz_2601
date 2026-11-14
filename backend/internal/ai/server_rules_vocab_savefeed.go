package ai

import (
	"regexp"
	"strings"
)

// Server-rule saveFeed (a time trigger saves a weather / pressure / warnings
// snapshot as a record, dsl/server_rules_feed.go). This init runs after
// server_rules_vocab_pick.go (files initialize in name order): it swaps
// lines of the base prompt and the reads section, then appends its own.

// serverRulesSaveFeedLines: prompt line -> replacement.
var serverRulesSaveFeedLines = [][2]string{
	{`- do(1〜5 個・1 個に 1 種類だけ): notify{to,title,body} / createRecord{entity,values} / updateRecord{ref?,values} / increment{ref,field,by?}。`,
		`- do(1〜5 個・1 個に 1 種類だけ): notify{to,title,body} / createRecord{entity,values} / updateRecord{ref?,values} / increment{ref,field,by?} / saveFeed{provider,region,entity,fields}(下の「天気・気圧の記録」)。`},
	{`外部のデータの取得。`, `外部のデータの取得(天気・気圧・警報を毎日記録するのは saveFeed で書ける。それ以外の外部のデータは不可)。`},
}

var serverRulesSaveFeedSection = strings.TrimSpace(`
## 天気・気圧の記録(saveFeed)
- saveFeed {"provider":"weather|pressure|warnings","region":"<府県予報区コード 6 桁>","entity":"<記録する entity>","fields":{"date":"<date field>","summary":"<string/text field>","min":"<number field>","max":"<number field>","drop":"<boolean field>"}}: 時刻のきっかけ(each 無し)の時に、公開の天気データのその時の値を entity に記録 1 件として足す。予報しか見られないものを履歴にして、後で体調などと比べられる。アプリに 3 つまで。
- weather = 今日の天気(summary 天気・min / max 気温 °C)。pressure = これから 24 時間の気圧(summary・min / max hPa・drop 6 hPa 以上の急な低下)。warnings = 今出ている警報・注意報(summary だけ・無ければ「発表なし」)。date は実行の日。fields は使うものだけ書く(summary・min・max・drop のどれか 1 つは要る)。required の field は date か summary にする。
- region は feed と同じコード(130000 東京都・170000 石川県・270000 大阪府・016000 札幌)。場所の言及が無ければ 130000(東京都)を使う。利用者の位置や記録の値は送らない。
- entity はルールと同じ側(private は作成者の記録)。画面は記録の entity の list や chart で履歴を見せる。データが取れない日は記録しない。
- 体調と気圧: 「毎朝 7 時に東京の気圧と天気を記録して、頭痛との関係を見たい」(private の pressure_log{day:date, note:string, low:number, high:number, falling:boolean}・weather_log{day:date, weather:string, low:number, high:number})= on {every:"daily",time:"07:00"}・do [{"saveFeed":{"provider":"pressure","region":"130000","entity":"pressure_log","fields":{"date":"day","summary":"note","min":"low","max":"high","drop":"falling"}}},{"saveFeed":{"provider":"weather","region":"130000","entity":"weather_log","fields":{"date":"day","summary":"weather","min":"low","max":"high"}}}]。
`)

func init() {
	for _, l := range serverRulesSaveFeedLines {
		serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1)
		serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
	}
	serverRulesReadsSection += "\n\n" + serverRulesSaveFeedSection // the prompt still ends with the section
	serverRulesPrompt += "\n\n" + serverRulesSaveFeedSection
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + `|気圧|天気を(記録|残)|気温を(記録|残)|警報を(記録|残)`)
}
