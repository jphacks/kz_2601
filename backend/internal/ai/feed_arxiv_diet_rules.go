package ai

import "strings"

// feedArxivDietRules: the arxiv (arXiv の分野の新着論文) and diet (国会会議録の
// 発言の検索) feeds. Like feedRules, every line contains "」は feed" so a
// build without feed drops them all. NHK のニュースは規約(個人の利用だけ・
// プログラムでの再提供を許さない)で入れていない (external-feeds.md).
var feedArxivDietRules = strings.TrimSpace(`
- 「arXiv の新着論文」「AI・機械学習・自然言語処理の最新論文」「論文を追う」は feed{provider: arxiv, feedId: 分野, title: 〇〇の新着論文, limit: 10}(全幅)。feedId は cs.AI(人工知能)/ cs.CL(自然言語処理)/ cs.CV(画像)/ cs.LG(機械学習)/ cs.RO(ロボット)/ cs.HC(HCI)/ cs.SE(ソフトウェア)/ cs.CR(セキュリティ)/ stat.ML のどれか 1 つ(分からなければ cs.AI)。行は題名・著者・投稿日で、要約・翻訳・キーワード検索はできない。読んだ論文のメモは別の entity と list で作ってよい。
- 「国会の議論」「国会で〇〇がどう話されたか」「国会会議録」は feed{provider: diet, query: 探す語(40 字まで・例 こども家庭庁), title: 国会で「〇〇」, limit: 10}。行は発言者(会派)・院と会議名・日付で、タップで会議録の発言を開く。query はアプリに決めて書く(利用者の入力・記録の値では検索できない)。発言の要約・賛否の判定はできない。NHK などのニュースは feed では読めない(feed の rss の 3 つと arxiv・diet だけ)。
`)

func init() { systemPrompt += "\n\n" + feedArxivDietRules }
