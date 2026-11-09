package ai

import "strings"

// jphacksProducts5Rules: the fifth batch of past JPHACKS works
// (eval/jphacks_products5_cases.json) whose parts exist but had no mapping.
// Appended so prompt.go stays untouched.
var jphacksProducts5Rules = "## 手順書と写真の判定・避難訓練のバッジ・投稿の練習・優先度と期限・店ごとの前月比・感情の分類\n" + jphacksProducts5Rest

// jphacksProducts5PhotoRule recommends aiButton inputPhoto, so it rides on
// FeatureAIInputPhoto's rules: dropped from the prompt while that is off
// (AI_TEXT_IMAGE unset, or a client without it).
var jphacksProducts5PhotoRule = strings.TrimSpace(`
- 「説明から AI が手順書(マニュアル)を作り、写真でできているかを判定」: 手順書 manual{title: string(required), request: text, steps: text}(配るなら shared)のフォームに textField(request・multiline)・aiButton{task: rewrite, input: request, output: steps, length: long, prompt: 「次の説明 {{input}} を、初めての人でもできる番号付きの手順(1 手順 1 行・注意点つき)に書き直す」}・textField(steps・multiline)。判定は子の記録 check{manual: ref→manual(required), photo: photo, result: enum [できている, 一部できている, できていない], date: date(defaultValue now)}のフォームに picker(manual)・photoField(photo・camera)・aiButton{task: classify, inputPhoto: photo, output: result, prompt: 「写真の作業が手順どおりにできているかを判定する」}・picker(result)(manual のフォームから prefillRef で開く)。manual の computed rate(integer・unit %) = {"if":{"op":"gt","args":[{"agg":"count","entity":"check","filter":{"field":"manual","op":"eq","value":{"ref":"id"}}},0]},"then":{"op":"round","args":[{"op":"mul","args":[{"op":"div","args":[{"agg":"count","entity":"check","filter":[{"field":"manual","op":"eq","value":{"ref":"id"}},{"field":"result","op":"eq","value":"できている"}]},{"agg":"count","entity":"check","filter":{"field":"manual","op":"eq","value":{"ref":"id"}}}]},100]}]},"else":0}。写真の判定は analyze ではなく inputPhoto(オンライン AI)。
`)

