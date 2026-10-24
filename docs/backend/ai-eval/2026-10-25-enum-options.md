# ai-eval 2026-10-25: 選択肢(enum)の編集 (#194)

claude-sonnet-5-5 / effort low・ローカルの in-memory store。土台は `examples/okozukai.json`(種類 = お小遣い・給与・ボーナス)と `examples/kakeibo.json`。前 = 896e639 のプロンプト、後 = このブランチ。2 周の合計(raw JSON はコミットしない)。

## /v1/generate(edit)直接・各ケース 2 回 × 2 周

| ケース | 発話 | 前 | 後 |
|---|---|---|---|
| enum_add | 副業も足して | 4/4 | 4/4 |
| enum_remove | ボーナスは要らない | 4/4 | 4/4 |
| enum_rename | お小遣いを臨時収入に変えて | 4/4 | 4/4 |
| enum_user | 種類を自分で増やせるようにして | 0/4(ref にはなるが初期値が無い) | 4/4 |
| enum_user2 | カテゴリを自由に追加したり消したりできるようにしたい | 0/4(同上) | 4/4 |

合格 = options だけが変わり field 数が同じ(メモ欄・「その他」なし)/ 自分で増やす依頼は ref + マスタのフォーム + 初期の選択肢(`refPick`)。

## chat-first(/v1/chat mode=edit → ready の transcript か案 1 の instruction → /v1/generate)× 2 周

| ケース | 発話 | 前 | 後 |
|---|---|---|---|
| edit_enum_add | 副業も足して | 2/2 | 2/2 |
| edit_enum_remove | ボーナスは要らない | 2/2 | 2/2 |
| edit_enum_rename | お小遣いを臨時収入に変えて | 2/2 | 2/2 |
| edit_enum_user | 種類を自分で増やせるようにして | 0/2(transcript が「文字で自由に入力」→ 文字入力欄が足される) | 2/2 |
| edit_enum_vague | 種類をもっと増やしたい | 0/2(question で止まる) | 2/2(proposal: 選択肢を足す / 自分で増減) |

chat の p50 は 1.7s → 1.9s(入力 +430 tokens)。
