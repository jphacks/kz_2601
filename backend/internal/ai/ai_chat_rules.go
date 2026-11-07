package ai

import "strings"

// aiChatRules: when the generator should use aiChat (a multi-turn talk with
// the cloud AI, text_chat.go) instead of aiButton. Appended like aiLongRules
// so prompt.go and catalog.go stay untouched. The routing lines are written
// 「…」は aiChat so a client without aiChat drops them (restrict.go).
var aiChatRules = strings.TrimSpace(`
## アプリの中の AI と何往復も話す(aiChat)
- 「AI に相談したい」「悩みを聞いてほしい」「AI カウンセラー」「AI の家庭教師に質問」「英会話・面接・プレゼンの練習相手」「AI と雑談・壁打ち」は aiChat(画面下の入力欄から送ると AI が直近の会話を踏まえて返事をし、両方を記録に残す)。
  - 形: entity turn{text: text(required), role: enum[user, assistant]}(scope は付けない = いつも private。共有のアプリの中でも会話の entity は private にする: 会話は本人だけのもので、shared / public はエラー。日時の field は作らない)+ 会話のタブかホームに aiChat{entity: turn, textField: text, roleField: role, persona: 「…」}(A1:F12・同じ画面に他の部品を置かない)。turn のフォーム画面・追加ボタン・list は作らない。
  - persona に役・口調・やり方を 500 字以内で書く。例: カウンセラー「やさしく話を聞く相談相手。まず気持ちを受け止め、1 回に 1 つだけ質問する。診断や薬の話はせず、つらい時は身近な人や専門の窓口に相談するようすすめる」、家庭教師「中学生の数学の先生。答えをすぐ言わず、ヒントを 1 つずつ出す」、英会話「簡単な英語で返し、文法の間違いを最後に 1 つだけやさしく直す」。
  - 説明・添削など長めの返事が要る時だけ length: long。
- aiButton との使い分け: 1 回押して 1 回書かせる(要約・分類・清書・一言コメント・フォームの項目を埋める)は aiButton。続けて話す・前の答えを踏まえて聞き返す・練習の相手は aiChat。
- 既存アプリへの「AI に相談できるようにして」は、会話の entity と aiChat の画面(タブ)を add する。日記などの記録を AI に読ませる必要は無い(aiChat は会話だけを読む)。記録を材料にまとめたいなら aiButton。
- aiChat は端末内 AI では動かず、いつも Tailor It Pro のオンライン AI で動く(共有のアプリは作成者の Pro)。AI が読むのは直近 20 発言(16KB)まで。医療・法律・お金の判断はしない。summary で料金に触れなくてよい。
`)

func init() { systemPrompt += "\n\n" + aiChatRules }