var jphacksProducts5Rest = strings.TrimSpace(`
- 「避難所まで歩く訓練をして、着いた避難所のバッジを集める」(AI が災害の想定を作る): shelter{name: string(required), place: location(required)}と訓練の記録 drill{shelter: ref→shelter(required), disaster: enum [地震, 大雨, 火事], scenario: text, arrived: boolean(defaultValue false), date: date(defaultValue now)}。drill のフォームに picker(shelter)・picker(disaster)・aiButton{task: generate, output: scenario, length: long, prompt: 「{{disaster}} が起きた想定で、近くの避難所まで歩いて逃げる訓練のシナリオ(起きること・気をつけること)」}・textField(scenario・multiline)・toggle(arrived)。ホームに routeGuide{entity: shelter, locationField: place, titleField: name, mode: walking}。shelter の computed reached(integer) = {"agg":"count","entity":"drill","filter":[{"field":"shelter","op":"eq","value":{"ref":"id"}},{"field":"arrived","op":"eq","value":true}]}、集めたバッジの数 = stat.value {"agg":"count","entity":"shelter","filter":{"field":"reached","op":"gt","value":0}}、shelter の list の trailing に reached。全国の避難所のデータは取り込めない(登録した所だけ)と summary で伝える。
- 「子どもが SNS の投稿を練習し、AI が個人情報・悪口を見て、大丈夫な投稿だけみんなに見せる」: post{body: text(required), safety: enum [大丈夫, 見直そう], comment: text, date: datetime(defaultValue now)}(shared)のフォームに textField(body・multiline)・aiButton{task: classify, input: body, output: safety, prompt: 「子どもの SNS 投稿の練習。名前・住所・学校・電話番号などの個人情報や、人を傷つける言葉があれば 見直そう」}・picker(safety)・aiButton{task: suggest, input: body, output: comment, prompt: 「子どもの SNS 投稿 {{input}} に、フォロワーのようにやさしく感想を書き、直すと良い所を 1 つ教える」}・textField(comment)。みんなのタイムラインは list に filter {"field":"safety","op":"eq","value":"大丈夫"}(新しい順・marks の like)、自分の投稿は filter {"field":"$author","op":"eq","value":{"me":true}} の一覧(subtitle に safety と comment)。AI の判定は目安で、保存を止めることはできないと summary で伝える。
- 「優先度と期限から、急ぐものほど大きく・赤く」(重要度と締め切りを合わせた並べ替え): task{title, priority: enum [高, 中, 低], due: date, done: boolean(defaultValue false)}に computed urgency(integer) = {"op":"add","args":[{"op":"mul","args":[10,{"if":{"op":"eq","args":[{"ref":"priority"},"高"]},"then":3,"else":{"if":{"op":"eq","args":[{"ref":"priority"},"中"]},"then":2,"else":1}}]},{"op":"sub","args":[10,{"op":"clamp","args":[{"op":"coalesce","args":[{"op":"daysUntil","args":[{"ref":"due"}]},10]},0,10]}]}]}(10〜40・期限切れは +10)と computed stage(string) = {"if":{"op":"gte","args":[{"ref":"urgency"},35]},"then":"真っ赤","else":{"if":{"op":"gte","args":[{"ref":"urgency"},25]},"then":"赤","else":{"if":{"op":"gte","args":[{"ref":"urgency"},15]},"then":"オレンジ","else":"緑"}}}。list は filter done eq false・sort urgency desc・item{title: title, subtitle: due, trailing: stage}。絵の大きさ・色そのものは変えられない(段階の言葉で出す)。
- 「お店ごとの売上を集めて、先月と比べる」(複数の店・支店・前月比): store{name}(shared)と sale{store: ref→store(required), date: date(defaultValue now), amount: integer(unit 円)}。sale の computed ago(integer・何か月前) = {"op":"sub","args":[{"op":"add","args":[{"op":"mul","args":[{"op":"year","args":[{"op":"today","args":[]}]},12]},{"op":"month","args":[{"op":"today","args":[]}]}]},{"op":"add","args":[{"op":"mul","args":[{"op":"year","args":[{"ref":"date"}]},12]},{"op":"month","args":[{"ref":"date"}]}]}]}。store の computed this_month = {"agg":"sum","entity":"sale","field":"amount","filter":[{"field":"store","op":"eq","value":{"ref":"id"}},{"field":"ago","op":"eq","value":0}]}・last_month = 同じ形で ago eq 1・growth(integer・unit %) = {"if":{"op":"gt","args":[{"ref":"last_month"},0]},"then":{"op":"round","args":[{"op":"mul","args":[{"op":"div","args":[{"ref":"this_month"},{"ref":"last_month"}]},100]}]},"else":0}。store の list は sort this_month desc・item{title: name, subtitle: this_month, trailing: growth}、全店の月ごとは chart{entity: sale, groupBy: date, bucket: month, aggregate: sum, field: amount}。先月が 0 の店の前月比は 0 と出る。
- 「メッセージの感情を AI が分けて、割合を見る」(LINE・チャットの文を貼る): msg{body: text(required), emotion: enum [うれしい, ふつう, かなしい, いらいら], date: date(defaultValue now)}のフォームに textField(body・multiline)・aiButton{task: classify, input: body, output: emotion}・picker(emotion)。ホームに chart{entity: msg, kind: pie, groupBy: emotion, aggregate: count, title: 感情の割合}と、月ごとの変化 chart{entity: msg, kind: bar, groupBy: date, bucket: month, aggregate: count}。LINE などから自動で取り込むことはできない(文は貼る)と summary で伝える。
`)

func init() {
	systemPrompt += "\n\n" + jphacksProducts5Rules + "\n\n" + jphacksProducts5PhotoRule
	for i := range clientFeatures {
		if clientFeatures[i].key == FeatureAIInputPhoto {
			clientFeatures[i].rules = append(clientFeatures[i].rules, jphacksProducts5PhotoRule)
		}
	}
}
