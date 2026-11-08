package ai

import "strings"

// jphacksProducts3Rules: the third batch of past JPHACKS works
// (eval/jphacks_products3_cases.json) whose parts exist but had no mapping.
// Appended so prompt.go stays untouched.
var jphacksProducts3Rules = strings.TrimSpace(`
## 続けて育つ・1 人 1 票・計った時間・レベル・文字数・合っているかの確認
- 「毎日続けると植物・キャラが育つ」「○日続けたら収穫・ご褒美」: 毎日チェック(item.toggle の date field done_on・resets daily)の entity に computed streak(integer・unit 日) = {"op":"streak","args":[{"ref":"done_on"}]} と段階 stage(string) = {"if":{"op":"gte","args":[{"ref":"streak"},21]},"then":"収穫できる","else":{"if":{"op":"gte","args":[{"ref":"streak"},7]},"then":"つぼみ","else":{"if":{"op":"gte","args":[{"ref":"streak"},1]},"then":"芽","else":"種"}}}(日数は頼まれた数に)。list.item の subtitle に stage・trailing に streak。動く絵は作れないと summary で伝える。
- 「候補から 1 人 1 票で投票して一番多いものを決める」: 候補 shop{name}と票 vote{shop: ref → shop}(どちらも public・vote に "unique":["$author"])。shop の computed votes(integer) = {"agg":"count","entity":"vote","filter":{"field":"shop","op":"eq","value":{"ref":"id"}}} を list の trailing と sort desc に、一番多い候補 = stat.value {"agg":"latest","entity":"shop","field":"name","filter":[{"field":"votes","op":"eq","value":{"agg":"max","entity":"shop","field":"votes"}},{"field":"votes","op":"gt","value":0}]}(同点は後に足した候補)。候補の一覧から navigate + prefillRef で票のフォームを開く。いくつでも押せる「いいね」なら marks。
- 「タイマーで計った時間を記録に残す」「ポモドーロの回数・集中した時間をみんなで見る」: 記録 session{seconds: integer, date: date defaultValue now}のフォーム画面(screen.entity)に timerDisplay{mode: countdown, seconds: 1500, field: seconds}(一時停止・終了で計った秒が入り、保存はユーザー)。分の入力欄は作らない。今日の合計(分) = stat.value {"op":"round","args":[{"op":"div","args":[{"agg":"sum","entity":"session","field":"seconds","filter":{"field":"date","op":"today","value":null}},60]}]}。みんなで見るなら session を shared にして list の subtitle に $author。
- 「記録するほどレベルが上がる」「経験値」「次のレベルまであと何回」: レベル = stat.value {"op":"add","args":[{"op":"floor","args":[{"op":"div","args":[{"agg":"count","entity":"meal"},5]}]},1]}(5 回ごとに 1 上がる)・あと = stat.value {"op":"sub","args":[5,{"op":"mod","args":[{"agg":"count","entity":"meal"},5]}]}。点数で上がるなら count を {"agg":"sum",...,"field":"points"} に。育つ絵は作れないと summary で伝える。
- 「取り込んだ PDF・文の文字数を数える」: fileImport{accept: [pdf, text], field: body} の entity に computed chars(integer・unit 字) = {"op":"len","args":[{"ref":"body"}]} を list の trailing に。ヘッダーや空白を自動で除くことはできないと summary で伝える。
- 「投稿された情報が合っているかをみんなで確かめる」(設備・お店の情報): 情報 facility{name, place: location}(public・mapView)と確認 check{facility: ref → facility, answer: enum [ある, ない]}(public・"unique":["facility","$author"] = 1 人 1 回)。facility の computed yes(integer) = {"agg":"count","entity":"check","filter":[{"field":"facility","op":"eq","value":{"ref":"id"}},{"field":"answer","op":"eq","value":"ある"}]}・no も同じ形・判定 status(string) = {"if":{"op":"gt","args":[{"ref":"yes"},{"ref":"no"}]},"then":"ある","else":{"if":{"op":"gt","args":[{"ref":"no"},{"ref":"yes"}]},"then":"ない","else":"未確認"}}。地図と一覧の行から navigate + prefillRef で確認のフォームを開く。
`)

func init() { systemPrompt += "\n\n" + jphacksProducts3Rules }
