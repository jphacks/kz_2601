# チャット基本化 eval の追い込み(親 #184 / #189 の後続)— 2026-10-25

[2026-10-25-chat-first.md](2026-10-25-chat-first.md) の「懸念・次にやるなら」3 点を、`chat_first.go` のプロンプトだけで直した。API の契約(リクエスト・レスポンスの形、案の数の上限 3)は変えていない。

- モデル・設定は前回と同じ(chat = `claude-sonnet-5-5` / effort low)
- 実行: `go run ./cmd/ai-eval -chat -capabilities answer,proposal -env-file <.env> -tag tune-rN`。API 実行は 2 周(r1 → 字数をさらに絞る → r2)
- raw JSON はコミットしていない(行数を増やさないため)。数値は下の表がすべて

## 結果(before = #189 の r2、after = 今回の r2)

| | before | after r1 | after r2 |
|---|---|---|---|
| ケース | 24 / 24 | 27 / 27 | **27 / 27** |
| ステップ | 31 / 31 | 34 / 34 | **34 / 34** |
| proposal p50(全体) | 3.7 秒(n=3) | 2.95 秒(n=5) | **2.4 秒**(n=5、最大 3.0 秒) |
| edit / proposal p50 | 3.6 秒(n=2) | 3.2 秒(n=3) | **2.4 秒**(n=3) |
| create / proposal | 3.7 秒(n=1) | 2.8 / 3.0 秒 | 2.4 / 3.0 秒 |
| edit / answer p50 | 1.7 秒 | 1.9 秒 | 1.9 秒 |
| 全体 p50(Claude を呼んだステップ) | 2.1 秒 | 2.0 秒 | 2.1 秒 |

- 案の数は 3 個のまま(5 件とも 3 案)。速くなったのは各欄の字数を絞ったため(title 12 字・detail 20 字・instruction 40 字 / create は 50 字)。ストリームで 1 案ずつ返す方法は iOS との契約が変わるので採らなかった
- after r2 の edit / ready に 6.0 秒が 1 件(`edit_amount_big`)。ready の経路は今回触っていないので揺れと見ている(r1 では 3.1 秒)
- 追加したケース 3 本: `edit_benri`「もっと便利にしたい」・`create_osusume`(proposal の件数を増やして p50 を安定させる)、`edit_en_label`(英語の answer でラベルを訳さないか)

## 直したこと

1. **proposal を 3 秒以内に**: 各欄の上限を短くし、「グリッドの座標や細かい数値は書かない」を足した(前は「A1:F3 に広げて」のような指示が出ていた)。案 1 を generate に流すケース(`edit_mieyasuku` `edit_family`)は通っている
2. **できないことを案に混ぜない**: iOS の `Record`(`ios/Packages/MetamonDSL/Sources/MetamonDSL/Records/Record.swift`)は id・data・日時しか読まず、`authorName` を捨てている。MetamonRenderer にも投稿者名を出す部品は無い。なので「このアプリの仕組み」の「(投稿者名つき)」を消し、「投稿者名は画面に出せない(代わり → 『使った人』の項目)」を「できないこと」に足した。案には「できないことを一部にも混ぜない」と明記。`edit_family` に `optionsNot: 投稿者` と `genNot: 省き/省いた/出せない` を付け、r1・r2 とも通過(generate も何も落とさず通る)
3. **英語の answer でラベルを訳さない**: 「画面名・タブ名・ラベルは今のアプリの表記どおり『』で引用。en でも訳さない」を足した。r2: "The 『集計』 tab has a pie chart 『カテゴリ別(今月)』 …"(前は "Stats tab" "Category (this month)")

## eval ツールに足したもの

`chat_cases.json` のステップに `optionsNot`(どの案の title / detail / instruction にも含まれない)と `genNot`(generate の summary に含まれない)を足した。

## 残る懸念

- proposal の件数がまだ 5 件で、p50 の揺れは ±0.5 秒くらいある。3 秒の余裕は 0.6 秒ほど
- `edit_family` の r2 は text に「家族だけに限る設定はまだありません」が出なかった(案 1 の detail に「家族以外も見られる」とは書いている)。共有は誰でも読めることを、案を選ぶ前に声でも伝えたいなら iOS 側で detail も読み上げるか、ここで text に必ず入れさせる
- 投稿者名を本当に出したいなら iOS の `Record` に `authorName` を足し、一覧の行で出す部品を作る必要がある(backend は既に保存している)
